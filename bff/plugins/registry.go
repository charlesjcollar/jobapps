// Package plugins is the registry of company scan plugins.
//
// Each company is a folder under plugins/ whose init function calls Register.
// plugins/all imports every such folder; it's generated, so adding or removing
// a company is just adding or deleting its folder, then regenerating:
//
//	go generate ./plugins/...   (or: go run ./plugins/internal/gen)
//
// The Docker build regenerates automatically. Code shared between companies
// (e.g. one client per job-board system) lives under plugins/shared/ and is
// not registered itself.
package plugins

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"jobapps/bff/scan"
)

var (
	mu       sync.Mutex
	registry = map[string]scan.Plugin{}
)

// Register adds a plugin under id, which should be its folder name (e.g. "netflix").
// It panics on a duplicate id, which is a programming error caught at startup.
func Register(id string, p scan.Plugin) {
	mu.Lock()
	defer mu.Unlock()
	id = strings.ToLower(id)
	if _, dup := registry[id]; dup {
		panic(fmt.Sprintf("plugins: %q registered twice", id))
	}
	registry[id] = p
}

// All returns every registered plugin, sorted by name.
func All() []scan.Plugin {
	mu.Lock()
	defer mu.Unlock()
	ps := make([]scan.Plugin, 0, len(registry))
	for _, p := range registry {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool { return strings.ToLower(ps[i].Name()) < strings.ToLower(ps[j].Name()) })
	return ps
}

// IDs returns every registered id, sorted.
func IDs() []string {
	mu.Lock()
	defer mu.Unlock()
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Get returns the plugin registered under id (case-insensitive).
func Get(id string) (scan.Plugin, bool) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := registry[strings.ToLower(id)]
	return p, ok
}
