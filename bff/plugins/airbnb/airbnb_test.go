package airbnb

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

func job(title, location, workplace string, offices ...string) greenhouse.Job {
	var j greenhouse.Job
	j.Title = title
	j.Location.Name = location
	if workplace != "" {
		j.Metadata = append(j.Metadata, struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		}{"Workplace Type", workplace})
	}
	for _, o := range offices {
		j.Offices = append(j.Offices, struct {
			Name     string `json:"name"`
			Location string `json:"location"`
		}{o, o})
	}
	return j
}

func TestCheck(t *testing.T) {
	cases := []struct {
		j          greenhouse.Job
		text       string
		wantReject string // substring of the reason; "" = kept
	}{
		{job("Senior Software Engineer, Payments", "Remote, USA", "Remote"), "", ""},
		{job("Software Engineer, Payments", "Remote - USA", "Remote"), "", ""},
		{job("Senior Software Engineer Guest & Host", "US - Remote", "Remote"), "", ""},
		{job("Senior Full Stack Engineer, Tools", "United States", "Remote"), "", ""},
		{job("Senior Software Engineer, Prototyping (Full Stack)", "United States", "Remote"), "", ""},
		{job("Senior Software Engineer, Internal Tools", "United States", "Remote"), "", ""},
		// Location is a city, but the office says US and the role is remote.
		{job("Senior Software Engineer", "San Francisco, CA", "Remote", "San Francisco, United States"), "", ""},
		{job("Senior Software Engineer", "United States", "remote"), "", ""},

		{job("Staff Software Engineer, Payments", "United States", "Remote"), "", "doesn't start"},
		{job("Senior Staff Software Engineer", "United States", "Remote"), "", "doesn't start"},
		{job("Senior Software Engineer, Staff Level", "United States", "Remote"), "", `"Staff"`},
		{job("Software Engineering Intern (BS/MS)", "United States", "Onsite"), "", "doesn't start"},
		{job("Software Engineer Intern", "United States", "Remote"), "", `"Intern"`},
		{job("Senior Software Engineer, Security", "United States", "Remote"), "", `"Security"`},
		{job("Senior Security Engineer", "United States", "Remote"), "", "doesn't start"},
		{job("Software Engineer, Manager Tools", "United States", "Remote"), "", `"Manager"`},
		{job("Engineering Manager, Payments", "United States", "Remote"), "", "doesn't start"},
		{job("Senior Machine Learning Engineer", "United States", "Remote"), "", "doesn't start"},
		{job("Senior Software Engineer", "Brazil", "Remote", "Brazil"), "", "isn't in"},
		{job("Senior Software Engineer", "Campus, Somewhere", "Remote"), "", "isn't in"},
		{job("Senior Software Engineer", "United States", "Hybrid"), "", "workplace type is Hybrid"},
		{job("Senior Software Engineer", "San Francisco, CA", "Onsite", "San Francisco, United States"), "", "workplace type is Onsite"},
		{job("Senior Software Engineer", "United States", ""), "", "no workplace type"},

		{job("Senior Software Engineer", "United States", "Remote"), "5+ years of software engineering experience", ""},
		{job("Senior Software Engineer", "United States", "Remote"), "6+ years of relevant experience", ""},
		{job("Senior Software Engineer", "United States", "Remote"), "7+ years of industry experience", "7+ years"},
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

func TestScan(t *testing.T) {
	fixture, err := os.ReadFile("testdata/jobs.json")
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/boards/airbnb/jobs" || r.URL.Query().Get("content") != "true" {
			http.NotFound(w, r)
			return
		}
		w.Write(fixture)
	}))
	defer srv.Close()

	jobs, err := New(srv.URL, fastWeb(), Criteria).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
	if len(jobs) != 2 {
		for _, j := range jobs {
			t.Logf("kept %s", j.JobTitle)
		}
		t.Fatalf("kept %d jobs, want 2", len(jobs))
	}

	a := jobs[0]
	if a.CompanyName != "Airbnb" || a.PostingID != "9000001" || a.JobTitle != "Senior Software Engineer, Example Team" {
		t.Errorf("job 1 = %+v", a)
	}
	if a.Link != "https://careers.airbnb.com/positions/9000001?gh_jid=9000001" {
		t.Errorf("link = %q", a.Link)
	}
	if a.PostedDate == nil || a.PostedDate.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("posted = %v", a.PostedDate)
	}
	if a.Location == nil || *a.Location != "Remote, USA" {
		t.Errorf("location = %v", a.Location)
	}
	if a.SalaryRange == nil || *a.SalaryRange != "$150,000 – $180,000" {
		t.Errorf("salary = %v", a.SalaryRange)
	}

	b := jobs[1]
	if b.Location == nil || *b.Location != "San Francisco, CA (Remote)" {
		t.Errorf("job 2 location = %v", b.Location)
	}
	if b.SalaryRange != nil {
		t.Errorf("job 2 salary = %q, want none", *b.SalaryRange)
	}
}

func TestScanErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"server error", 500, "oops", "500"},
		{"not found", 404, "", "404"},
		{"not json", 200, "<html>", "greenhouse"},
		{"no jobs field", 200, `{"postings": []}`, `no "jobs" field`},
		{"workplace field renamed", 200, `{"jobs": [{"id": 1, "title": "Senior Software Engineer", "location": {"name": "United States"}, "metadata": [{"name": "Work Style", "value": "Remote"}]}]}`, `"Workplace Type"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()
			jobs, err := New(srv.URL, fastWeb(), Criteria).Scan(context.Background())
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, c.wantErr)
			}
			if len(jobs) != 0 {
				t.Errorf("got %d jobs, want none", len(jobs))
			}
		})
	}
}

func TestEmptyBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jobs": [], "meta": {"total": 0}}`))
	}))
	defer srv.Close()
	jobs, err := New(srv.URL, fastWeb(), Criteria).Scan(context.Background())
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%d err=%v, want 0 and nil", len(jobs), err)
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
	fixture, err := os.ReadFile("testdata/jobs.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(fixture) }))
	defer srv.Close()

	e := &recordingExpirer{saved: []store.Job{
		{ID: 1, PostingID: "9000001", Status: "NEW"},           // listed and kept
		{ID: 2, PostingID: "9000003", Status: "INTERESTED"},    // listed but filtered out (hybrid): still open
		{ID: 3, PostingID: "8000000", Status: "USER_REJECTED"}, // gone
	}}
	jobs, err := New(srv.URL, fastWeb(), Criteria).Scan(scan.WithExpirer(context.Background(), e))
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Errorf("kept %d, want 2", len(jobs))
	}
	if e.company != "Airbnb" {
		t.Errorf("checked company %q, want Airbnb", e.company)
	}
	if len(e.expired) != 1 || e.expired[0] != 3 {
		t.Errorf("expired %v, want [3]", e.expired)
	}
}

func TestScanSkipsExpirationOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer srv.Close()
	e := &recordingExpirer{saved: []store.Job{{ID: 3, PostingID: "8000000", Status: "NEW"}}}
	if _, err := New(srv.URL, fastWeb(), Criteria).Scan(scan.WithExpirer(context.Background(), e)); err == nil {
		t.Fatal("want an error")
	}
	if e.company != "" || len(e.expired) != 0 {
		t.Errorf("expiration ran after a failed fetch: company=%q expired=%v", e.company, e.expired)
	}
}
