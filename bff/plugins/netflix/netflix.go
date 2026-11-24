// Package netflix scans Netflix's careers site (https://explore.jobs.netflix.net/careers),
// which is hosted on Eightfold.
//
// To change which postings are kept, edit Criteria below. To remove this
// plugin, delete this folder and regenerate plugins/all (the Docker build does
// it automatically).
package netflix

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"jobapps/bff/plugins"
	"jobapps/bff/plugins/shared/eightfold"
	"jobapps/bff/plugins/shared/jobtext"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
	"jobapps/bff/store"
)

// Criteria decides which postings are kept. All checks must pass.
var Criteria = Filter{
	// Sent to the site's search; everything after this is filtered here.
	SearchQuery: "Software Engineer",

	// Title must start with one of these (case-insensitive). A level or team
	// may follow, e.g. "Software Engineer 5 - Payments".
	TitlePrefixes: []string{
		"Software Engineer",
		"Senior Software Engineer",
		"Software Developer",
		"Senior Software Developer",
		"Full Stack Software Engineer",
		// Netflix sometimes writes "Software Engineering 5 - Ads Finance".
		"Software Engineering",
	},
	// Title must not contain any of these words or phrases (case-insensitive,
	// whole words: "Intern" doesn't match "Internal").
	TitleExcludes: []string{"in Test", "Staff", "Principal", "Intern", "Manager"},

	// Netflix puts levels in titles: "Software Engineer 5", "(L5)", "(4/5)".
	// 4 is mid-level, 5 senior, 6 roughly staff, 7 principal. A posting is kept
	// if its lowest listed level is at most MaxLevel, or if it lists none.
	MaxLevel: 5,

	// At least one of the posting's locations must contain one of these.
	Countries: []string{"United States"},

	// Drop postings whose work arrangement is labeled as any of these.
	// Spelling variants match ("on-site", "On Site", "onsite"). Unlabeled
	// postings are kept. The full posting's work type is trusted over the
	// search results' label, which Netflix sometimes gets wrong (e.g. "onsite"
	// for a "Remote, United States" role).
	ExcludeWorkTypes: []string{"Hybrid", "Onsite"},

	// Drop postings asking for more years of experience than this. Kept when
	// the description doesn't say.
	MaxYearsRequired: 6,
}

const (
	companyName = "Netflix"
	baseURL     = "https://explore.jobs.netflix.net"
	domain      = "netflix.com"
)

func init() {
	w := web.New()
	// The site sends no rate-limit headers. Its search API answers 429 at
	// around 6-7 requests a minute (3s and 8s spacing both hit it), while
	// fetching single postings at 3s has been fine. So page through search
	// slowly, and back off for a while if the site still pushes back.
	w.MinInterval = 3 * time.Second
	w.BaseBackoff = 20 * time.Second
	w.MaxRetries = 5
	p := New(baseURL, w, Criteria)
	p.client.SearchInterval = 12 * time.Second
	plugins.Register("netflix", p)
}

// Plugin scans Netflix postings.
type Plugin struct {
	client *eightfold.Client
	filter Filter
}

func New(base string, w *web.Client, f Filter) *Plugin {
	return &Plugin{client: &eightfold.Client{BaseURL: base, Domain: domain, Web: w}, filter: f}
}

func (p *Plugin) Name() string { return companyName }

// Timeout allows for the slow pacing the site needs.
func (p *Plugin) Timeout() time.Duration { return 15 * time.Minute }

func (p *Plugin) Scan(ctx context.Context) ([]store.NewJob, error) {
	log := scan.Logger(ctx)

	positions, searchErr := p.client.Search(ctx, p.filter.SearchQuery)
	if searchErr != nil && len(positions) == 0 {
		return nil, searchErr
	}

	// Saved postings missing from the full (unfiltered) search results have
	// come down. Only trust the results if the search finished.
	var expireErr error
	if searchErr == nil {
		openIDs := make([]string, len(positions))
		for i, pos := range positions {
			openIDs[i] = pos.PostingID()
		}
		_, expireErr = scan.ExpireMissing(ctx, companyName, openIDs)
	} else {
		log.Warn("skipping expiration check: search didn't finish")
	}

	// First pass on search results: no extra requests needed.
	var candidates []eightfold.Position
	for _, pos := range positions {
		if reason := p.filter.CheckListing(pos); reason != "" {
			log.Debug("rejected", "title", pos.Name, "location", pos.Location, "reason", reason)
			continue
		}
		candidates = append(candidates, pos)
	}
	log.Info("filtered search results", "results", len(positions), "candidates", len(candidates))

	// Second pass needs each candidate's full description.
	var jobs []store.NewJob
	var detailErrs []error
	for _, c := range candidates {
		if ctx.Err() != nil {
			detailErrs = append(detailErrs, ctx.Err())
			break
		}
		full, err := p.client.Position(ctx, c.ID)
		if err != nil {
			log.Warn("couldn't fetch posting details; skipping it this scan", "title", c.Name, "err", err)
			detailErrs = append(detailErrs, err)
			continue
		}
		text := jobtext.HTMLToText(full.JobDescription)
		if strings.TrimSpace(text) == "" {
			log.Warn("posting has an empty description; can't check years of experience", "title", c.Name)
		}
		if reason := p.filter.CheckDetails(full, text); reason != "" {
			log.Debug("rejected", "title", c.Name, "location", c.Location, "reason", reason)
			continue
		}
		jobs = append(jobs, toJob(c, full, text))
		log.Debug("kept", "title", c.Name, "location", c.Location)
	}

	err := expireErr
	if searchErr != nil {
		err = errors.Join(err, fmt.Errorf("search stopped early, so some postings weren't checked: %w", searchErr))
	}
	if len(detailErrs) > 0 {
		err = errors.Join(err, fmt.Errorf("%d of %d postings couldn't be checked: %w", len(detailErrs), len(candidates), detailErrs[0]))
	}
	return jobs, err
}

