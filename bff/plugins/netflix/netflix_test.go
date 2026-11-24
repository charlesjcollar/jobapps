package netflix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"jobapps/bff/plugins/shared/eightfold"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/store"
)

func TestCheckListing(t *testing.T) {
	us := "Los Gatos,California,United States of America"
	cases := []struct {
		title, location, option string
		wantReject              string // substring of the reason; "" = kept
	}{
		{"Software Engineer 5 - Payments", us, "remote_local", ""},
		{"Senior Software Engineer (L5), Productivity", "Remote, United States", "remote_local", ""},
		{"Software Engineer (4/5), Studio", us, "remote_local", ""},
		{"Software Engineer (5/6), Studio", us, "remote_local", ""}, // lowest level is 5
		{"Full Stack Software Engineer (4/5), Games", us, "remote_local", ""},
		{"Software Developer", us, "remote_local", ""},
		{"Software Engineer", us, "", ""}, // no level or work type listed
		{"Software Engineer 6 - Platform", us, "onsite", "level 6"},
		{"Senior Software Engineer (L6)", us, "onsite", "level 6"},
		{"Software Engineer 7", us, "onsite", "level 7"},
		{"Staff Software Engineer", us, "onsite", "doesn't start"},
		{"Software Engineer, Staff Level", us, "onsite", `"Staff"`},
		{"Software Engineer in Test 5", us, "onsite", `"in Test"`},
		{"Security Software Engineer 5", us, "onsite", "doesn't start"},
		{"Software Engineering Manager", us, "onsite", `"Manager"`},
		{"Software Engineering 5 - Ads Finance", us, "remote_local", ""},
		{"Software Engineering L5, Open Connect", us, "remote_local", ""},
		{"Software Engineering 6 - Ads Finance", us, "onsite", "level 6"},
		{"Software Engineer Intern - Summer 2027", us, "onsite", `"Intern"`},
		{"Software Engineer 5, Internal Tools", us, "remote_local", ""},
		{"Machine Learning Engineer 5", us, "onsite", "doesn't start"},
		{"Software Engineer 5", "Tartu,Estonia", "onsite", "isn't in"},
		{"Software Engineer 5", "Poland - Remote", "onsite", "isn't in"},
		{"Software Engineer 5", us, "hybrid", "labeled Hybrid"},
		{"Software Engineer 5", "New York (Hybrid),New York,United States of America", "remote_local", "labeled Hybrid"},
		{"Software Engineer 5", us, "onsite", "labeled Onsite"},
		{"Software Engineer 5", us, "On-site", "labeled Onsite"},
		// Search label contradicts a remote location: left for the details check.
		{"Software Engineer 5", "Remote, United States", "onsite", ""},
	}
	for _, c := range cases {
		got := Criteria.CheckListing(eightfold.Position{Name: c.title, Location: c.location, Locations: []string{c.location}, WorkLocationOption: c.option})
		if c.wantReject == "" && got != "" {
			t.Errorf("%q @ %q: rejected (%s), want kept", c.title, c.location, got)
		}
		if c.wantReject != "" && !strings.Contains(got, c.wantReject) {
			t.Errorf("%q @ %q: got %q, want rejection containing %q", c.title, c.location, got, c.wantReject)
		}
	}
}

func TestCheckDetails(t *testing.T) {
	cases := []struct {
		text, workType, want string
	}{
		{"You have 5+ years of experience.", "Remote", ""},
		{"You have 5+ years of experience.", "Onsite", "labeled Onsite"},
		{"You have 5+ years of experience.", "On Site", "labeled Onsite"},
		{"You have 6+ years of experience.", "", ""},
		{"You have 7+ years of experience.", "", "7+ years"},
		{"At least 10 years of experience building large systems.", "", "10+ years"},
		{"Deep experience with distributed systems.", "", ""}, // not stated: keep
		{"5+ years of experience.", "Hybrid", "labeled Hybrid"},
	}
	for _, c := range cases {
		p := eightfold.Position{}
		if c.workType != "" {
			p.CustomJD.DataFields.WorkType = []string{c.workType}
		}
		got := Criteria.CheckDetails(p, c.text)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%q / %q: got %q, want %q", c.text, c.workType, got, c.want)
		}
	}
}

