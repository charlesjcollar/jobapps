package monarchmoney

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"jobapps/bff/plugins/shared/ashby"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
	"jobapps/bff/store"
)

func job(title, workplace string, locations ...string) ashby.Job {
	j := ashby.Job{Title: title, WorkplaceType: workplace, IsListed: true}
	if len(locations) > 0 {
		j.Location = locations[0]
		for _, l := range locations[1:] {
			j.SecondaryLocations = append(j.SecondaryLocations, struct {
				Location string `json:"location"`
			}{l})
		}
	}
	return j
}

func TestCheck(t *testing.T) {
	cases := []struct {
		j          ashby.Job
		text       string
		wantReject string // substring of the reason; "" = kept
	}{
		{job("Future Engineering Opportunities", "Remote", "Remote"), "", ""},
		{job("Senior Software Engineer, Mobile", "Remote", "Remote (US)"), "", ""},
		{job("Software Engineer, Backend", "Remote", "Remote (US)", "Remote (Canada)"), "", ""},
		{job("Senior Full-Stack Engineer", "Remote", "Remote (USA)"), "", ""},
		{job("Senior Product Engineer", "Remote", "Remote - United States"), "", ""},
		{job("Software Engineer", "Remote", "Remote (Canada)", "Remote"), "", ""}, // also open anywhere

		// Combined levels count as their lowest.
		{job("Software Engineer (Senior/Staff)", "Remote", "Remote (US)"), "", ""},
		{job("Senior/Staff Software Engineer", "Remote", "Remote (US)"), "", ""},
		{job("Software Engineer (Senior/Staff/Principal), Platform", "Remote", "Remote (US)"), "", ""},
		{job("Software Engineer (Staff/Principal)", "Remote", "Remote (US)"), "", `"Staff"`},
		{job("Staff/Principal Software Engineer", "Remote", "Remote (US)"), "", "doesn't start"},
		{job("Software Engineer (iOS/Android)", "Remote", "Remote (US)"), "", ""},

		{job("Future Opportunities", "Remote", "Remote"), "", "doesn't start"},
		{job("IT Support Engineer", "Remote", "Remote"), "", "doesn't start"},
		{job("Product Manager (Senior/Staff/Principal)", "Remote", "Remote (US)"), "", "doesn't start"},
		{job("Staff Software Engineer", "Remote", "Remote (US)"), "", "doesn't start"},
		{job("Software Engineering Manager", "Remote", "Remote (US)"), "", "doesn't start"},
		{job("Senior Software Engineer, Security", "Remote", "Remote (US)"), "", `"Security"`},
		{job("Software Engineer Intern", "Remote", "Remote (US)"), "", `"Intern"`},
		{job("Senior Software Engineer, Quality", "Remote", "Remote (US)"), "", `"Quality"`},

		{job("Senior Software Engineer", "Remote", "Remote (Canada)"), "", "aren't in"},
		{job("Senior Software Engineer", "Remote", "Remote (non-US)"), "", "aren't in"},
		{job("Senior Software Engineer", "Remote", "Remote (Canada)", "Remote (non-US)"), "", "aren't in"},
		{job("Senior Software Engineer", "Hybrid", "New York (US)"), "", "workplace type is Hybrid"},
		{job("Senior Software Engineer", "OnSite", "Remote (US)"), "", "workplace type is OnSite"},
		{job("Senior Software Engineer", "", "Remote (US)"), "", "no workplace type"},

		{job("Senior Software Engineer", "Remote", "Remote (US)"), "6+ years of professional experience", ""},
		{job("Senior Software Engineer", "Remote", "Remote (US)"), "7+ years of professional experience", "7+ years"},
	}
	for _, c := range cases {
		got := Criteria.Check(c.j, c.text)
		if c.wantReject == "" && got != "" {
			t.Errorf("%q @ %q: rejected (%s), want kept", c.j.Title, c.j.Location, got)
		}
		if c.wantReject != "" && !strings.Contains(got, c.wantReject) {
			t.Errorf("%q @ %q: got %q, want rejection containing %q", c.j.Title, c.j.Location, got, c.wantReject)
		}
	}
}

func TestIsRemoteWithoutWorkplaceType(t *testing.T) {
	j := job("Senior Software Engineer", "", "Remote (US)")
	j.IsRemote = true
	if got := Criteria.Check(j, ""); got != "" {
		t.Errorf("rejected (%s), want kept", got)
	}
}

