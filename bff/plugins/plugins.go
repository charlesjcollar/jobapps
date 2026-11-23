// Package plugins lists the company scanners the bff runs on each scan.
package plugins

import (
	"os"

	"jobapps/bff/scan"
)

// All returns every plugin to run on a scan, in display order.
func All() []scan.Plugin {
	var ps []scan.Plugin

	// Real company plugins go here.

	if os.Getenv("DEMO_PLUGINS") == "true" {
		ps = append(ps, demoPlugins()...)
	}
	return ps
}
