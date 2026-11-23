package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	graphql "github.com/graph-gophers/graphql-go"
	_ "github.com/lib/pq"

	"jobapps/bff/scan"
	"jobapps/bff/store"
)

// newTestSchema runs the db migrations into a throwaway Postgres schema and
// returns a GraphQL schema and store backed by it. The throwaway schema is dropped when
// the test ends, so existing data in the database is never touched.
//
// Set TEST_DATABASE_URL to run, e.g.
// postgres://jobapps:jobapps@localhost:5432/jobapps?sslmode=disable
func newTestSchema(t *testing.T) (*graphql.Schema, *store.Store) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })

	pgSchema := fmt.Sprintf("bff_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + pgSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP SCHEMA ` + pgSchema + ` CASCADE`) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", pgSchema)
	u.RawQuery = q.Encode()

	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	migrations, _ := filepath.Glob("../../db/migrations/V*.sql")
	sort.Strings(migrations)
	if len(migrations) == 0 {
		t.Fatal("no migrations found")
	}
	for _, m := range migrations {
		sqlText, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(sqlText)); err != nil {
			t.Fatalf("%s: %v", m, err)
		}
	}

	st := store.New(db)
	s, err := NewSchema(st, scan.NewManager(nil, st, scan.Options{}))
	if err != nil {
		t.Fatal(err)
	}
	return s, st
}

// exec runs a GraphQL operation and decodes its data into out. It fails the
// test on GraphQL errors unless wantErr is set, in which case it returns them.
func exec(t *testing.T, s *graphql.Schema, query string, vars map[string]any, out any) []string {
	t.Helper()
	resp := s.Exec(context.Background(), query, "", vars)
	var errs []string
	for _, e := range resp.Errors {
		errs = append(errs, e.Message)
	}
	if out != nil && resp.Data != nil {
		if err := json.Unmarshal(resp.Data, out); err != nil {
			t.Fatal(err)
		}
	}
	return errs
}

