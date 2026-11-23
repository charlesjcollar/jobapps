package scan

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jobapps/bff/store"
)

type fakePlugin struct {
	name  string
	delay time.Duration
	jobs  []store.NewJob
	err   error
	panic bool
	// ignoreCtx makes Scan block for delay even after the context is cancelled.
	ignoreCtx bool
	running   *atomic.Int32
	maxSeen   *atomic.Int32
}

func (p fakePlugin) Name() string { return p.name }

func (p fakePlugin) Scan(ctx context.Context) ([]store.NewJob, error) {
	if p.running != nil {
		n := p.running.Add(1)
		defer p.running.Add(-1)
		for {
			m := p.maxSeen.Load()
			if n <= m || p.maxSeen.CompareAndSwap(m, n) {
				break
			}
		}
	}
	if p.ignoreCtx {
		time.Sleep(p.delay)
	} else {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if p.panic {
		panic("boom")
	}
	return p.jobs, p.err
}

// fakeSaver dedupes on (company, postingId) like the real store.
type fakeSaver struct {
	mu   sync.Mutex
	seen map[string]bool
	err  error
}

func (s *fakeSaver) InsertNew(_ context.Context, jobs []store.NewJob) ([]store.Job, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	var out []store.Job
	for _, j := range jobs {
		k := j.CompanyName + "|" + j.PostingID
		if !s.seen[k] {
			s.seen[k] = true
			out = append(out, store.Job{CompanyName: j.CompanyName, PostingID: j.PostingID})
		}
	}
	return out, nil
}

func job(company, id string) store.NewJob {
	return store.NewJob{CompanyName: company, JobTitle: "Engineer", PostingID: id, Link: "https://example.com/" + id}
}

// waitFinal reads snapshots until the channel closes and returns the last one.
func waitFinal(t *testing.T, ch <-chan Snapshot) (last Snapshot, count int) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case s, ok := <-ch:
			if !ok {
				return last, count
			}
			last, count = s, count+1
		case <-timeout:
			t.Fatal("scan did not finish")
		}
	}
}

func runToEnd(t *testing.T, m *Manager) Snapshot {
	t.Helper()
	start := m.Start()
	ch, err := m.Subscribe(context.Background(), start.ID)
	if err != nil {
		t.Fatal(err)
	}
	final, _ := waitFinal(t, ch)
	return final
}

func TestFailuresDontStopOtherPlugins(t *testing.T) {
	m := NewManager([]Plugin{
		fakePlugin{name: "ok", delay: 10 * time.Millisecond, jobs: []store.NewJob{job("A", "1"), job("A", "2")}},
		fakePlugin{name: "err", delay: 5 * time.Millisecond, err: errors.New("HTTP 500 from careers page")},
		fakePlugin{name: "panic", panic: true},
		fakePlugin{name: "slow", delay: time.Hour},
		fakePlugin{name: "stuck", delay: time.Hour, ignoreCtx: true},
		fakePlugin{name: "bad job", jobs: []store.NewJob{job("B", "1"), {CompanyName: "B", JobTitle: "x", Link: "y"}}},
		fakePlugin{name: "ok2", jobs: []store.NewJob{job("C", "1")}},
	}, &fakeSaver{}, Options{Concurrency: 7, PluginTimeout: 100 * time.Millisecond})

	final := runToEnd(t, m)
	if final.Running() {
		t.Fatal("final snapshot still running")
	}

	want := map[string]struct {
		status       RunStatus
		found, added int
		errContains  string
	}{
		"ok":      {Succeeded, 2, 2, ""},
		"err":     {Failed, 0, 0, "HTTP 500"},
		"panic":   {Failed, 0, 0, "panic: boom"},
		"slow":    {Failed, 0, 0, "timed out"},
		"stuck":   {Failed, 0, 0, "timed out"},
		"bad job": {Failed, 2, 0, "posting 2 of 2 has an empty postingId"},
		"ok2":     {Succeeded, 1, 1, ""},
	}
	for _, r := range final.Runs {
		w := want[r.Name]
		if r.Status != w.status || r.JobsFound != w.found || r.JobsAdded != w.added || !strings.Contains(r.Error, w.errContains) {
			t.Errorf("%s: got %+v, want %+v", r.Name, r, w)
		}
		if r.StartedAt == nil || r.FinishedAt == nil {
			t.Errorf("%s: missing timestamps", r.Name)
		}
	}
}

