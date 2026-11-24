// Command plugin runs one scan plugin and prints what it finds, without
// touching the database. For developing and debugging plugins.
//
//	go run ./cmd/plugin                 list plugins
//	go run ./cmd/plugin netflix         run one; print the postings it keeps
//	go run ./cmd/plugin -v netflix      also log every rejected posting (with the
//	                                    reason) and every HTTP request
//	go run ./cmd/plugin -json netflix   print kept postings as JSON
//
// Logs go to stderr, results to stdout. Exits 1 if the plugin returns an error
// (any postings found before the error are still printed).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"jobapps/bff/plugins"
	_ "jobapps/bff/plugins/all"
	"jobapps/bff/scan"
	"jobapps/bff/store"
)

func main() {
	verbose := flag.Bool("v", false, "log rejected postings and HTTP requests")
	asJSON := flag.Bool("json", false, "print results as JSON")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/plugin [-v] [-json] <plugin>")
		fmt.Fprintln(os.Stderr, "plugins:", strings.Join(plugins.IDs(), ", "))
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Println("Plugins (id: name):")
		for _, id := range plugins.IDs() {
			p, _ := plugins.Get(id)
			fmt.Printf("  %s: %s\n", id, p.Name())
		}
		return
	}
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	p, ok := plugins.Get(flag.Arg(0))
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown plugin %q; available: %s\n", flag.Arg(0), strings.Join(plugins.IDs(), ", "))
		os.Exit(2)
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})).With("plugin", p.Name())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	timeout := 2 * time.Minute
	if tp, ok := p.(scan.TimeoutPlugin); ok {
		timeout = tp.Timeout()
	}
	ctx, cancel := context.WithTimeout(scan.WithLogger(ctx, logger), timeout)
	defer cancel()

	start := time.Now()
	jobs, err := p.Scan(ctx)
	logger.Info("done", "kept", len(jobs), "took", time.Since(start).Round(time.Second))

	if *asJSON {
		printJSON(jobs)
	} else {
		printTable(jobs)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nERROR: %v\n", err)
		os.Exit(1)
	}
}

func printTable(jobs []store.NewJob) {
	if len(jobs) == 0 {
		fmt.Println("No postings matched.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "POSTING ID\tTITLE\tLOCATION\tSALARY\tPOSTED\tLINK")
	for _, j := range jobs {
		posted := ""
		if j.PostedDate != nil {
			posted = j.PostedDate.Format("2006-01-02")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", j.PostingID, j.JobTitle, str(j.Location), str(j.SalaryRange), posted, j.Link)
	}
	tw.Flush()
	fmt.Printf("\n%d postings\n", len(jobs))
}

func printJSON(jobs []store.NewJob) {
	type out struct {
		Company     string  `json:"companyName"`
		Title       string  `json:"jobTitle"`
		PostingID   string  `json:"postingId"`
		Link        string  `json:"link"`
		PostedDate  *string `json:"postedDate"`
		Location    *string `json:"location"`
		SalaryRange *string `json:"salaryRange"`
	}
	list := make([]out, len(jobs))
	for i, j := range jobs {
		list[i] = out{j.CompanyName, j.JobTitle, j.PostingID, j.Link, nil, j.Location, j.SalaryRange}
		if j.PostedDate != nil {
			d := j.PostedDate.Format("2006-01-02")
			list[i].PostedDate = &d
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(list)
}

func str(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}