func mustExec(t *testing.T, s *graphql.Schema, query string, vars map[string]any, out any) {
	t.Helper()
	if errs := exec(t, s, query, vars, out); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func newJob(company, postingID, title string) store.NewJob {
	return store.NewJob{CompanyName: company, JobTitle: title, PostingID: postingID, Link: "https://example.com/" + postingID}
}

func withPosted(j store.NewJob, ymd string) store.NewJob {
	d, _ := time.Parse("2006-01-02", ymd)
	j.PostedDate = &d
	return j
}

// seed saves jobs the way a scan does and returns the ones inserted, as GraphQL IDs.
func seed(t *testing.T, st *store.Store, jobs ...store.NewJob) []string {
	t.Helper()
	inserted, err := st.InsertNew(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(inserted))
	for i, j := range inserted {
		ids[i] = strconv.FormatInt(j.ID, 10)
	}
	return ids
}

func TestInsertNewSkipsDuplicates(t *testing.T) {
	s, st := newTestSchema(t)

	first := withPosted(newJob("Acme", "A1", "Engineer"), "2026-11-20")
	salary, location, blank := "$150k–$190k", "Remote (US)", "  "
	first.SalaryRange, first.Location = &salary, &location
	second := newJob("Acme", "A2", "Designer")
	second.SalaryRange = &blank
	ids := seed(t, st,
		first,
		second,
		newJob("Acme", "A2", "Designer"), // repeated in batch
	)
	if len(ids) != 2 {
		t.Fatalf("first insert: got %d, want 2", len(ids))
	}

	var got struct {
		Job struct {
			Status, PostedDate    string
			SalaryRange, Location *string
		}
	}
	q := `query($id: ID!) { job(id: $id) { status postedDate salaryRange location } }`
	mustExec(t, s, q, map[string]any{"id": ids[0]}, &got)
	if got.Job.Status != "NEW" || got.Job.PostedDate != "2026-11-20" ||
		got.Job.SalaryRange == nil || *got.Job.SalaryRange != salary || got.Job.Location == nil || *got.Job.Location != location {
		t.Errorf("inserted job: got %+v", got.Job)
	}
	mustExec(t, s, q, map[string]any{"id": ids[1]}, &got)
	if got.Job.SalaryRange != nil || got.Job.Location != nil {
		t.Errorf("blank salary should be stored as null: got %+v", got.Job)
	}

	// Second scan: A1 already saved, A3 new.
	inserted, err := st.InsertNew(context.Background(), []store.NewJob{newJob("Acme", "A1", "Engineer"), newJob("Acme", "A3", "PM")})
	if err != nil || len(inserted) != 1 || inserted[0].PostingID != "A3" {
		t.Fatalf("second insert: got %+v, %v", inserted, err)
	}
}

func TestJobAndUpdateJob(t *testing.T) {
	s, st := newTestSchema(t)
	id := seed(t, st, withPosted(newJob("Acme", "A1", "Engineer"), "2026-11-20"))[0]

	type jobResp struct {
		ID, Status, JobTitle, RetrievedDate, LastUpdated string
		Notes, PostedDate, ContactName, ContactEmail     *string
		SalaryRange, Location                            *string
	}

	var got struct{ Job *jobResp }
	mustExec(t, s, `query($id: ID!) { job(id: $id) { id status jobTitle notes postedDate retrievedDate lastUpdated } }`,
		map[string]any{"id": id}, &got)
	if got.Job == nil || got.Job.ID != id {
		t.Fatalf("job: got %+v", got.Job)
	}

	mustExec(t, s, `{ job(id: "999999") { id } }`, nil, &got)
	if got.Job != nil {
		t.Errorf("missing job: expected null, got %+v", got.Job)
	}

	const update = `mutation($id: ID!, $input: UpdateJobInput!) {
		updateJob(id: $id, input: $input) {
			id status jobTitle notes postedDate retrievedDate lastUpdated contactName contactEmail salaryRange location
		}
	}`
	var up struct{ UpdateJob jobResp }

	// Set status and notes; jobTitle and postedDate untouched.
	mustExec(t, s, update, map[string]any{"id": id, "input": map[string]any{"status": "APPLIED", "notes": "sent resume v3"}}, &up)
	if up.UpdateJob.Status != "APPLIED" || up.UpdateJob.Notes == nil || *up.UpdateJob.Notes != "sent resume v3" ||
		up.UpdateJob.JobTitle != "Engineer" || up.UpdateJob.PostedDate == nil {
		t.Errorf("update 1: got %+v", up.UpdateJob)
	}

	// Contact fields.
	mustExec(t, s, update, map[string]any{"id": id, "input": map[string]any{"contactName": "Pat Lee", "contactEmail": "pat@example.com"}}, &up)
	if up.UpdateJob.ContactName == nil || *up.UpdateJob.ContactName != "Pat Lee" || up.UpdateJob.ContactEmail == nil {
		t.Errorf("contact update: got %+v", up.UpdateJob)
	}

	// Salary and location.
	mustExec(t, s, update, map[string]any{"id": id, "input": map[string]any{"salaryRange": "$200k", "location": "NYC"}}, &up)
	if up.UpdateJob.SalaryRange == nil || *up.UpdateJob.SalaryRange != "$200k" || up.UpdateJob.Location == nil || *up.UpdateJob.Location != "NYC" {
		t.Errorf("salary/location update: got %+v", up.UpdateJob)
	}
	mustExec(t, s, update, map[string]any{"id": id, "input": map[string]any{"salaryRange": nil}}, &up)
	if up.UpdateJob.SalaryRange != nil || up.UpdateJob.Location == nil {
		t.Errorf("clear salary: got %+v", up.UpdateJob)
	}

	// Explicit null or a blank string clears nullable fields.
	mustExec(t, s, update, map[string]any{"id": id, "input": map[string]any{"postedDate": nil, "notes": nil, "contactName": "  "}}, &up)
	if up.UpdateJob.PostedDate != nil || up.UpdateJob.Notes != nil || up.UpdateJob.ContactName != nil ||
		up.UpdateJob.ContactEmail == nil || up.UpdateJob.Status != "APPLIED" {
		t.Errorf("update 2: got %+v", up.UpdateJob)
	}

	if errs := exec(t, s, update, map[string]any{"id": "999999", "input": map[string]any{"status": "OFFER"}}, nil); len(errs) == 0 {
		t.Error("expected error updating missing job")
	}
	if errs := exec(t, s, update, map[string]any{"id": id, "input": map[string]any{"jobTitle": ""}}, nil); len(errs) == 0 {
		t.Error("expected error for empty jobTitle")
	}

	// Changing postingId to collide with another job is rejected.
	seed(t, st, newJob("Acme", "A2", "Other"))
	if errs := exec(t, s, update, map[string]any{"id": id, "input": map[string]any{"postingId": "A2"}}, nil); len(errs) == 0 {
		t.Error("expected duplicate error")
	}
}

func TestJobsListFilterSortPage(t *testing.T) {
	s, st := newTestSchema(t)

	// Insert 5 jobs, then give them distinct statuses.
	jobIDs := seed(t, st,
		newJob("Acme", "1", "Senior Engineer"),
		withPosted(newJob("Beta Co", "2", "Designer"), "2026-01-01"),
		newJob("acme labs", "3", "Staff Engineer"),
		withPosted(newJob("Gamma", "4", "Engineer_Intern"), "2026-06-01"),
		newJob("Delta", "5", "Recruiter Lead"), // "rui" would match r_i if _ were a wildcard
	)

	statuses := []string{"APPLIED", "USER_REJECTED", "NEW", "COMPANY_REJECTED", "INTERVIEWING"}
	for i, id := range jobIDs {
		mustExec(t, s, `mutation($id: ID!, $s: JobStatus!) { updateJob(id: $id, input: {status: $s}) { id } }`,
			map[string]any{"id": id, "s": statuses[i]}, nil)
	}

	type page struct {
		Jobs struct {
			TotalCount int
			Items      []struct{ PostingID, Status string }
		}
	}
	list := func(args string) page {
		t.Helper()
		var p page
		mustExec(t, s, `{ jobs`+args+` { totalCount items { postingId status } } }`, nil, &p)
		return p
	}
	ids := func(p page) string {
		out := ""
		for _, it := range p.Jobs.Items {
			out += it.PostingID
		}
		return out
	}

	if p := list(""); p.Jobs.TotalCount != 5 || len(p.Jobs.Items) != 5 {
		t.Errorf("no args: got %+v", p)
	}
	if got := ids(list(`(filter: {excludedStatuses: [USER_REJECTED, COMPANY_REJECTED]}, sort: {field: STATUS})`)); got != "315" {
		t.Errorf("excluded + status sort: got %q, want 315", got)
	}
	if got := ids(list(`(filter: {statuses: [APPLIED, NEW]}, sort: {field: COMPANY_NAME})`)); got != "13" {
		t.Errorf("statuses + company sort: got %q, want 13", got)
	}
	if p := list(`(filter: {statuses: []})`); p.Jobs.TotalCount != 0 {
		t.Errorf("empty statuses: got %+v", p)
	}
	if got := ids(list(`(filter: {companyName: "ACME"}, sort: {field: COMPANY_NAME, direction: DESC})`)); got != "31" {
		t.Errorf("companyName filter: got %q, want 31", got)
	}
	// "_" must match literally, not as a wildcard.
	if got := ids(list(`(filter: {jobTitle: "r_i"})`)); got != "4" {
		t.Errorf("jobTitle literal underscore: got %q, want 4", got)
	}
	// Nulls last both ways.
	if got := ids(list(`(sort: {field: POSTED_DATE, direction: DESC})`))[:2]; got != "42" {
		t.Errorf("posted desc: got %q, want 42...", got)
	}
	if got := ids(list(`(sort: {field: POSTED_DATE})`))[:2]; got != "24" {
		t.Errorf("posted asc: got %q, want 24...", got)
	}
	// Paging.
	if p := list(`(sort: {field: COMPANY_NAME}, limit: 2, offset: 2)`); ids(p) != "25" || p.Jobs.TotalCount != 5 {
		t.Errorf("paging: got %q total %d, want 25 total 5", ids(p), p.Jobs.TotalCount)
	}
	if errs := exec(t, s, `{ jobs(limit: 501) { totalCount } }`, nil, nil); len(errs) == 0 {
		t.Error("expected limit error")
	}
}