func fastWeb() *web.Client {
	w := web.New()
	w.MinInterval = 0
	w.BaseBackoff = time.Millisecond
	w.MaxRetries = 1
	return w
}

func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	fixture, err := os.ReadFile("testdata/board.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/posting-api/job-board/monarchmoney" || r.URL.Query().Get("includeCompensation") != "true" {
			http.NotFound(w, r)
			return
		}
		w.Write(fixture)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestScan(t *testing.T) {
	jobs, err := New(fixtureServer(t).URL, fastWeb(), Criteria).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		for _, j := range jobs {
			t.Logf("kept %s", j.JobTitle)
		}
		t.Fatalf("kept %d jobs, want 2", len(jobs))
	}

	a := jobs[0]
	if a.CompanyName != "Monarch Money" || a.PostingID != "00000000-0000-0000-0000-000000000001" ||
		a.JobTitle != "Future Engineering Opportunities" ||
		a.Link != "https://jobs.ashbyhq.com/monarchmoney/00000000-0000-0000-0000-000000000001" {
		t.Errorf("job 1 = %+v", a)
	}
	if a.PostedDate == nil || a.PostedDate.Format("2006-01-02") != "2025-05-22" {
		t.Errorf("posted = %v", a.PostedDate)
	}
	if a.Location == nil || *a.Location != "Remote" {
		t.Errorf("location = %v", a.Location)
	}
	if a.SalaryRange == nil || *a.SalaryRange != "$120,000 – $230,000" {
		t.Errorf("salary = %v", a.SalaryRange)
	}

	// No structured pay: falls back to the description. Title is trimmed.
	b := jobs[1]
	if b.JobTitle != "Software Engineer (Senior/Staff), Example Team" {
		t.Errorf("job 2 title = %q", b.JobTitle)
	}
	if b.SalaryRange == nil || *b.SalaryRange != "$150,000 – $190,000" {
		t.Errorf("job 2 salary = %v", b.SalaryRange)
	}
}

// recordingExpirer returns fixed saved jobs and records what gets expired.
type recordingExpirer struct {
	saved   []store.Job
	company string
	expired []int64
}

func (e *recordingExpirer) CompanyJobs(_ context.Context, company string, _ []string) ([]store.Job, error) {
	e.company = company
	return e.saved, nil
}

func (e *recordingExpirer) MarkExpired(_ context.Context, ids []int64, _ []string) ([]store.Job, error) {
	e.expired = append(e.expired, ids...)
	return make([]store.Job, len(ids)), nil
}

func TestScanExpiresMissingPostings(t *testing.T) {
	e := &recordingExpirer{saved: []store.Job{
		{ID: 1, PostingID: "00000000-0000-0000-0000-000000000001", Status: "APPLIED"}, // listed
		{ID: 2, PostingID: "00000000-0000-0000-0000-000000000004", Status: "NEW"},     // listed, filtered out
		{ID: 3, PostingID: "00000000-0000-0000-0000-000000000005", Status: "NEW"},     // unlisted: gone
		{ID: 4, PostingID: "00000000-0000-0000-0000-000000000099", Status: "NEW"},     // gone
	}}
	if _, err := New(fixtureServer(t).URL, fastWeb(), Criteria).Scan(scan.WithExpirer(context.Background(), e)); err != nil {
		t.Fatal(err)
	}
	if e.company != "Monarch Money" {
		t.Errorf("checked company %q", e.company)
	}
	if len(e.expired) != 2 || e.expired[0] != 3 || e.expired[1] != 4 {
		t.Errorf("expired %v, want [3 4]", e.expired)
	}
}

func TestScanErrors(t *testing.T) {
	for _, c := range []struct {
		name, body, wantErr string
		status              int
	}{
		{"server error", "oops", "500", 500},
		{"no jobs field", `{"apiVersion": "1"}`, `no "jobs" field`, 200},
		{"not json", "<html>", "ashby", 200},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()
			e := &recordingExpirer{saved: []store.Job{{ID: 1, PostingID: "x", Status: "NEW"}}}
			jobs, err := New(srv.URL, fastWeb(), Criteria).Scan(scan.WithExpirer(context.Background(), e))
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, c.wantErr)
			}
			if len(jobs) != 0 || len(e.expired) != 0 {
				t.Errorf("jobs=%d expired=%v, want none", len(jobs), e.expired)
			}
		})
	}
}
