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
	"fmt"
	"sort"

	"github.com/navjyotnishant/relayent/bridge/adapters"
	"github.com/navjyotnishant/relayent/internal/api"
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

// binPresenter is implemented by stub adapters, which report Available()==false
// (they can't run jobs) but can still say whether the CLI exists on this host.
type binPresenter interface{ BinPresent() bool }

// Describe reports every known backend: whether its CLI is installed here, whether
// the adapter is implemented, and whether it can actually run jobs (Ready).
// The relay cannot see this machine, so the bridge is the source of truth.
func (r *Registry) Describe() []api.BackendInfo {
	out := make([]api.BackendInfo, 0, len(r.adapters))
	for name, a := range r.adapters {
		ready := a.Available() // implemented adapters gate on the CLI being present
		installed, supported := ready, true
		// A stub adapter is never Ready; ask it separately whether the CLI exists.
		if bp, ok := a.(binPresenter); ok {
			supported = false
			installed = bp.BinPresent()
		}
		out = append(out, api.BackendInfo{
			Name:      name,
			Installed: installed,
			Supported: supported,
			Ready:     ready,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
