package scan

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"jobapps/bff/store"
)

// ExpirableStatuses are the statuses ExpireMissing may change to EXPIRED: New,
// Interested and Not pursuing. Jobs I've applied to (or further) keep their
// status even after the posting comes down.
var ExpirableStatuses = []string{"NEW", "INTERESTED", "USER_REJECTED"}

// Expiration safety limit: if more than half of a company's expirable saved
// jobs (and more than safetyMinJobs of them) vanish in one scan, it's far more
// likely the site changed how it lists or identifies postings than that they
// all closed at once, so nothing is expired and the run reports an error.
const safetyMinJobs = 5

// Expirer reads and expires saved jobs. *store.Store implements it.
type Expirer interface {
	// CompanyJobs returns the company's saved jobs with one of statuses.
	CompanyJobs(ctx context.Context, company string, statuses []string) ([]store.Job, error)
	// MarkExpired sets jobs to EXPIRED if their status is still one of onlyIf,
	// and returns the ones it changed.
	MarkExpired(ctx context.Context, ids []int64, onlyIf []string) ([]store.Job, error)
}

type expirerKey struct{}

// WithExpirer returns a context carrying e, for ExpireMissing to use.
func WithExpirer(ctx context.Context, e Expirer) context.Context {
	return context.WithValue(ctx, expirerKey{}, e)
}

// ExpireMissing marks the company's saved postings that are no longer open as
// EXPIRED. Every plugin calls it from Scan, right after fetching the site's
// list of open postings.
//
//   - openPostingIDs must be the posting IDs of every open posting the site
//     listed, before the plugin's filters: a saved job that now fails a filter
//     is still open. IDs must be built the same way as store.NewJob.PostingID.
//   - Only call it when the list is complete. If fetching the list failed or
//     stopped early, skip it, or open postings would be expired.
//
// Only jobs in ExpirableStatuses are changed. With no Expirer in ctx (e.g. the
// plugin CLI without -db), it only logs that it skipped. An empty list is also
// skipped, since that more likely means the site broke than that the company
// has nothing open. It returns how many jobs were expired.
func ExpireMissing(ctx context.Context, company string, openPostingIDs []string) (int, error) {
	log := Logger(ctx)
	e, ok := ctx.Value(expirerKey{}).(Expirer)
	if !ok || e == nil {
		log.Info("skipping expiration check: no database")
		return 0, nil
	}
	if len(openPostingIDs) == 0 {
		log.Warn("skipping expiration check: the site listed no open postings")
		return 0, nil
	}

	open := make(map[string]bool, len(openPostingIDs))
	for _, id := range openPostingIDs {
		open[strings.TrimSpace(id)] = true
	}

	saved, err := e.CompanyJobs(ctx, company, ExpirableStatuses)
	if err != nil {
		return 0, fmt.Errorf("expiration check: reading saved jobs: %w", err)
	}
	var missing []store.Job
	for _, j := range saved {
		if !open[strings.TrimSpace(j.PostingID)] {
			missing = append(missing, j)
		}
	}
	if len(missing) == 0 {
		log.Info("expiration check: all saved postings are still open", "saved", len(saved))
		return 0, nil
	}
	if len(missing) > safetyMinJobs && len(missing)*2 > len(saved) {
		return 0, fmt.Errorf("expiration check: %d of %d saved postings are missing from the site, which more likely "+
			"means the site changed how it lists or identifies postings than that they all closed; nothing was "+
			"expired. Check with: go run ./cmd/plugin -db <plugin>", len(missing), len(saved))
	}

	ids := make([]int64, len(missing))
	was := make(map[int64]string, len(missing))
	for i, j := range missing {
		ids[i] = j.ID
		was[j.ID] = j.Status
	}
	expired, err := e.MarkExpired(ctx, ids, ExpirableStatuses)
	if err != nil {
		return 0, fmt.Errorf("expiration check: marking %d jobs expired: %w", len(ids), err)
	}
	for _, j := range expired {
		log.Info("expired posting", "postingId", j.PostingID, "title", j.JobTitle, "was", was[j.ID])
	}
	log.Info("expiration check done", "saved", len(saved), "expired", len(expired))
	return len(expired), nil
}

// countingExpirer counts the jobs a plugin expires, for its Run.
type countingExpirer struct {
	Expirer
	n atomic.Int64
}

func (c *countingExpirer) MarkExpired(ctx context.Context, ids []int64, onlyIf []string) ([]store.Job, error) {
	jobs, err := c.Expirer.MarkExpired(ctx, ids, onlyIf)
	c.n.Add(int64(len(jobs)))
	return jobs, err
}
