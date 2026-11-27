// Package monarchmoney scans Monarch Money's job board
// (https://jobs.ashbyhq.com/monarchmoney), hosted on Ashby. A single request
// returns every posting with its description and pay.
//
// To change which postings are kept, edit Criteria below. To remove this
// plugin, delete this folder and regenerate plugins/all (the Docker build does
// it automatically).
package monarchmoney

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"jobapps/bff/plugins"
	"jobapps/bff/plugins/shared/ashby"
	"jobapps/bff/plugins/shared/jobtext"
	"jobapps/bff/plugins/shared/web"
	"jobapps/bff/scan"
	"jobapps/bff/store"
)

// Criteria decides which postings are kept. All checks must pass.
var Criteria = Filter{
	// Title must start with one of these (case-insensitive, whole words). A
	// team usually follows, e.g. "Senior Software Engineer, Mobile".
	TitlePrefixes: []string{
		"Software Engineer",
		"Senior Software Engineer",
		"Software Developer",
		"Senior Software Developer",
		"Full Stack Engineer",
		"Senior Full Stack Engineer",
		"Full-Stack Engineer",
		"Senior Full-Stack Engineer",
		"Full Stack Software Engineer",
		"Senior Full Stack Software Engineer",
		"Product Engineer",
		"Senior Product Engineer",
		// Monarch's standing "apply for future roles" posting.
		"Future Engineering Opportunities",
	},
	// Title must not contain any of these words or phrases (case-insensitive,
	// whole words: "Intern" doesn't match "Internal").
	TitleExcludes: []string{"Staff", "Principal", "Manager", "Security", "Intern", "in Test", "Quality"},

	// Monarch lists several levels in one title, e.g. "(Senior/Staff/Principal)".
	// A combined level counts as its lowest level: if any of the listed levels
	// is one of these, the group is read as that level, so "Software Engineer
	// (Senior/Staff)" is kept; "(Staff/Principal)" is still excluded.
	AcceptableLevels: []string{"Junior", "Mid", "Mid-Level", "Intermediate", "Senior", "Sr"},

	// The posting's workplace type must be one of these ("Remote", "Hybrid",
	// "OnSite").
	WorkplaceTypes: []string{"Remote"},
	// One of the posting's locations must name one of these (whole words;
	// "non-US" doesn't count) or be just "Remote", which Monarch uses for
	// roles open anywhere it hires.
	Countries: []string{"US", "USA", "United States"},

	// Drop postings asking for more years of experience than this. Kept when
	// the description doesn't say.
	MaxYearsRequired: 6,
}

const (
	companyName = "Monarch Money"
	board       = "monarchmoney"
)

func init() {
	plugins.Register("monarchmoney", New(ashby.DefaultBaseURL, web.New(), Criteria))
}

// Plugin scans Monarch Money postings.
type Plugin struct {
	client *ashby.Client
	filter Filter
}

func New(base string, w *web.Client, f Filter) *Plugin {
	return &Plugin{client: &ashby.Client{BaseURL: base, Board: board, Web: w}, filter: f}
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
		openIDs[i] = j.ID
	}
	_, expireErr := scan.ExpireMissing(ctx, companyName, openIDs)

	var jobs []store.NewJob
	for _, j := range all {
		text := j.Text()
		if reason := p.filter.Check(j, text); reason != "" {
			log.Debug("rejected", "title", j.Title, "location", j.Location, "reason", reason)
			continue
		}
		if strings.TrimSpace(text) == "" {
			log.Warn("posting has an empty description; can't check years of experience", "title", j.Title)
		}
		jobs = append(jobs, toJob(j, text))
		log.Debug("kept", "title", j.Title, "location", j.Location)
	}
	log.Info("filtered postings", "postings", len(all), "kept", len(jobs))
	return jobs, expireErr
}

