package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fast() *Client {
	c := New()
	c.MinInterval = 0
	c.BaseBackoff = time.Millisecond
	c.MaxRetries = 3
	return c
}

func TestCertificateErrorsAreNotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()

	// Default client doesn't trust the test server's self-signed certificate.
	_, err := fast().Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("err = %v, want a certificate error", err)
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Errorf("certificate error was retried: %v", err)
	}
}

func TestRetriesRateLimitThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var v struct{ OK bool }
	if err := fast().GetJSON(context.Background(), srv.URL, &v); err != nil || !v.OK {
		t.Fatalf("err=%v v=%+v", err, v)
	}
	if hits.Load() != 3 {
		t.Errorf("hits = %d, want 3", hits.Load())
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fast().Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "404") || hits.Load() != 1 {
		t.Errorf("err=%v hits=%d, want one 404 with no retries", err, hits.Load())
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := fast().Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "after 4 attempts") {
		t.Errorf("err = %v", err)
	}
	if hits.Load() != 4 {
		t.Errorf("hits = %d, want 4 (1 + 3 retries)", hits.Load())
	}
}

func TestIntervalCountsFromEndOfPreviousRequest(t *testing.T) {
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		times = append(times, time.Now())
		time.Sleep(150 * time.Millisecond) // slow response, longer than the interval
	}))
	defer srv.Close()

	c := fast()
	c.MinInterval = 100 * time.Millisecond
	for range 2 {
		if _, err := c.Get(context.Background(), srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	// Second request must start at least 150ms (response) + 100ms (interval) after the first.
	if gap := times[1].Sub(times[0]); gap < 250*time.Millisecond {
		t.Errorf("gap between request starts = %s, want >= 250ms", gap)
	}
}
