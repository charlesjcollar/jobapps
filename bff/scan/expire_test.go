package scan

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"jobapps/bff/store"
)

// fakeExpirer holds saved jobs in memory like the real store.
type fakeExpirer struct {
	mu       sync.Mutex
	jobs     []store.Job
	readErr  error
	writeErr error
	writes   int
}

func (f *fakeExpirer) CompanyJobs(_ context.Context, company string, statuses []string) ([]store.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return nil, f.readErr
	}
	var out []store.Job
	for _, j := range f.jobs {
		if j.CompanyName == company && slices.Contains(statuses, j.Status) {
			out = append(out, j)
		}
	}
	return out, nil
}

func (f *fakeExpirer) MarkExpired(_ context.Context, ids []int64, onlyIf []string) ([]store.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if f.writeErr != nil {
		return nil, f.writeErr
	}
	var out []store.Job
	for i := range f.jobs {
		if slices.Contains(ids, f.jobs[i].ID) && slices.Contains(onlyIf, f.jobs[i].Status) {
			f.jobs[i].Status = "EXPIRED"
			out = append(out, f.jobs[i])
		}
	}
	return out, nil
}

func (f *fakeExpirer) status(postingID string) string {
	for _, j := range f.jobs {
		if j.PostingID == postingID {
			return j.Status
		}
	}
	return ""
}

func saved(id int64, company, postingID, status string) store.Job {
	return store.Job{ID: id, CompanyName: company, PostingID: postingID, Status: status, JobTitle: "Engineer"}
}

func TestExpireMissing(t *testing.T) {
	f := &fakeExpirer{jobs: []store.Job{
		saved(1, "Acme", "A1", "NEW"),
		saved(2, "Acme", "A2", "INTERESTED"),
		saved(3, "Acme", "A3", "USER_REJECTED"),
		saved(4, "Acme", "A4", "APPLIED"),          // never expired
		saved(5, "Acme", "A5", "COMPANY_REJECTED"), // never expired
		saved(6, "Acme", "A6", "NEW"),              // still open
		saved(7, "Acme", "A7", "NEW"),              // still open
		saved(8, "Acme", "A8", "NEW"),              // still open
		saved(9, "Other Co", "A1", "NEW"),          // other company, same ID
	}}
	ctx := WithExpirer(context.Background(), f)

	n, err := ExpireMissing(ctx, "Acme", []string{" A6 ", "A7", "A8", "NEW-ONE"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("expired %d, want 3", n)
	}
	want := map[string]string{"A1": "EXPIRED", "A2": "EXPIRED", "A3": "EXPIRED", "A4": "APPLIED", "A5": "COMPANY_REJECTED", "A6": "NEW"}
	for id, st := range want {
		if got := f.status(id); got != st {
			t.Errorf("%s: status %s, want %s", id, got, st)
		}
	}
	if f.jobs[8].Status != "NEW" {
		t.Errorf("other company's job was changed to %s", f.jobs[8].Status)
	}
}

func TestExpireMissingNothingMissing(t *testing.T) {
	f := &fakeExpirer{jobs: []store.Job{saved(1, "Acme", "A1", "NEW")}}
	n, err := ExpireMissing(WithExpirer(context.Background(), f), "Acme", []string{"A1"})
	if n != 0 || err != nil || f.writes != 0 {
		t.Fatalf("n=%d err=%v writes=%d, want 0, nil, 0", n, err, f.writes)
	}
}

func TestExpireMissingSkips(t *testing.T) {
	// No expirer in ctx: the CLI without a database.
	if n, err := ExpireMissing(context.Background(), "Acme", []string{"A1"}); n != 0 || err != nil {
		t.Errorf("no expirer: n=%d err=%v", n, err)
	}
	// Empty list: more likely a broken site than nothing open.
	f := &fakeExpirer{jobs: []store.Job{saved(1, "Acme", "A1", "NEW")}}
	if n, err := ExpireMissing(WithExpirer(context.Background(), f), "Acme", nil); n != 0 || err != nil || f.writes != 0 {
		t.Errorf("empty list: n=%d err=%v writes=%d", n, err, f.writes)
	}
}

func TestExpireMissingSafetyLimit(t *testing.T) {
	var jobs []store.Job
	for i := range 10 {
		jobs = append(jobs, saved(int64(i+1), "Acme", "A"+string(rune('0'+i)), "NEW"))
	}

	// 6 of 10 missing: over half and over 5, so nothing changes.
	f := &fakeExpirer{jobs: slices.Clone(jobs)}
	n, err := ExpireMissing(WithExpirer(context.Background(), f), "Acme", []string{"A0", "A1", "A2", "A3"})
	if err == nil || !strings.Contains(err.Error(), "6 of 10") || n != 0 || f.writes != 0 {
		t.Fatalf("n=%d err=%v writes=%d, want refusal and no writes", n, err, f.writes)
	}

	// 5 of 10 missing: allowed.
	f = &fakeExpirer{jobs: slices.Clone(jobs)}
	n, err = ExpireMissing(WithExpirer(context.Background(), f), "Acme", []string{"A0", "A1", "A2", "A3", "A4"})
	if err != nil || n != 5 {
		t.Fatalf("n=%d err=%v, want 5 expired", n, err)
	}

	// All of a small set missing: allowed (the minimum keeps it from blocking
	// companies with only a few saved jobs).
	f = &fakeExpirer{jobs: slices.Clone(jobs[:3])}
	n, err = ExpireMissing(WithExpirer(context.Background(), f), "Acme", []string{"OTHER"})
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v, want 3 expired", n, err)
	}
}

func TestExpireMissingErrors(t *testing.T) {
	f := &fakeExpirer{readErr: errors.New("db down")}
	if _, err := ExpireMissing(WithExpirer(context.Background(), f), "Acme", []string{"A1"}); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Errorf("read error: got %v", err)
	}
	f = &fakeExpirer{jobs: []store.Job{saved(1, "Acme", "A1", "NEW")}, writeErr: errors.New("db down")}
	if _, err := ExpireMissing(WithExpirer(context.Background(), f), "Acme", []string{"A2"}); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Errorf("write error: got %v", err)
	}
}

// expiringPlugin calls ExpireMissing like a real plugin does.
type expiringPlugin struct{ open []string }

func (expiringPlugin) Name() string { return "Acme" }
func (p expiringPlugin) Scan(ctx context.Context) ([]store.NewJob, error) {
	if _, err := ExpireMissing(ctx, "Acme", p.open); err != nil {
		return nil, err
	}
	return []store.NewJob{job("Acme", "A9")}, nil
}

func TestRunCountsExpired(t *testing.T) {
	f := &fakeExpirer{jobs: []store.Job{saved(1, "Acme", "A1", "NEW"), saved(2, "Acme", "A2", "NEW")}}
	m := NewManager([]Plugin{expiringPlugin{open: []string{"A2", "A9"}}}, &fakeSaver{}, Options{Expirer: f})
	final := runToEnd(t, m)
	r := final.Runs[0]
	if r.Status != Succeeded || r.JobsExpired != 1 || r.JobsAdded != 1 {
		t.Fatalf("run = %+v, want succeeded with 1 expired and 1 added", r)
	}
}

func TestRunWithoutExpirer(t *testing.T) {
	m := NewManager([]Plugin{expiringPlugin{open: []string{"A9"}}}, &fakeSaver{}, Options{})
	r := runToEnd(t, m).Runs[0]
	if r.Status != Succeeded || r.JobsExpired != 0 {
		t.Fatalf("run = %+v", r)
	}
}
