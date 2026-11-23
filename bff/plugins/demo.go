package plugins

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"jobapps/bff/scan"
	"jobapps/bff/store"
)

// demoPlugins are fake scanners for trying out the scan UI before real plugins
// exist. Enabled with DEMO_PLUGINS=true. They wait a few seconds and return no
// jobs, so they never write to the database. One always fails.
func demoPlugins() []scan.Plugin {
	return []scan.Plugin{
		demo{name: "Demo: Example Corp"},
		demo{name: "Demo: Sample Industries"},
		demo{name: "Demo: Placeholder Inc", err: errors.New("GET https://careers.placeholder.example/jobs: 503 Service Unavailable")},
		demo{name: "Demo: Test Labs"},
		demo{name: "Demo: Fictional Systems"},
	}
}

type demo struct {
	name string
	err  error
}

func (d demo) Name() string { return d.name }

func (d demo) Scan(ctx context.Context) ([]store.NewJob, error) {
	select {
	case <-time.After(time.Duration(1000+rand.IntN(4000)) * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, d.err
}
