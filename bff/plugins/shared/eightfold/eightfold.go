// Package eightfold reads job postings from career sites hosted on Eightfold AI
// (URLs like https://<company>.jobs.<company>.net/careers or
// https://<company>.eightfold.ai/careers), via the JSON API the site itself uses.
package eightfold

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
)

// PageSize is what the API returns per page; it ignores larger values.
const PageSize = 10

// maxPages stops runaway pagination if the API misreports its count.
const maxPages = 200

type Client struct {
	// BaseURL is the career site origin, e.g. https://explore.jobs.netflix.net
	BaseURL string
	// Domain is the company domain the API expects, e.g. netflix.com
	Domain string
	Web    *web.Client
	// SearchInterval is an extra wait between search pages, for sites whose
	// search API is rate-limited harder than the rest. Zero means only Web's
	// own pacing applies.
	SearchInterval time.Duration
}

// Position is one posting as the API returns it. Only the fields plugins use
// are decoded.
type Position struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Location           string   `json:"location"`
	Locations          []string `json:"locations"`
	Department         string   `json:"department"`
	TCreate            int64    `json:"t_create"` // Unix seconds
	DisplayJobID       string   `json:"display_job_id"`
	ATSJobID           string   `json:"ats_job_id"`
	WorkLocationOption string   `json:"work_location_option"` // e.g. "onsite", "remote_local", "hybrid"
	CanonicalURL       string   `json:"canonicalPositionUrl"`
	// JobDescription is HTML. Empty in search results; filled by Position().
	JobDescription string `json:"job_description"`
	CustomJD       struct {
		DataFields struct {
			WorkType    []string `json:"work_type"`    // e.g. ["Remote"]
			PostingDate []string `json:"posting_date"` // e.g. ["11-19-2026"]
		} `json:"data_fields"`
	} `json:"custom_JD"`
}

// PostingID is the company's requisition ID, falling back to Eightfold's ID.
func (p Position) PostingID() string {
	for _, s := range []string{p.DisplayJobID, p.ATSJobID} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return fmt.Sprint(p.ID)
}

// WorkType is the work arrangement from the detail fields ("Remote", "Hybrid", ...), if any.
func (p Position) WorkType() string {
	return strings.Join(p.CustomJD.DataFields.WorkType, ", ")
}

type searchResponse struct {
	Count     *int       `json:"count"`
	Positions []Position `json:"positions"`
}

// Search returns every position matching query, paging through all results.
// If a page fails, it returns the positions fetched so far with the error.
func (c *Client) Search(ctx context.Context, query string) ([]Position, error) {
	log := scan.Logger(ctx)
	var all []Position
	seen := map[int64]bool{}

	for page := 0; page < maxPages; page++ {
		if page > 0 && c.SearchInterval > 0 {
			select {
			case <-ctx.Done():
				return all, fmt.Errorf("eightfold search page %d: %w", page+1, ctx.Err())
			case <-time.After(c.SearchInterval):
			}
		}
		q := url.Values{
			"domain":  {c.Domain},
			"query":   {query},
			"start":   {fmt.Sprint(page * PageSize)},
			"num":     {fmt.Sprint(PageSize)},
			"sort_by": {"timestamp"},
		}
		var res searchResponse
		if err := c.Web.GetJSON(ctx, c.BaseURL+"/api/apply/v2/jobs?"+q.Encode(), &res); err != nil {
			return all, fmt.Errorf("eightfold search page %d: %w", page+1, err)
		}
		if res.Count == nil {
			return all, fmt.Errorf("eightfold search page %d: response has no \"count\" field; the API may have changed", page+1)
		}
		if page == 0 {
			log.Info("eightfold search", "query", query, "count", *res.Count, "pages", (*res.Count+PageSize-1)/PageSize)
		}
		for _, p := range res.Positions {
			if !seen[p.ID] {
				seen[p.ID] = true
				all = append(all, p)
			}
		}
		if len(res.Positions) == 0 || (page+1)*PageSize >= *res.Count {
			return all, nil
		}
	}
	return all, fmt.Errorf("eightfold search: stopped after %d pages", maxPages)
}

// Position fetches one posting with its full description.
func (c *Client) Position(ctx context.Context, id int64) (Position, error) {
	q := url.Values{"domain": {c.Domain}}
	var p Position
	if err := c.Web.GetJSON(ctx, fmt.Sprintf("%s/api/apply/v2/jobs/%d?%s", c.BaseURL, id, q.Encode()), &p); err != nil {
		return p, fmt.Errorf("eightfold position %d: %w", id, err)
	}
	if p.ID == 0 {
		return p, fmt.Errorf("eightfold position %d: response has no \"id\"; the API may have changed", id)
	}
	return p, nil
}
