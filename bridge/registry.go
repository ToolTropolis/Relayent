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
// The relay cannot see this machine, so the bridge is the source of truth.
// Ready additionally requires a logged-in check to pass where the adapter can
// tell (AuthChecker) — Installed alone does not guarantee jobs will succeed.
func (r *Registry) Describe(ctx context.Context) []api.BackendInfo {
	out := make([]api.BackendInfo, 0, len(r.adapters))
	for name, a := range r.adapters {
		installed := a.Available()
		ready := installed
		// Installed is not the same as usable: a CLI that's present but not
		// signed in will fail every job. Only downgrade Ready when the check
		// itself succeeded (ok) — an inconclusive check must not hide a
		// backend that's actually fine.
		if installed {
			if ac, ok := a.(adapters.AuthChecker); ok {
				if loggedIn, checked := ac.LoggedIn(ctx); checked && !loggedIn {
					ready = false
				}
			}
		}
		info := api.BackendInfo{
			Name:      name,
			Installed: installed,
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
