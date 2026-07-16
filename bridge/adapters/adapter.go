// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Backend adapter contract for the Relayent bridge. Each adapter
//   shells out to an already-authenticated local CLI (Claude Code, Codex, ...)
//   in headless mode and returns text or structured JSON. Adapters never store
//   or handle credentials — they reuse the CLI's own subscription session.
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package adapters

import "context"

// Request is one generation to run against a local CLI.
type Request struct {
	Model      string // optional model override (adapter maps to the CLI's --model)
	Prompt     string // user/content prompt (passed on stdin)
	System     string // optional system instruction
	JSONSchema any    // when non-nil, ask the CLI for structured JSON matching this schema
}

// Result is what an adapter produces. When the request carried a JSONSchema and
// the CLI returned valid JSON, JSON is set and IsJSON is true; otherwise Text holds
// the raw output.
type Result struct {
	IsJSON bool
	JSON   any
	Text   string
}

// Adapter runs one Request against a specific local CLI backend.
type Adapter interface {
	// Name is the backend identifier used in job.backend (e.g. "claude").
	Name() string
	// Available reports whether the backing CLI is installed/usable.
	Available() bool
	// Run executes the request, honoring ctx cancellation/timeout.
	Run(ctx context.Context, req Request) (Result, error)
}
