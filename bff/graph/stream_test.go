package graph

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jobapps/bff/scan"
	"jobapps/bff/store"

	"github.com/graph-gophers/graphql-go/relay"
)

type slowPlugin struct {
	name string
	err  string
}

func (p slowPlugin) Name() string { return p.name }
func (p slowPlugin) Scan(ctx context.Context) ([]store.NewJob, error) {
	time.Sleep(20 * time.Millisecond)
	if p.err != "" {
		return nil, &scanErr{p.err}
	}
	return nil, nil
}

type scanErr struct{ msg string }

func (e *scanErr) Error() string { return e.msg }

// TestScanOverHTTP starts a scan through /graphql and follows it through
// /graphql/stream, the same way the frontend does. No database needed: the
// plugins return no jobs, so nothing is saved.
func TestScanOverHTTP(t *testing.T) {
	m := scan.NewManager([]scan.Plugin{slowPlugin{name: "Acme"}, slowPlugin{name: "Globex", err: "403 Forbidden"}}, nil, scan.Options{})
	schema, err := NewSchema(nil, m)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /graphql", &relay.Handler{Schema: schema})
	mux.Handle("POST /graphql/stream", StreamHandler(schema))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	post := func(path, query string, vars map[string]any) *http.Response {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
		res, err := http.Post(srv.URL+path, "application/json", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	type scanJSON struct {
		ID      string
		Running bool
		Plugins []struct {
			Name, Status string
			Error        *string
		}
	}

	res := post("/graphql", `mutation { startScan { id running plugins { name status error } } }`, nil)
	var started struct{ Data struct{ StartScan scanJSON } }
	json.NewDecoder(res.Body).Decode(&started)
	res.Body.Close()
	s := started.Data.StartScan
	if !s.Running || len(s.Plugins) != 2 || s.Plugins[0].Name != "Acme" || s.Plugins[0].Status == "" {
		t.Fatalf("startScan: %+v", s)
	}

	res = post("/graphql/stream", `subscription($id: ID!) { scanUpdated(id: $id) { id running plugins { name status error } } }`,
		map[string]any{"id": s.ID})
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}

	var events []scanJSON
	completed := false
	sc := bufio.NewScanner(res.Body)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: ") && event == "next":
			var msg struct {
				Data   struct{ ScanUpdated scanJSON }
				Errors []any
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &msg); err != nil || len(msg.Errors) > 0 {
				t.Fatalf("bad event %q: %v", line, err)
			}
			events = append(events, msg.Data.ScanUpdated)
		case event == "complete":
			completed = true
		}
	}
	if !completed || len(events) < 2 {
		t.Fatalf("completed=%v, %d events", completed, len(events))
	}
	last := events[len(events)-1]
	if last.Running || last.Plugins[0].Status != "SUCCEEDED" || last.Plugins[1].Status != "FAILED" ||
		last.Plugins[1].Error == nil || *last.Plugins[1].Error != "403 Forbidden" {
		t.Errorf("final event: %+v", last)
	}

	// Unknown scan id reports an error in the stream.
	res2 := post("/graphql/stream", `subscription { scanUpdated(id: "999") { id } }`, nil)
	defer res2.Body.Close()
	buf := new(strings.Builder)
	done := make(chan struct{})
	go func() { bufio.NewReader(res2.Body).WriteTo(buf); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream for unknown scan didn't end")
	}
	if !strings.Contains(buf.String(), "scan 999 not found") {
		t.Errorf("unknown scan: got %q", buf.String())
	}
}