func toJob(j ashby.Job, text string) store.NewJob {
	out := store.NewJob{
		CompanyName: companyName,
		JobTitle:    clean(j.Title),
		PostingID:   j.ID,
		Link:        j.JobURL,
	}
	if out.Link == "" {
		out.Link = "https://jobs.ashbyhq.com/" + board + "/" + j.ID
	}
	if t, ok := j.Published(); ok {
		d := t.UTC()
		out.PostedDate = &d
	}
	if loc := clean(j.Location); loc != "" {
		out.Location = &loc
	}
	salary := j.SalaryRange("USD")
	if salary == "" {
		salary = jobtext.SalaryRange(text)
	}
	if salary != "" {
		out.SalaryRange = &salary
	}
	return out
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// Filter holds the rules in Criteria.
type Filter struct {
	TitlePrefixes    []string
	TitleExcludes    []string
	AcceptableLevels []string
	WorkplaceTypes   []string
	Countries        []string
	MaxYearsRequired int
}

// Check returns why the posting was rejected, or "" to keep it. text is the
// description as plain text.
func (f Filter) Check(j ashby.Job, text string) string {
	title := f.collapseLevels(clean(j.Title))

	if jobtext.StartsWithAnyWordFold(title, f.TitlePrefixes...) == "" {
		return "title doesn't start with an allowed title"
	}
	if bad := jobtext.ContainsAnyWordFold(title, f.TitleExcludes...); bad != "" {
		return fmt.Sprintf("title contains %q", bad)
	}

	if len(f.WorkplaceTypes) > 0 {
		wt := j.WorkplaceType
		if wt == "" && j.IsRemote {
			wt = "Remote"
		}
		if !slices.ContainsFunc(f.WorkplaceTypes, func(w string) bool { return normalize(w) == normalize(wt) }) {
			if wt == "" {
				return "no workplace type listed"
			}
			return "workplace type is " + wt
		}
	}

	if len(f.Countries) > 0 {
		ok := false
		for _, l := range j.Locations() {
			l = clean(l)
			if onlyRemote.MatchString(l) || jobtext.ContainsAnyWordFold(nonUS.ReplaceAllString(l, " "), f.Countries...) != "" {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Sprintf("locations %q aren't in %s", strings.Join(j.Locations(), "; "), strings.Join(f.Countries, ", "))
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

var (
	// A location that says nothing but "Remote".
	onlyRemote = regexp.MustCompile(`(?i)^\W*remote\W*$`)
	// "non-US", "Non US", "outside the US" don't mean the US.
	nonUS = regexp.MustCompile(`(?i)\b(non[\s-]*|outside( of)?( the)?\s+)(US|USA|United States)\b`)
	// Two or more slash-separated words, optionally in parentheses:
	// "(Senior/Staff/Principal)", "Senior/Staff".
	levelGroup = regexp.MustCompile(`\(?\s*[A-Za-z][A-Za-z-]*(?:\s*/\s*[A-Za-z][A-Za-z-]*)+\s*\)?`)
)

// knownLevels are the words collapseLevels treats as levels. A slash group
// with any other word (e.g. "iOS/Android") is left alone.
var knownLevels = []string{"junior", "jr", "mid", "mid-level", "intermediate", "senior", "sr", "staff", "principal", "lead", "distinguished"}

// collapseLevels rewrites a slash group of levels as its acceptable level, if
// it has one: "Software Engineer (Senior/Staff)" -> "Software Engineer Senior",
// "Senior/Staff Software Engineer" -> "Senior Software Engineer".
func (f Filter) collapseLevels(title string) string {
	out := levelGroup.ReplaceAllStringFunc(title, func(g string) string {
		parts := strings.Split(strings.Trim(g, "() "), "/")
		accept := ""
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if !slices.Contains(knownLevels, strings.ToLower(p)) {
				return g
			}
			if accept == "" && slices.ContainsFunc(f.AcceptableLevels, func(a string) bool { return strings.EqualFold(a, p) }) {
				accept = p
			}
		}
		if accept == "" {
			return g
		}
		return " " + accept + " "
	})
	return clean(out)
}

func normalize(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(s))
}