func TestSecondScanSkipsSavedJobs(t *testing.T) {
	saver := &fakeSaver{}
	m := NewManager([]Plugin{fakePlugin{name: "A", jobs: []store.NewJob{job("A", "1"), job("A", "2")}}}, saver, Options{})
	if r := runToEnd(t, m).Runs[0]; r.JobsAdded != 2 {
		t.Fatalf("first scan: %+v", r)
	}
	if r := runToEnd(t, m).Runs[0]; r.JobsFound != 2 || r.JobsAdded != 0 {
		t.Fatalf("second scan: %+v", r)
	}
}

func TestSaveErrorFailsPlugin(t *testing.T) {
	m := NewManager([]Plugin{fakePlugin{name: "A", jobs: []store.NewJob{job("A", "1")}}}, &fakeSaver{err: errors.New("db down")}, Options{})
	if r := runToEnd(t, m).Runs[0]; r.Status != Failed || !strings.Contains(r.Error, "saving jobs: db down") {
		t.Fatalf("got %+v", r)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var running, maxSeen atomic.Int32
	var ps []Plugin
	for range 10 {
		ps = append(ps, fakePlugin{name: "p", delay: 20 * time.Millisecond, running: &running, maxSeen: &maxSeen})
	}
	m := NewManager(ps, &fakeSaver{}, Options{Concurrency: 3})
	runToEnd(t, m)
	if got := maxSeen.Load(); got != 3 {
		t.Errorf("max concurrent plugins = %d, want 3", got)
	}
}

func TestOneScanAtATime(t *testing.T) {
	m := NewManager([]Plugin{fakePlugin{name: "A", delay: 50 * time.Millisecond}}, &fakeSaver{}, Options{})
	first := m.Start()
	if again := m.Start(); again.ID != first.ID {
		t.Fatalf("second Start started a new scan: %s vs %s", again.ID, first.ID)
	}
	if cur := m.Current(); cur == nil || cur.ID != first.ID {
		t.Fatalf("Current: got %+v", cur)
	}

	ch, _ := m.Subscribe(context.Background(), first.ID)
	waitFinal(t, ch)

	if cur := m.Current(); cur != nil {
		t.Errorf("Current after finish: got %+v, want nil", cur)
	}
	if next := m.Start(); next.ID == first.ID {
		t.Error("Start after finish reused the old scan")
	}
}

func TestSubscribe(t *testing.T) {
	m := NewManager([]Plugin{
		fakePlugin{name: "A", delay: 20 * time.Millisecond},
		fakePlugin{name: "B", delay: 40 * time.Millisecond},
	}, &fakeSaver{}, Options{})
	start := m.Start()
	if len(start.Runs) != 2 || start.Runs[0].Name != "A" || !start.Running() {
		t.Fatalf("initial snapshot: %+v", start)
	}

	ch, err := m.Subscribe(context.Background(), start.ID)
	if err != nil {
		t.Fatal(err)
	}
	final, n := waitFinal(t, ch)
	if n < 2 || final.Running() {
		t.Errorf("got %d snapshots, final running=%v", n, final.Running())
	}

	// Subscribing to a finished scan gets its final state, then closes.
	ch, _ = m.Subscribe(context.Background(), start.ID)
	if again, n := waitFinal(t, ch); n != 1 || again.Running() {
		t.Errorf("late subscribe: %d snapshots, running=%v", n, again.Running())
	}

	if _, err := m.Subscribe(context.Background(), "nope"); err == nil {
		t.Error("expected error for unknown scan")
	}
}

func TestSubscribeCancel(t *testing.T) {
	m := NewManager([]Plugin{fakePlugin{name: "A", delay: time.Hour}}, &fakeSaver{}, Options{PluginTimeout: time.Hour})
	start := m.Start()
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := m.Subscribe(ctx, start.ID)
	<-ch
	cancel()
	select {
	case _, ok := <-ch:
		for ok {
			_, ok = <-ch
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after cancel")
	}
}

func TestNoPlugins(t *testing.T) {
	m := NewManager(nil, &fakeSaver{}, Options{})
	s := m.Start()
	if s.Running() || len(s.Runs) != 0 {
		t.Fatalf("got %+v", s)
	}
	if m.Current() != nil {
		t.Error("Current should be nil")
	}
}
