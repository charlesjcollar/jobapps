package alpaca

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"jobapps/bff/plugins/shared/greenhouse"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
	"jobapps/bff/store"
)

func job(title, location string, offices ...string) greenhouse.Job {
	var j greenhouse.Job
	j.Title = title
	j.Location.Name = location
	for _, o := range offices {
		j.Offices = append(j.Offices, struct {
			Name     string `json:"name"`
			Location string `json:"location"`
		}{Name: o})
	}
	return j
}

func TestCheck(t *testing.T) {
	cases := []struct {
		j          greenhouse.Job
		text       string
		wantReject string // substring of the reason; "" = kept
	}{
		// Locations that count.
		{job("Senior Software Engineer - Ledger", "Remote - Americas"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote -  Americas "), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote Americas"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - North America"), "", ""},
		{job("Senior Software Engineer - Ledger", "North America - Remote"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - Global"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - Global (Eastern Time Zone)"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - Anywhere"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - Europe, Americas"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - Europe or Remote America"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - North America - South America"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote - US"), "", ""},
		// No "Remote" in the location, but the office says so.
		{job("Senior Software Engineer - Ledger", "Global Anywhere", "Remote - Global - Anywhere"), "", ""},
		{job("Senior Software Engineer - Ledger", "North America and Europe", "Remote - European Time Zones", "Remote - Global - Eastern Time Zone"), "", ""},
		// Just "Remote": decided by the offices.
		{job("Senior Software Engineer - Ledger", "Remote", "Remote - United States"), "", ""},
		{job("Senior Software Engineer - Ledger", "Remote", "Remote - South America"), "", "no office"},

		// Locations that don't.
		{job("Senior Software Engineer - Ledger", "Remote - Europe"), "", "isn't in"},
		{job("Senior Software Engineer - Ledger", "Remote - EMEA"), "", "isn't in"},
		{job("Senior Software Engineer - Ledger", "Remote - Japan"), "", "isn't in"},
		{job("Senior Software Engineer - Ledger", "Remote - South America"), "", "isn't in"},
		{job("Senior Software Engineer - Ledger", "Remote - Canada"), "", "isn't in"},
		{job("Senior Software Engineer - Ledger", "India", "Remote - Global - Anywhere"), "", "isn't in"},
		{job("Senior Software Engineer - Ledger", "New York, NY", "New York"), "", "isn't remote"},

		// Titles.
		{job("Software Engineer - Core Trading", "Remote - Americas or EU"), "", ""},
		{job("Senior Full-Stack Engineer - Trading API", "Remote - Americas"), "", ""},
		{job("Senior Full Stack Engineer", "Remote - Americas"), "", ""},
		{job("Senior Product Engineer - Growth", "Remote - North America - Europe"), "", ""},
		{job("Senior Software Engineer – Agent Experiences", "Remote - Americas, Europe"), "", ""},
		{job("Senior Software Engineer (US or Canada)", "Remote - Americas"), "", ""},
		{job("Staff Software Engineer - Platform", "Remote - Americas"), "", "doesn't start"},
		{job("Senior Software Engineer, Staff Level", "Remote - Americas"), "", `"Staff"`},
		{job("Senior Software Engineer, Quality Engineering", "Global Anywhere", "Remote - Global - Anywhere"), "", `"Quality"`},
		{job("Senior Software Engineer - Security", "Remote - Americas"), "", `"Security"`},
		{job("AI Security Engineer", "Remote - Americas"), "", "doesn't start"},
		{job("Engineering Manager (Hands-On), Canada", "Remote - Americas"), "", "doesn't start"},
		{job("Software Engineer Intern", "Remote - Americas"), "", `"Intern"`},
		{job("Senior Data Platform Engineer", "Remote - Americas"), "", "doesn't start"},
		{job("Senior Software Engineer - New Markets Japan", "Remote - Global"), "", "tied to Japan"},
		{job("Senior Software Engineer, Canada", "Remote - Americas"), "", "tied to Canada"},
		{job("Senior Software Engineer - APAC", "Remote - Anywhere"), "", "tied to APAC"},

		// Years of experience.
		{job("Senior Software Engineer", "Remote - Americas"), "5+ years of experience building systems", ""},
		{job("Senior Software Engineer", "Remote - Americas"), "6+ years of professional experience", ""},
		{job("Senior Software Engineer", "Remote - Americas"), "7+ years of professional experience", "7+ years"},
	}
	for _, c := range cases {
		got := Criteria.Check(c.j, c.text)
		if c.wantReject == "" && got != "" {
			t.Errorf("%q @ %q: rejected (%s), want kept", c.j.Title, c.j.Location.Name, got)
		}
		if c.wantReject != "" && !strings.Contains(got, c.wantReject) {
			t.Errorf("%q @ %q: got %q, want rejection containing %q", c.j.Title, c.j.Location.Name, got, c.wantReject)
		}
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
	fixture, err := os.ReadFile("testdata/jobs.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/boards/alpaca/jobs" || r.URL.Query().Get("content") != "true" {
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
	if a.CompanyName != "Alpaca" || a.PostingID != "7000001" || a.JobTitle != "Senior Full-Stack Engineer - Example Tools" {
		t.Errorf("job 1 = %+v", a)
	}
	if a.Link != "https://job-boards.greenhouse.io/alpaca/jobs/7000001" {
		t.Errorf("link = %q", a.Link)
	}
	if a.PostedDate == nil || a.PostedDate.Format("2006-01-02") != "2026-11-02" {
		t.Errorf("posted = %v", a.PostedDate)
	}
	if a.Location == nil || *a.Location != "Remote - North America - South America - Europe" {
		t.Errorf("location = %v", a.Location)
	}
	// "USD $500 ... USD $150" stipends aren't a salary range.
	if a.SalaryRange != nil {
		t.Errorf("salary = %q, want none", *a.SalaryRange)
	}

	if b := jobs[1]; b.SalaryRange == nil || *b.SalaryRange != "$150,000 – $180,000" {
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
		{ID: 1, PostingID: "7000001", Status: "NEW"},           // listed and kept
		{ID: 2, PostingID: "7000003", Status: "INTERESTED"},    // listed but filtered out: still open
		{ID: 3, PostingID: "6000000", Status: "USER_REJECTED"}, // gone
	}}
	if _, err := New(fixtureServer(t).URL, fastWeb(), Criteria).Scan(scan.WithExpirer(context.Background(), e)); err != nil {
		t.Fatal(err)
	}
	if e.company != "Alpaca" {
		t.Errorf("checked company %q, want Alpaca", e.company)
	}
	if len(e.expired) != 1 || e.expired[0] != 3 {
		t.Errorf("expired %v, want [3]", e.expired)
	}
}

func TestScanErrors(t *testing.T) {
	for _, c := range []struct {
		name, body, wantErr string
		status              int
	}{
		{"server error", "oops", "500", 500},
		{"no jobs field", `{"postings": []}`, `no "jobs" field`, 200},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()
			e := &recordingExpirer{saved: []store.Job{{ID: 1, PostingID: "6000000", Status: "NEW"}}}
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
