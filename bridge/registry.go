// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Backend registry for the Relayent bridge. Maps a job's backend name
//   to its adapter. Adding a backend later is one constructor + one registry entry.
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package main

import (
	"fmt"
	"sort"

	"github.com/navjyotnishant/relayent/bridge/adapters"
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
