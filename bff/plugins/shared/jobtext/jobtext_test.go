package jobtext

import "testing"

func TestYearsRequired(t *testing.T) {
	cases := []struct {
		text string
		want int
		ok   bool
	}{
		{"You have 7+ years of experience building services.", 7, true},
		{"5+ years of professional software engineering experience", 5, true},
		{"At least seven (7) years of experience with distributed systems.", 7, true},
		{"8 or more years of relevant experience", 8, true},
		{"Experience: 5-8 years in backend development", 5, true},
		{"5 to 8 years of industry experience", 5, true},
		{"3+ years of Go experience. 10+ years of overall engineering experience.", 10, true},
		{"10 years experience preferred", 10, true},
		{"Strong experience with Java and Kubernetes.", 0, false},
		{"You choose each year how much of your compensation you want in salary.", 0, false},
		{"We have been streaming for 25 years. Experience with Go is a plus.", 0, false}, // years and experience in different sentences
		{"Our 401(k) and 2 weeks of PTO. Experience required.", 0, false},
		{"• 6+ years of experience\n• Bachelor's degree", 6, true},
	}
	for _, c := range cases {
		got, ok := YearsRequired(c.text)
		if got != c.want || ok != c.ok {
			t.Errorf("YearsRequired(%q) = %d, %v; want %d, %v", c.text, got, ok, c.want, c.ok)
		}
	}
}

func TestContainsAnyWordFold(t *testing.T) {
	cases := []struct {
		s    string
		want string
	}{
		{"Software Engineer Intern - Summer", "Intern"},
		{"Software Engineer, Internal Tools", ""},
		{"Software Engineering Manager", "Manager"},
		{"Software Engineer in Test 5", "in Test"},
		{"Software Engineer, Testing Infra", ""},
		{"Staff/ Principal Software Engineer", "Staff"},
	}
	for _, c := range cases {
		if got := ContainsAnyWordFold(c.s, "Intern", "Manager", "in Test", "Staff", "Principal"); got != c.want {
			t.Errorf("%q: got %q, want %q", c.s, got, c.want)
		}
	}
}

func TestHTMLToText(t *testing.T) {
	got := HTMLToText(`<p>Hello&nbsp;<b>world</b></p><ul><li>5+ years</li><li>Go &amp; Java</li></ul>`)
	want := "Hello world\n5+ years\nGo & Java"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSalaryRange(t *testing.T) {
	cases := map[string]string{
		"The range for this role is $500,000 - $860,000. This will vary.": "$500,000 – $860,000",
		"Pay: $150k–$190k plus equity":                                    "$150,000 – $190,000",
		"Base salary $120,000 to $165,000 per year":                       "$120,000 – $165,000",
		"We give a $50 - $100 stipend. Salary $200,000 - $250,000":        "$200,000 – $250,000",
		"Competitive salary":                                              "",
	}
	for in, want := range cases {
		if got := SalaryRange(in); got != want {
			t.Errorf("SalaryRange(%q) = %q, want %q", in, got, want)
		}
	}
}
