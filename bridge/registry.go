// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Backend registry for the Relayent bridge. Maps a job's backend name
//
//	to its adapter. Adding a backend later is one constructor + one registry entry.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/ToolTropolis/Relayent/bridge/adapters"
	"github.com/ToolTropolis/Relayent/internal/api"
)

// Registry holds the available backend adapters keyed by name.
type Registry struct {
	adapters map[string]adapters.Adapter
}

// NewRegistry builds the default registry with all known adapters.
func NewRegistry() *Registry {
	r := &Registry{adapters: map[string]adapters.Adapter{}}
	for _, a := range []adapters.Adapter{
		adapters.NewClaudeAdapter(),
		adapters.NewCodexAdapter(),
		adapters.NewGeminiAdapter(),
		adapters.NewCursorAdapter(),
	} {
		r.adapters[a.Name()] = a
	}
	return r
}

// Get returns the adapter for a backend name, or an error if unknown.
func (r *Registry) Get(name string) (adapters.Adapter, error) {
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q", name)
	}
	return a, nil
}

// Available returns the sorted names of adapters whose CLI is installed.
func (r *Registry) Available() []string {
	var out []string
	for name, a := range r.adapters {
		if a.Available() {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Describe reports every known backend: whether its CLI is installed here, whether
// the adapter is implemented, and whether it can actually run jobs (Ready).
// The relay cannot see this machine, so the bridge is the source of truth. Every
// registered adapter is now implemented (supported), so installed == ready == the
// CLI being present; the fields are kept distinct for the wire contract.
func (r *Registry) Describe(ctx context.Context) []api.BackendInfo {
	out := make([]api.BackendInfo, 0, len(r.adapters))
	for name, a := range r.adapters {
		ready := a.Available() // the CLI is present
		info := api.BackendInfo{
			Name:      name,
			Installed: ready,
			Supported: true,
			Ready:     ready,
		}
		// Only ask a usable backend for its models: probing a missing CLI would
		// just time out, and a stub has nothing to say.
		if ready {
			if ml, ok := a.(adapters.ModelLister); ok {
				models, def, probed := ml.Models(ctx)
				info.Models, info.DefaultModel, info.ModelsProbed = models, def, probed
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
