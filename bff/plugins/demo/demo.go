// Package demo registers fake scan plugins for trying out the scan UI. They're
// only registered when DEMO_PLUGINS=true, wait a few seconds, and return no
// jobs, so they never write to the database. One always fails.
package demo

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"time"

	"jobapps/bff/plugins"
	"jobapps/bff/store"
)

func init() {
	if os.Getenv("DEMO_PLUGINS") != "true" {
		return
	}
	plugins.Register("demo-example", demo{name: "Demo: Example Corp"})
	plugins.Register("demo-sample", demo{name: "Demo: Sample Industries"})
	plugins.Register("demo-placeholder", demo{name: "Demo: Placeholder Inc", err: errors.New("GET https://careers.placeholder.example/jobs: 503 Service Unavailable")})
	plugins.Register("demo-test", demo{name: "Demo: Test Labs"})
	plugins.Register("demo-fictional", demo{name: "Demo: Fictional Systems"})
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
