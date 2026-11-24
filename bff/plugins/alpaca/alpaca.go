// Package alpaca scans Alpaca's careers page (https://alpaca.markets/hiring),
// whose postings come from Greenhouse (board "alpaca"). A single request
// returns every posting with its description.
//
// Alpaca is remote-first and has no workplace-type field: the region is only
// in the free-text location ("Remote - Americas", "Global Anywhere", "Remote -
// Europe, Americas"), so the region rules below work on that text.
//
// To change which postings are kept, edit Criteria below. To remove this
// plugin, delete this folder and regenerate plugins/all (the Docker build does
// it automatically).
package alpaca

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"jobapps/bff/plugins"
	"jobapps/bff/plugins/shared/greenhouse"
	"jobapps/bff/plugins/shared/jobtext"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
	"jobapps/bff/store"
)

// Criteria decides which postings are kept. All checks must pass.
var Criteria = Filter{
	// Title must start with one of these (case-insensitive, whole words). A
	// team usually follows, e.g. "Senior Software Engineer - Ledger".
	TitlePrefixes: []string{
		"Software Engineer",
		"Senior Software Engineer",
		"Software Developer",
		"Senior Software Developer",
		"Full Stack Engineer",
		"Senior Full Stack Engineer",
		"Full-Stack Engineer", // Alpaca's spelling
		"Senior Full-Stack Engineer",
		"Full Stack Software Engineer",
		"Senior Full Stack Software Engineer",
		// Product-focused full-stack roles, e.g. "Senior Product Engineer - Growth".
		"Product Engineer",
		"Senior Product Engineer",
	},
	// Title must not contain any of these words or phrases (case-insensitive,
	// whole words: "Intern" doesn't match "Internal").
	TitleExcludes: []string{"Staff", "Principal", "Manager", "Security", "Intern", "in Test", "Quality"},

	// The location must name one of these (whole words, ignoring case). If the
	// location is just "Remote", the posting's offices are checked instead.
	Regions: []string{"North America", "Americas", "America", "Global", "Anywhere", "United States", "USA", "US"},
	// Removed from the text before matching Regions, so "South America"
	// doesn't count as "America".
	NotRegions: []string{"South America", "Latin America", "Central America"},
	// A title naming one of these is tied to that country or region and is
	// dropped, unless the title also names one of Regions ("US or Canada").
	TitleRegionExcludes: []string{"Canada", "APAC", "Asia", "EMEA", "Europe", "EU", "UK", "Japan", "Singapore", "SEA", "India", "Brazil", "LATAM"},

	// The location or an office must say one of these.
	RemoteWords: []string{"Remote", "Anywhere"},

	// Drop postings asking for more years of experience than this. Kept when
	// the description doesn't say.
	MaxYearsRequired: 6,
}

const (
	companyName = "Alpaca"
	board       = "alpaca"
)

func init() {
	plugins.Register("alpaca", New(greenhouse.DefaultBaseURL, web.New(), Criteria))
}

// Plugin scans Alpaca postings.
type Plugin struct {
	client *greenhouse.Client
	filter Filter
}

func New(base string, w *web.Client, f Filter) *Plugin {
	return &Plugin{client: &greenhouse.Client{BaseURL: base, Board: board, Web: w}, filter: f}
}

func (p *Plugin) Name() string { return companyName }

func (p *Plugin) Scan(ctx context.Context) ([]store.NewJob, error) {
	log := scan.Logger(ctx)

	all, err := p.client.Jobs(ctx)
	if err != nil {
		return nil, err
	}

	// Saved postings missing from the full (unfiltered) list have come down.
	openIDs := make([]string, len(all))
	for i, j := range all {
		openIDs[i] = j.PostingID()
	}
	_, expireErr := scan.ExpireMissing(ctx, companyName, openIDs)

	var jobs []store.NewJob
	for _, j := range all {
		text := j.Text()
		if reason := p.filter.Check(j, text); reason != "" {
			log.Debug("rejected", "title", j.Title, "location", j.Location.Name, "reason", reason)
			continue
		}
		if strings.TrimSpace(text) == "" {
			log.Warn("posting has an empty description; can't check years of experience", "title", j.Title)
		}
		jobs = append(jobs, toJob(j, text))
		log.Debug("kept", "title", j.Title, "location", j.Location.Name)
	}
	log.Info("filtered postings", "postings", len(all), "kept", len(jobs))
	return jobs, expireErr
}

