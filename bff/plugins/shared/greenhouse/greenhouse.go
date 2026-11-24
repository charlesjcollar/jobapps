// Package greenhouse reads job postings from Greenhouse job boards through the
// public Job Board API (https://developers.greenhouse.io/job-board.html). One
// request returns every open posting on a board, descriptions included.
//
// A company's board token is the name in its Greenhouse URLs, e.g. "airbnb" in
// https://boards.greenhouse.io/airbnb or ...?gh_jid=123 links on its own site.
package greenhouse

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	"jobapps/bff/plugins/shared/jobtext"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
)

// DefaultBaseURL is the Job Board API origin.
const DefaultBaseURL = "https://boards-api.greenhouse.io"

type Client struct {
	// BaseURL defaults to DefaultBaseURL; tests point it at a local server.
	BaseURL string
	// Board is the company's board token, e.g. "airbnb".
	Board string
	Web   *web.Client
}

// Job is one posting as the API returns it. Only the fields plugins use are
// decoded.
type Job struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
	// Metadata holds the company's custom fields, e.g. "Workplace Type".
	Metadata []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	} `json:"metadata"`
	Offices []struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	} `json:"offices"`
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
	FirstPublished string `json:"first_published"` // RFC 3339
	UpdatedAt      string `json:"updated_at"`      // RFC 3339
	// Content is the description as HTML that has been HTML-escaped again
	// ("&lt;p&gt;..."). Use Text().
	Content string `json:"content"`
}

// Field returns the custom metadata field with this name (ignoring case) as
// text, or "" if the posting doesn't have it. List values are joined with ", ".
func (j Job) Field(name string) string {
	for _, m := range j.Metadata {
		if !strings.EqualFold(strings.TrimSpace(m.Name), name) {
			continue
		}
		switch v := m.Value.(type) {
		case nil:
			return ""
		case string:
			return strings.TrimSpace(v)
		case []any:
			parts := make([]string, 0, len(v))
			for _, x := range v {
				parts = append(parts, strings.TrimSpace(fmt.Sprint(x)))
			}
			return strings.Join(parts, ", ")
		default:
			return fmt.Sprint(v)
		}
	}
	return ""
}

// HasField reports whether the posting has a metadata field with this name.
func (j Job) HasField(name string) bool {
	for _, m := range j.Metadata {
		if strings.EqualFold(strings.TrimSpace(m.Name), name) {
			return true
		}
	}
	return false
}

// Places returns the location name followed by each office's name and
// location, for matching against countries.
func (j Job) Places() []string {
	out := []string{j.Location.Name}
	for _, o := range j.Offices {
		out = append(out, o.Name, o.Location)
	}
	return out
}

// Text returns the description as plain text.
func (j Job) Text() string {
	return jobtext.HTMLToText(html.UnescapeString(j.Content))
}

// Published returns when the posting first went up, falling back to its last
// update. ok is false if neither date parses.
func (j Job) Published() (t time.Time, ok bool) {
	for _, s := range []string{j.FirstPublished, j.UpdatedAt} {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// PostingID is Greenhouse's job ID, which is also what companies show in links.
func (j Job) PostingID() string { return strconv.FormatInt(j.ID, 10) }

type jobsResponse struct {
	Jobs *[]Job `json:"jobs"`
	Meta struct {
		Total *int `json:"total"`
	} `json:"meta"`
}

// Jobs returns every open posting on the board, with descriptions.
func (c *Client) Jobs(ctx context.Context) ([]Job, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u := fmt.Sprintf("%s/v1/boards/%s/jobs?content=true", strings.TrimRight(base, "/"), url.PathEscape(c.Board))

	var res jobsResponse
	if err := c.Web.GetJSON(ctx, u, &res); err != nil {
		return nil, fmt.Errorf("greenhouse board %q: %w", c.Board, err)
	}
	if res.Jobs == nil {
		return nil, fmt.Errorf("greenhouse board %q: response has no \"jobs\" field; the API may have changed", c.Board)
	}
	jobs := *res.Jobs
	if res.Meta.Total != nil && *res.Meta.Total != len(jobs) {
		scan.Logger(ctx).Warn("greenhouse returned a different number of jobs than it reported",
			"board", c.Board, "total", *res.Meta.Total, "returned", len(jobs))
	}
	scan.Logger(ctx).Info("greenhouse board", "board", c.Board, "jobs", len(jobs))
	return jobs, nil
}
