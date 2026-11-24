// Package web is the HTTP client plugins use to fetch job boards politely.
//
// Requests to the same host are spaced at least MinInterval apart across every
// plugin in the process, so many companies on one job-board system don't flood
// it. Rate-limit and server errors are retried with backoff, and a 429 pauses
// the whole host, not just the one request.
package web

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"jobapps/bff/scan"
)

const DefaultUserAgent = "Mozilla/5.0 (compatible; jobapps/1.0; personal job tracker)"

// Client fetches URLs with per-host pacing and retries. The zero value is not
// usable; use New and adjust fields before the first request.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// MinInterval is the minimum gap between one request to a host finishing
	// and the next one starting.
	MinInterval time.Duration
	// MaxRetries is how many times a retryable failure is retried.
	MaxRetries int
	// BaseBackoff is the first retry delay when the server doesn't send
	// Retry-After; it doubles each attempt, up to MaxBackoff.
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	// MaxBodyBytes caps response size.
	MaxBodyBytes int64
}

func New() *Client {
	return &Client{
		HTTP:         &http.Client{Timeout: 30 * time.Second},
		UserAgent:    DefaultUserAgent,
		MinInterval:  time.Second,
		MaxRetries:   4,
		BaseBackoff:  5 * time.Second,
		MaxBackoff:   2 * time.Minute,
		MaxBodyBytes: 10 << 20,
	}
}

// HTTPError is a non-2xx response.
type HTTPError struct {
	URL    string
	Status int
	Body   string // first part of the body, for debugging
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("GET %s: %d %s", e.URL, e.Status, http.StatusText(e.Status))
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

// GetJSON fetches url and decodes the JSON body into v.
func (c *Client) GetJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("GET %s: decoding JSON (the site may have changed): %w; body starts: %s", rawURL, err, snippet(body))
	}
	return nil
}

// Get fetches url and returns the body, retrying rate limits and server errors.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("bad URL %q: %w", rawURL, err)
	}
	h := hostFor(u.Host)
	log := scan.Logger(ctx)

	for attempt := 0; ; attempt++ {
		if err := h.wait(ctx, c.MinInterval); err != nil {
			return nil, err
		}
		start := time.Now()
		body, retryAfter, err := c.do(ctx, rawURL)
		h.finished(c.MinInterval)
		log.Debug("GET", "url", rawURL, "attempt", attempt+1, "took", time.Since(start).Round(time.Millisecond), "err", err)
		if err == nil {
			return body, nil
		}
		if !retryable(err) || attempt >= c.MaxRetries || ctx.Err() != nil {
			if attempt > 0 {
				return nil, fmt.Errorf("%w (after %d attempts)", err, attempt+1)
			}
			return nil, err
		}

		delay := retryAfter
		if delay <= 0 {
			delay = min(c.BaseBackoff<<attempt, c.MaxBackoff)
			delay += time.Duration(rand.Int64N(int64(delay)/4 + 1)) // jitter
		}
		var he *HTTPError
		if errors.As(err, &he) && he.Status == http.StatusTooManyRequests {
			h.pause(delay) // the whole host is rate-limiting us
		}
		log.Warn("retrying request", "url", rawURL, "err", err, "attempt", attempt+1, "delay", delay.Round(time.Second))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, fmt.Errorf("%w (gave up waiting to retry: %v)", err, ctx.Err())
		}
	}
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retryAfter time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json, text/html;q=0.9, */*;q=0.8")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer res.Body.Close()

	body, err = io.ReadAll(io.LimitReader(res.Body, c.MaxBodyBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("GET %s: reading body: %w", rawURL, err)
	}
	if int64(len(body)) > c.MaxBodyBytes {
		return nil, 0, fmt.Errorf("GET %s: response larger than %d bytes", rawURL, c.MaxBodyBytes)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, parseRetryAfter(res.Header.Get("Retry-After")), &HTTPError{URL: rawURL, Status: res.StatusCode, Body: snippet(body)}
	}
	return body, 0, nil
}

func retryable(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		switch he.Status {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout,
			http.StatusInternalServerError, http.StatusRequestTimeout:
			return true
		}
		return false
	}
	// Certificate problems won't fix themselves on retry.
	var certErr *tls.CertificateVerificationError
	var unknownCA x509.UnknownAuthorityError
	var badHost x509.HostnameError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &certErr) || errors.As(err, &unknownCA) || errors.As(err, &badHost) || errors.As(err, &invalid) {
		return false
	}
	var ne net.Error
	return errors.As(err, &ne) // timeouts, connection resets, DNS hiccups
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
		return min(time.Duration(secs)*time.Second, 10*time.Minute)
	}
	if t, err := http.ParseTime(v); err == nil {
		return min(max(time.Until(t), 0), 10*time.Minute)
	}
	return 0
}

func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// --- per-host pacing, shared by every client in the process ---

type host struct {
	mu   sync.Mutex
	next time.Time // earliest time the next request may start
}

var (
	hostsMu sync.Mutex
	hosts   = map[string]*host{}
)

func hostFor(name string) *host {
	hostsMu.Lock()
	defer hostsMu.Unlock()
	h, ok := hosts[name]
	if !ok {
		h = &host{}
		hosts[name] = h
	}
	return h
}

// finished records that a request just completed, so the next one waits a
// full interval from now, however long this one took.
func (h *host) finished(interval time.Duration) {
	h.pause(interval)
}

// wait blocks until this request's turn, reserving the slot after it.
func (h *host) wait(ctx context.Context, interval time.Duration) error {
	h.mu.Lock()
	now := time.Now()
	at := h.next
	if at.Before(now) {
		at = now
	}
	h.next = at.Add(interval)
	h.mu.Unlock()

	if d := time.Until(at); d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// pause pushes the host's next slot out by at least d.
func (h *host) pause(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t := time.Now().Add(d); t.After(h.next) {
		h.next = t
	}
}
