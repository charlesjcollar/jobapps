// Package airbnb scans Airbnb's careers site (https://careers.airbnb.com/),
// whose postings come from Greenhouse (board "airbnb"). A single request
// returns every posting with its description.
//
// To change which postings are kept, edit Criteria below. To remove this
// plugin, delete this folder and regenerate plugins/all (the Docker build does
// it automatically).
package airbnb

import (
	"context"
	"fmt"
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
	// team usually follows, e.g. "Senior Software Engineer, Payments".
	TitlePrefixes: []string{
		"Software Engineer",
		"Senior Software Engineer",
		"Software Developer",
		"Senior Software Developer",
		"Full Stack Software Engineer",
		"Senior Full Stack Software Engineer",
		"Full Stack Engineer",
		"Senior Full Stack Engineer",
		"Fullstack Engineer",
		"Senior Fullstack Engineer",
	},
	// Title must not contain any of these words or phrases (case-insensitive,
	// whole words: "Intern" doesn't match "Internal").
	TitleExcludes: []string{"Staff", "Principal", "Manager", "Security", "Intern", "in Test"},

	// The posting's location or one of its offices must name one of these
	// (whole words, so "US" doesn't match "Campus").
	Countries: []string{"United States", "USA", "US"},

	// The posting's "Workplace Type" must be one of these. Airbnb's site shows
	// "Remote" as "Live and Work Anywhere"; the others are "Hybrid" and
	// "Onsite". Postings without a workplace type are dropped.
	WorkplaceTypes: []string{"Remote"},

	// Drop postings asking for more years of experience than this. Kept when
	// the description doesn't say.
	MaxYearsRequired: 6,
}

const (
	companyName = "Airbnb"
	board       = "airbnb"
	// workplaceField is the Greenhouse custom field holding Hybrid/Onsite/Remote.
	workplaceField = "Workplace Type"
)

func init() {
	plugins.Register("airbnb", New(greenhouse.DefaultBaseURL, web.New(), Criteria))
}

// Plugin scans Airbnb postings.
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

	// If no posting has the workplace field, Airbnb renamed or dropped it and
	// every posting would be rejected; fail loudly instead of finding nothing.
	if len(p.filter.WorkplaceTypes) > 0 && len(all) > 0 {
		found := false
		for _, j := range all {
			if j.HasField(workplaceField) {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("none of %d postings has a %q field; Airbnb may have renamed it", len(all), workplaceField)
		}
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
		JobTitle:    strings.Join(strings.Fields(j.Title), " "),
		PostingID:   j.PostingID(),
		Link:        j.AbsoluteURL,
	}
	if out.Link == "" {
		out.Link = fmt.Sprintf("https://careers.airbnb.com/positions/%d/", j.ID)
	}
	if t, ok := j.Published(); ok {
		d := t.UTC()
		out.PostedDate = &d
	}
	if loc := formatLocation(j); loc != "" {
		out.Location = &loc
	}
	if s := jobtext.SalaryRange(text); s != "" {
		out.SalaryRange = &s
	}
	return out
}

// formatLocation cleans up the location text ("Remote, USA  ") and notes the
// workplace type when the text doesn't, e.g. "San Francisco, CA (Remote)".
func formatLocation(j greenhouse.Job) string {
	loc := strings.Join(strings.Fields(j.Location.Name), " ")
	wt := j.Field(workplaceField)
	switch {
	case loc == "":
		return wt
	case wt != "" && !jobtext.ContainsFold(loc, wt):
		return fmt.Sprintf("%s (%s)", loc, wt)
	}
	return loc
}

// Filter holds the rules in Criteria.
type Filter struct {
	TitlePrefixes    []string
	TitleExcludes    []string
	Countries        []string
	WorkplaceTypes   []string
	MaxYearsRequired int
}

// Check returns why the posting was rejected, or "" to keep it. text is the
// description as plain text.
func (f Filter) Check(j greenhouse.Job, text string) string {
	title := strings.Join(strings.Fields(j.Title), " ")

	if !hasAnyWordPrefix(title, f.TitlePrefixes) {
		return "title doesn't start with an allowed title"
	}
	if bad := jobtext.ContainsAnyWordFold(title, f.TitleExcludes...); bad != "" {
		return fmt.Sprintf("title contains %q", bad)
	}

	if len(f.Countries) > 0 {
		match := false
		for _, p := range j.Places() {
			if jobtext.ContainsAnyWordFold(p, f.Countries...) != "" {
				match = true
				break
			}
		}
		if !match {
			return fmt.Sprintf("location %q isn't in %s", strings.TrimSpace(j.Location.Name), strings.Join(f.Countries, ", "))
		}
	}

	if len(f.WorkplaceTypes) > 0 {
		wt := j.Field(workplaceField)
		if wt == "" {
			return "no workplace type listed"
		}
		ok := false
		for _, want := range f.WorkplaceTypes {
			if normalize(wt) == normalize(want) {
				ok = true
				break
			}
		}
		if !ok {
			return "workplace type is " + wt
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

// normalize ignores case, spaces, hyphens and underscores: "On-site" == "onsite".
func normalize(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(s))
}

// hasAnyWordPrefix reports whether title starts with one of prefixes (ignoring
// case) as whole words: "Software Engineer, Payments" yes, "Software
// Engineering Intern" no.
func hasAnyWordPrefix(title string, prefixes []string) bool {
	for _, pre := range prefixes {
		if len(title) < len(pre) || !strings.EqualFold(title[:len(pre)], pre) {
			continue
		}
		if len(title) == len(pre) {
			return true
		}
		next := title[len(pre)]
		if !(next >= 'a' && next <= 'z' || next >= 'A' && next <= 'Z') {
			return true
		}
	}
	return false
}
