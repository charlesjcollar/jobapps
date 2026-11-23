// Package scan runs plugins that look for new job postings and saves what they find.
//
// A scan runs every configured plugin (one per company) in the background. Each
// plugin's progress is tracked separately; a plugin that fails, panics, or times
// out is recorded as failed and the rest keep running. Subscribers receive a full
// snapshot of the scan every time anything changes.
package scan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"jobapps/bff/store"
)

// Plugin scans one company's job postings.
type Plugin interface {
	// Name identifies the plugin in the UI, usually the company name.
	Name() string
	// Scan returns every current posting it can find. Postings that are already
	// saved are filtered out afterwards, so plugins don't need to track that.
	Scan(ctx context.Context) ([]store.NewJob, error)
}

// Saver stores newly found jobs, skipping ones already saved.
type Saver interface {
	InsertNew(ctx context.Context, jobs []store.NewJob) ([]store.Job, error)
}

type RunStatus string

const (
	Pending   RunStatus = "PENDING"
	Running   RunStatus = "RUNNING"
	Succeeded RunStatus = "SUCCEEDED"
	Failed    RunStatus = "FAILED"
)

// Run is one plugin's progress within a scan.
type Run struct {
	Name       string
	Status     RunStatus
	StartedAt  *time.Time
	FinishedAt *time.Time
	JobsFound  int // postings the plugin returned
	JobsAdded  int // postings that weren't already saved
	Error      string
}

// Snapshot is a point-in-time copy of a scan. Safe to read without locking.
type Snapshot struct {
	ID         string
	StartedAt  time.Time
	FinishedAt *time.Time
	Runs       []Run
}

func (s Snapshot) Running() bool { return s.FinishedAt == nil }

type Options struct {
	// Concurrency is how many plugins run at once. Default 4.
	Concurrency int
	// PluginTimeout bounds each plugin's Scan call. Default 2 minutes.
	PluginTimeout time.Duration
}

// Manager starts scans and tracks the latest one. Only one scan runs at a time.
type Manager struct {
	plugins []Plugin
	saver   Saver
	opts    Options

	mu     sync.Mutex
	nextID int
	latest *scan // the running scan, or the most recent finished one
}

func NewManager(plugins []Plugin, saver Saver, opts Options) *Manager {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.PluginTimeout <= 0 {
		opts.PluginTimeout = 2 * time.Minute
	}
	return &Manager{plugins: plugins, saver: saver, opts: opts}
}

// Start begins a scan in the background and returns its initial state. If a
// scan is already running, Start returns that scan instead of starting another.
func (m *Manager) Start() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.latest != nil {
		if snap := m.latest.snapshot(); snap.Running() {
			return snap
		}
	}

	m.nextID++
	s := &scan{
		id:        strconv.Itoa(m.nextID),
		startedAt: time.Now(),
		runs:      make([]Run, len(m.plugins)),
		subs:      map[chan Snapshot]struct{}{},
	}
	for i, p := range m.plugins {
		s.runs[i] = Run{Name: p.Name(), Status: Pending}
	}
	m.latest = s

	if len(m.plugins) == 0 {
		now := time.Now()
		s.finishedAt = &now
		return s.snapshot()
	}

	go m.run(s)
	return s.snapshot()
}

// Current returns the running scan, or nil if none is running.
func (m *Manager) Current() *Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.latest == nil {
		return nil
	}
	snap := m.latest.snapshot()
	if !snap.Running() {
		return nil
	}
	return &snap
}

// Subscribe returns a channel that immediately receives the scan's current
// state, then a new snapshot on every change. It is closed after the final
// snapshot of a finished scan, or when ctx is done. A slow reader only misses
// intermediate snapshots, never the latest one.
func (m *Manager) Subscribe(ctx context.Context, id string) (<-chan Snapshot, error) {
	m.mu.Lock()
	s := m.latest
	m.mu.Unlock()
	if s == nil || s.id != id {
		return nil, fmt.Errorf("scan %s not found", id)
	}
	return s.subscribe(ctx), nil
}

func (m *Manager) run(s *scan) {
	sem := make(chan struct{}, m.opts.Concurrency)
	var wg sync.WaitGroup
	for i, p := range m.plugins {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			m.runPlugin(s, i, p)
		}()
	}
	wg.Wait()
	s.finish()
}

