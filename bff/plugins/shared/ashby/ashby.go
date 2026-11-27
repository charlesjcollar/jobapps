// Package ashby reads job postings from Ashby job boards
// (https://jobs.ashbyhq.com/<board>) through Ashby's public posting API. One
// request returns every listed posting with its description and compensation.
package ashby

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"jobapps/bff/plugins/shared/jobtext"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
)

// DefaultBaseURL is the posting API origin.
const DefaultBaseURL = "https://api.ashbyhq.com"

type Client struct {
	// BaseURL defaults to DefaultBaseURL; tests point it at a local server.
	BaseURL string
	// Board is the name in the board's URL, e.g. "monarchmoney".
	Board string
	Web   *web.Client
}

// Job is one posting as the API returns it. Only the fields plugins use are
// decoded.
type Job struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Department         string `json:"department"`
	Team               string `json:"team"`
	EmploymentType     string `json:"employmentType"` // e.g. "FullTime"
	Location           string `json:"location"`       // e.g. "Remote (US)"
	SecondaryLocations []struct {
		Location string `json:"location"`
	} `json:"secondaryLocations"`
	IsListed        bool   `json:"isListed"`
	IsRemote        bool   `json:"isRemote"`
	WorkplaceType   string `json:"workplaceType"` // "Remote", "Hybrid", "OnSite"
	PublishedAt     string `json:"publishedAt"`   // RFC 3339
	JobURL          string `json:"jobUrl"`
	DescriptionHTML string `json:"descriptionHtml"`
	DescriptionText string `json:"descriptionPlain"`
	Compensation    *struct {
		SummaryComponents []Component `json:"summaryComponents"`
	} `json:"compensation"`
}

// Component is one part of a posting's pay, e.g. a salary range.
type Component struct {
	CompensationType string   `json:"compensationType"` // "Salary", "EquityCashValue", "Bonus", ...
	Interval         string   `json:"interval"`         // e.g. "1 YEAR"
	CurrencyCode     string   `json:"currencyCode"`
	MinValue         *float64 `json:"minValue"`
	MaxValue         *float64 `json:"maxValue"`
}

// Locations returns the primary location followed by any secondary ones.
func (j Job) Locations() []string {
	out := []string{j.Location}
	for _, l := range j.SecondaryLocations {
		out = append(out, l.Location)
	}
	return out
}

// Text returns the description as plain text.
func (j Job) Text() string {
	if strings.TrimSpace(j.DescriptionText) != "" {
		return j.DescriptionText
	}
	return jobtext.HTMLToText(j.DescriptionHTML)
}

// Published returns when the posting went up; ok is false if it doesn't parse.
func (j Job) Published() (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, j.PublishedAt)
	return t, err == nil
}

// SalaryRange returns the yearly salary range in the given currency, e.g.
// "$120,000 – $230,000" for "USD", or "" if the posting doesn't list one.
func (j Job) SalaryRange(currency string) string {
	if j.Compensation == nil {
		return ""
	}
	for _, c := range j.Compensation.SummaryComponents {
		if c.CompensationType != "Salary" || !strings.EqualFold(c.CurrencyCode, currency) ||
			c.Interval != "1 YEAR" || c.MinValue == nil || c.MaxValue == nil {
			continue
		}
		symbol := currency + " "
		if strings.EqualFold(currency, "USD") {
			symbol = "$"
		}
		return fmt.Sprintf("%s%s – %s%s", symbol, commas(int(*c.MinValue)), symbol, commas(int(*c.MaxValue)))
	}
	return ""
}

func commas(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

type boardResponse struct {
	Jobs *[]Job `json:"jobs"`
}

// Jobs returns every listed posting on the board, with descriptions and
// compensation.
func (c *Client) Jobs(ctx context.Context) ([]Job, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u := fmt.Sprintf("%s/posting-api/job-board/%s?includeCompensation=true", strings.TrimRight(base, "/"), url.PathEscape(c.Board))

	var res boardResponse
	if err := c.Web.GetJSON(ctx, u, &res); err != nil {
		return nil, fmt.Errorf("ashby board %q: %w", c.Board, err)
	}
	if res.Jobs == nil {
		return nil, fmt.Errorf("ashby board %q: response has no \"jobs\" field; the API may have changed", c.Board)
	}
	var listed []Job
	for _, j := range *res.Jobs {
		if j.IsListed {
			listed = append(listed, j)
		}
	}
	scan.Logger(ctx).Info("ashby board", "board", c.Board, "jobs", len(listed), "unlisted", len(*res.Jobs)-len(listed))
	return listed, nil
}