// fakeSite serves testdata/search.json (paged like the real API) and
// testdata/position_<id>.json. failIDs return 500 for those positions.
func fakeSite(t *testing.T, failIDs map[int64]bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	raw, err := os.ReadFile("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	var all []json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("domain") != "netflix.com" {
			http.Error(w, "missing domain", 400)
			return
		}
		if r.URL.Path == "/api/apply/v2/jobs" {
			start, _ := strconv.Atoi(r.URL.Query().Get("start"))
			end := min(start+eightfold.PageSize, len(all))
			page := all[min(start, len(all)):end]
			json.NewEncoder(w).Encode(map[string]any{"count": len(all), "positions": page})
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/apply/v2/jobs/")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if failIDs[id] {
			http.Error(w, "boom", 500)
			return
		}
		body, err := os.ReadFile(fmt.Sprintf("testdata/position_%s.json", idStr))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func fastClient() *web.Client {
	w := web.New()
	w.MinInterval = 0
	w.BaseBackoff = time.Millisecond
	w.MaxRetries = 1
	return w
}

func TestScan(t *testing.T) {
	srv, requests := fakeSite(t, nil)
	jobs, err := New(srv.URL, fastClient(), Criteria).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]store.NewJob{}
	for _, j := range jobs {
		byID[j.PostingID] = j
	}
	// Fixture has 15 positions over 2 pages; see testdata/search.json.
	want := []string{"JR1001", "JR1002", "JR1003", "JR1015"}
	if len(jobs) != len(want) {
		t.Errorf("got %d jobs %v, want %v", len(jobs), keys(byID), want)
	}
	for _, id := range want {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing %s; got %v", id, keys(byID))
		}
	}

	j := byID["JR1001"]
	if j.CompanyName != "Netflix" || j.JobTitle != "Software Engineer 5 - Example Platform" ||
		j.Link != "https://explore.jobs.netflix.net/careers/job/1001" {
		t.Errorf("JR1001 fields: %+v", j)
	}
	if j.Location == nil || *j.Location != "Los Gatos, California, USA" {
		t.Errorf("JR1001 location: %v", deref(j.Location))
	}
	if j.SalaryRange == nil || *j.SalaryRange != "$300,000 – $500,000" {
		t.Errorf("JR1001 salary: %v", deref(j.SalaryRange))
	}
	if j.PostedDate == nil || j.PostedDate.Format("2006-01-02") != "2026-11-19" {
		t.Errorf("JR1001 posted: %v", j.PostedDate)
	}
	if byID["JR1002"].SalaryRange != nil {
		t.Errorf("JR1002 has no salary in its description but got %v", deref(byID["JR1002"].SalaryRange))
	}

	// 2 search pages + one detail request per candidate passing the listing checks (7).
	if got := requests.Load(); got != 9 {
		t.Errorf("made %d requests, want 9 (details should only be fetched for listing-level survivors)", got)
	}
}

func TestScanPartialFailure(t *testing.T) {
	srv, _ := fakeSite(t, map[int64]bool{1002: true})
	jobs, err := New(srv.URL, fastClient(), Criteria).Scan(context.Background())
	if err == nil || !strings.Contains(err.Error(), "1 of 7 postings couldn't be checked") || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want the 3 that could be checked", len(jobs))
	}
}

func TestScanSiteChanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jobs": []}`)) // no "count"/"positions"
	}))
	defer srv.Close()
	_, err := New(srv.URL, fastClient(), Criteria).Scan(context.Background())
	if err == nil || !strings.Contains(err.Error(), "API may have changed") {
		t.Errorf("err = %v", err)
	}
}

func keys(m map[string]store.NewJob) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