func (m *Manager) runPlugin(s *scan, i int, p Plugin) {
	s.update(i, func(r *Run) {
		now := time.Now()
		r.Status = Running
		r.StartedAt = &now
	})

	found, added, err := m.scanAndSave(p)

	s.update(i, func(r *Run) {
		now := time.Now()
		r.FinishedAt = &now
		r.JobsFound = found
		r.JobsAdded = added
		if err != nil {
			r.Status = Failed
			r.Error = err.Error()
			log.Printf("scan %s: plugin %q failed: %v", s.id, p.Name(), err)
		} else {
			r.Status = Succeeded
		}
	})
}

func (m *Manager) scanAndSave(p Plugin) (found, added int, err error) {
	jobs, err := m.callPlugin(p)
	if err != nil {
		return 0, 0, err
	}
	if err := validate(jobs); err != nil {
		return len(jobs), 0, err
	}
	if len(jobs) == 0 {
		return 0, 0, nil
	}

	inserted, err := m.saver.InsertNew(context.Background(), jobs)
	if err != nil {
		return len(jobs), 0, fmt.Errorf("saving jobs: %w", err)
	}
	return len(jobs), len(inserted), nil
}

// callPlugin runs p.Scan with a timeout, turning panics into errors. The call
// runs in its own goroutine so the timeout holds even if the plugin ignores ctx.
func (m *Manager) callPlugin(p Plugin) ([]store.NewJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.opts.PluginTimeout)
	defer cancel()

	type result struct {
		jobs []store.NewJob
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("plugin %q panicked: %v\n%s", p.Name(), rec, debug.Stack())
				done <- result{err: fmt.Errorf("panic: %v", rec)}
			}
		}()
		jobs, err := p.Scan(ctx)
		done <- result{jobs, err}
	}()

	timeout := fmt.Errorf("timed out after %s", m.opts.PluginTimeout)
	select {
	case r := <-done:
		if errors.Is(r.err, context.DeadlineExceeded) {
			return nil, timeout
		}
		return r.jobs, r.err
	case <-ctx.Done():
		return nil, timeout
	}
}

// validate rejects the whole batch if any posting is missing a required field,
// so plugin bugs surface as a failed run, not silently bad rows.
func validate(jobs []store.NewJob) error {
	for i := range jobs {
		j := &jobs[i]
		for _, f := range []struct {
			name string
			v    *string
		}{
			{"companyName", &j.CompanyName},
			{"jobTitle", &j.JobTitle},
			{"postingId", &j.PostingID},
			{"link", &j.Link},
		} {
			*f.v = strings.TrimSpace(*f.v)
			if *f.v == "" {
				return fmt.Errorf("posting %d of %d has an empty %s", i+1, len(jobs), f.name)
			}
		}
	}
	return nil
}

// scan is the mutable state behind a Snapshot.
type scan struct {
	id        string
	startedAt time.Time

	mu         sync.Mutex
	finishedAt *time.Time
	runs       []Run
	subs       map[chan Snapshot]struct{}
}

func (s *scan) snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *scan) snapshotLocked() Snapshot {
	runs := make([]Run, len(s.runs))
	copy(runs, s.runs)
	return Snapshot{ID: s.id, StartedAt: s.startedAt, FinishedAt: s.finishedAt, Runs: runs}
}

func (s *scan) update(i int, f func(*Run)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.runs[i])
	s.broadcastLocked()
}

func (s *scan) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.finishedAt = &now
	s.broadcastLocked()
	for ch := range s.subs {
		close(ch)
	}
	s.subs = nil
}

// broadcastLocked sends the current snapshot to every subscriber. Channels hold
// one snapshot; an unread one is replaced, since only the latest matters.
func (s *scan) broadcastLocked() {
	snap := s.snapshotLocked()
	for ch := range s.subs {
		select {
		case <-ch:
		default:
		}
		ch <- snap
	}
}

func (s *scan) subscribe(ctx context.Context) <-chan Snapshot {
	ch := make(chan Snapshot, 1)

	s.mu.Lock()
	ch <- s.snapshotLocked()
	if s.finishedAt != nil {
		close(ch)
		s.mu.Unlock()
		return ch
	}
	s.subs[ch] = struct{}{}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
	}()
	return ch
}
