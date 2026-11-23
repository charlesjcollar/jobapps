package graph

import (
	"context"
	"time"

	graphql "github.com/graph-gophers/graphql-go"

	"jobapps/bff/scan"
)

type scanResult struct {
	ID         graphql.ID
	StartedAt  DateTime
	FinishedAt *DateTime
	Running    bool
	Plugins    []*pluginRun
}

type pluginRun struct {
	Name       string
	Status     string
	StartedAt  *DateTime
	FinishedAt *DateTime
	JobsFound  int32
	JobsAdded  int32
	Error      *string
}

func toScan(s scan.Snapshot) *scanResult {
	out := &scanResult{
		ID:         graphql.ID(s.ID),
		StartedAt:  DateTime{s.StartedAt},
		FinishedAt: optTime(s.FinishedAt),
		Running:    s.Running(),
		Plugins:    make([]*pluginRun, len(s.Runs)),
	}
	for i, r := range s.Runs {
		pr := &pluginRun{
			Name:       r.Name,
			Status:     string(r.Status),
			StartedAt:  optTime(r.StartedAt),
			FinishedAt: optTime(r.FinishedAt),
			JobsFound:  int32(r.JobsFound),
			JobsAdded:  int32(r.JobsAdded),
		}
		if r.Error != "" {
			pr.Error = &r.Error
		}
		out.Plugins[i] = pr
	}
	return out
}

func optTime(t *time.Time) *DateTime {
	if t == nil {
		return nil
	}
	return &DateTime{*t}
}

func (r *Resolver) CurrentScan() *scanResult {
	if s := r.scans.Current(); s != nil {
		return toScan(*s)
	}
	return nil
}

func (r *Resolver) StartScan() *scanResult {
	return toScan(r.scans.Start())
}

func (r *Resolver) ScanUpdated(ctx context.Context, args struct{ ID graphql.ID }) (<-chan *scanResult, error) {
	snaps, err := r.scans.Subscribe(ctx, string(args.ID))
	if err != nil {
		return nil, err
	}
	out := make(chan *scanResult)
	go func() {
		defer close(out)
		for s := range snaps {
			select {
			case out <- toScan(s):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