func toJob(j greenhouse.Job, text string) store.NewJob {
	out := store.NewJob{
		CompanyName: companyName,
		JobTitle:    clean(j.Title),
		PostingID:   j.PostingID(),
		Link:        j.AbsoluteURL,
	}
	if out.Link == "" {
		out.Link = fmt.Sprintf("https://job-boards.greenhouse.io/alpaca/jobs/%d", j.ID)
	}
	if t, ok := j.Published(); ok {
		d := t.UTC()
		out.PostedDate = &d
	}
	if loc := clean(j.Location.Name); loc != "" {
		out.Location = &loc
	}
	if s := jobtext.SalaryRange(text); s != "" {
		out.SalaryRange = &s
	}
	return out
}

// clean collapses runs of whitespace: "Remote -  Americas " -> "Remote - Americas".
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// Filter holds the rules in Criteria.
type Filter struct {
	TitlePrefixes       []string
	TitleExcludes       []string
	Regions             []string
	NotRegions          []string
	TitleRegionExcludes []string
	RemoteWords         []string
	MaxYearsRequired    int
}

// Check returns why the posting was rejected, or "" to keep it. text is the
// description as plain text.
func (f Filter) Check(j greenhouse.Job, text string) string {
	title := clean(j.Title)
	location := clean(j.Location.Name)

	if jobtext.StartsWithAnyWordFold(title, f.TitlePrefixes...) == "" {
		return "title doesn't start with an allowed title"
	}
	if bad := jobtext.ContainsAnyWordFold(title, f.TitleExcludes...); bad != "" {
		return fmt.Sprintf("title contains %q", bad)
	}
	if r := jobtext.ContainsAnyWordFold(title, f.TitleRegionExcludes...); r != "" && f.region(title) == "" {
		return fmt.Sprintf("title is tied to %s", r)
	}

	if len(f.RemoteWords) > 0 {
		remote := false
		for _, p := range j.Places() {
			if jobtext.ContainsAnyWordFold(p, f.RemoteWords...) != "" {
				remote = true
				break
			}
		}
		if !remote {
			return fmt.Sprintf("location %q isn't remote", location)
		}
	}

	if len(f.Regions) > 0 && f.region(location) == "" {
		// "Remote" alone says nothing about where; the offices list does.
		if !onlyRemote.MatchString(location) {
			return fmt.Sprintf("location %q isn't in %s", location, strings.Join(f.Regions, ", "))
		}
		found := false
		for _, o := range j.Offices {
			if f.region(o.Name) != "" || f.region(o.Location) != "" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Sprintf("location is %q and no office is in %s", location, strings.Join(f.Regions, ", "))
		}
	}

	if f.MaxYearsRequired > 0 {
		if years, sentence, ok := jobtext.YearsRequiredWithSource(text); ok && years > f.MaxYearsRequired {
			if len(sentence) > 160 {
				sentence = sentence[:160] + "…"
			}
			return fmt.Sprintf("asks for %d+ years of experience: %q", years, sentence)
		}
	}
	return ""
}

// onlyRemote matches a location that says nothing but "Remote".
var onlyRemote = regexp.MustCompile(`(?i)^\W*remote\W*$`)

// region returns the first of Regions that s names, ignoring NotRegions, or "".
func (f Filter) region(s string) string {
	for _, nr := range f.NotRegions {
		s = regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(nr)+`\b`).ReplaceAllString(s, " ")
	}
	return jobtext.ContainsAnyWordFold(s, f.Regions...)
}
