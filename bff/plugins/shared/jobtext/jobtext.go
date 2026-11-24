// Package jobtext has text helpers for filtering job postings.
package jobtext

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

var (
	blockTags = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|li|/h[1-6]|/tr|/ul|/ol)[^>]*>`)
	anyTag    = regexp.MustCompile(`<[^>]*>`)
	spaces    = regexp.MustCompile(`[ \t\x{00a0}]+`)
	newlines  = regexp.MustCompile(`\n\s*\n+`)
)

// HTMLToText converts an HTML job description to plain text, one line per
// paragraph or list item.
func HTMLToText(s string) string {
	s = blockTags.ReplaceAllString(s, "\n")
	s = anyTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spaces.ReplaceAllString(s, " ")
	s = newlines.ReplaceAllString(s, "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ContainsFold reports whether s contains substr, ignoring case.
func ContainsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// ContainsAnyFold returns the first of subs that s contains (ignoring case), or "".
func ContainsAnyFold(s string, subs ...string) string {
	lower := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return sub
		}
	}
	return ""
}

// ContainsAnyWordFold returns the first of words that appears in s as a whole
// word or phrase (ignoring case), or "". "Intern" matches "Software Engineer
// Intern" but not "Internal Tools".
func ContainsAnyWordFold(s string, words ...string) string {
	for _, w := range words {
		if w == "" {
			continue
		}
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `\b`).MatchString(s) {
			return w
		}
	}
	return ""
}

var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
	"nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
}

const num = `(\d{1,2}|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen)`

// Matches "7+ years", "7 or more years", "5-8 years", "5 to 8 years", "at least seven (7) years", ...
var yearsRe = regexp.MustCompile(`(?i)\b` + num + `(?:\s*\(\d{1,2}\))?\s*(?:\+|plus)?\s*(?:(?:-|–|—|to)\s*` + num + `\s*\+?\s*)?(?:or more\s+|\+\s*)?years?\b`)

// YearsRequired returns the highest minimum years of experience the text asks
// for, and false if it doesn't state one. Only sentences that mention
// "experience" count, and ranges count by their lower end ("5-8 years" is 5).
// "3+ years of Go and 8+ years of backend experience" returns 8.
func YearsRequired(text string) (int, bool) {
	n, _, ok := YearsRequiredWithSource(text)
	return n, ok
}

// YearsRequiredWithSource is YearsRequired plus the sentence the number came
// from, for logging why a posting was rejected.
func YearsRequiredWithSource(text string) (years int, sentence string, ok bool) {
	best, bestSentence, found := 0, "", false
	for _, sentence := range splitSentences(text) {
		if !strings.Contains(strings.ToLower(sentence), "experience") {
			continue
		}
		for _, m := range yearsRe.FindAllStringSubmatch(sentence, -1) {
			n, ok := parseNum(m[1])
			if !ok || n == 0 || n > 30 {
				continue
			}
			if !found || n > best {
				best, bestSentence, found = n, strings.TrimSpace(sentence), true
			}
		}
	}
	return best, bestSentence, found
}

func parseNum(s string) (int, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	n, ok := numberWords[strings.ToLower(s)]
	return n, ok
}

var sentenceEnd = regexp.MustCompile(`[.;!?\n•]+\s*`)

func splitSentences(text string) []string {
	return sentenceEnd.Split(text, -1)
}

var salaryRe = regexp.MustCompile(`(?i)\$\s?(\d[\d,]*(?:\.\d+)?)\s?(k)?\s*(?:-|–|—|to)\s*\$\s?(\d[\d,]*(?:\.\d+)?)\s?(k)?`)

// SalaryRange finds the first dollar range in text ("$150,000 - $190,000",
// "$150k–$190k") and returns it normalized as "$150,000 – $190,000", or "".
// Ranges whose numbers look too small to be salaries (under $1,000) are ignored.
func SalaryRange(text string) string {
	for _, m := range salaryRe.FindAllStringSubmatch(text, -1) {
		lo, okLo := money(m[1], m[2])
		hi, okHi := money(m[3], m[4])
		if okLo && okHi && lo >= 1000 && hi >= lo {
			return "$" + commas(lo) + " – $" + commas(hi)
		}
	}
	return ""
}

func money(digits, k string) (int, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(digits, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	if k != "" {
		f *= 1000
	}
	return int(f), true
}

func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