func toJob(listing, full eightfold.Position, text string) store.NewJob {
	j := store.NewJob{
		CompanyName: companyName,
		JobTitle:    strings.TrimSpace(listing.Name),
		PostingID:   listing.PostingID(),
		Link:        listing.CanonicalURL,
	}
	if j.Link == "" {
		j.Link = fmt.Sprintf("%s/careers/job/%d", baseURL, listing.ID)
	}
	if listing.TCreate > 0 {
		d := time.Unix(listing.TCreate, 0).UTC()
		j.PostedDate = &d
	}
	if loc := formatLocations(listing); loc != "" {
		j.Location = &loc
	}
	if s := jobtext.SalaryRange(text); s != "" {
		j.SalaryRange = &s
	}
	return j
}

// formatLocations turns "Los Gatos,California,United States of America" into
// "Los Gatos, California, USA", joining several locations with " / ".
func formatLocations(p eightfold.Position) string {
	locs := p.Locations
	if len(locs) == 0 && p.Location != "" {
		locs = []string{p.Location}
	}
	out := make([]string, 0, len(locs))
	for _, l := range locs {
		parts := strings.Split(l, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		s := strings.Join(parts, ", ")
		s = strings.ReplaceAll(s, "United States of America", "USA")
		out = append(out, s)
	}
	return strings.Join(out, " / ")
}

// Filter holds the rules in Criteria.
type Filter struct {
	SearchQuery      string
	TitlePrefixes    []string
	TitleExcludes    []string
	MaxLevel         int
	Countries        []string
	ExcludeWorkTypes []string
	MaxYearsRequired int
}

// CheckListing applies the rules that only need search-result data. It returns
// why the posting was rejected, or "" to keep it.
func (f Filter) CheckListing(p eightfold.Position) string {
	title := strings.TrimSpace(p.Name)

	prefix := jobtext.StartsWithAnyWordFold(title, f.TitlePrefixes...)
	if prefix == "" {
		return "title doesn't start with an allowed title"
	}
	if bad := jobtext.ContainsAnyWordFold(title, f.TitleExcludes...); bad != "" {
		return fmt.Sprintf("title contains %q", bad)
	}
	if f.MaxLevel > 0 {
		if lvl, ok := lowestLevel(title, prefix); ok && lvl > f.MaxLevel {
			return fmt.Sprintf("level %d is above %d", lvl, f.MaxLevel)
		}
	}

	if len(f.Countries) > 0 {
		locs := append([]string{p.Location}, p.Locations...)
		match := false
		for _, l := range locs {
			if jobtext.ContainsAnyFold(l, f.Countries...) != "" {
				match = true
				break
			}
		}
		if !match {
			return fmt.Sprintf("location %q isn't in %s", p.Location, strings.Join(f.Countries, ", "))
		}
	}

	if wt := f.excludedWorkType(p.Location); wt != "" { // e.g. "New York (Hybrid)"
		return "labeled " + wt
	}
	// The search label is unreliable when it contradicts a remote location;
	// leave those for CheckDetails, which sees the full posting's work type.
	if !jobtext.ContainsFold(p.Location, "remote") {
		if wt := f.excludedWorkType(p.WorkLocationOption); wt != "" {
			return "labeled " + wt
		}
	}
	return ""
}

// CheckDetails applies the rules that need the full posting. text is the
// description as plain text.
func (f Filter) CheckDetails(p eightfold.Position, text string) string {
	label := p.WorkType() // authoritative when present
	if label == "" {
		label = p.WorkLocationOption
	}
	if wt := f.excludedWorkType(label); wt != "" {
		return "labeled " + wt
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

// excludedWorkType returns the first ExcludeWorkTypes entry found in any of
// the labels, or "". Labels are compared ignoring case, spaces, hyphens and
// underscores, so "On-site", "on_site" and "onsite" all match "Onsite".
func (f Filter) excludedWorkType(labels ...string) string {
	for _, l := range labels {
		nl := normalizeWorkType(l)
		if nl == "" {
			continue
		}
		for _, wt := range f.ExcludeWorkTypes {
			if nw := normalizeWorkType(wt); nw != "" && strings.Contains(nl, nw) {
				return wt
			}
		}
	}
	return ""
}

func normalizeWorkType(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(s))
}

var (
	// "(L5)" or "L5" anywhere in the title.
	levelL = regexp.MustCompile(`(?i)\bL([1-9])\b`)
	// Right after the title prefix: "5", "(5)", "(4/5)", "4/5".
	levelAfterPrefix = regexp.MustCompile(`^\s*\(?\s*([1-9])(?:\s*/\s*([1-9]))?\s*\)?(?:\s|,|-|–|$)`)
)

// lowestLevel finds the Netflix level(s) in a title and returns the lowest.
func lowestLevel(title, prefix string) (int, bool) {
	var levels []int
	if m := levelAfterPrefix.FindStringSubmatch(title[len(prefix):]); m != nil {
		for _, s := range m[1:] {
			if n, err := strconv.Atoi(s); err == nil {
				levels = append(levels, n)
			}
		}
	}
	for _, m := range levelL.FindAllStringSubmatch(title, -1) {
		n, _ := strconv.Atoi(m[1])
		levels = append(levels, n)
	}
	if len(levels) == 0 {
		return 0, false
	}
	lowest := levels[0]
	for _, n := range levels[1:] {
		lowest = min(lowest, n)
	}
	return lowest, true
}
