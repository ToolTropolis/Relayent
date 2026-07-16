// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Backend adapter contract for the Relayent bridge. Each adapter
//
//	shells out to an already-authenticated local CLI (Claude Code, Codex, ...)
//	in headless mode and returns text or structured JSON. Adapters never store
//	or handle credentials — they reuse the CLI's own subscription session.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package adapters

import "context"

// Request is one generation to run against a local CLI.
type Request struct {
	Model      string // optional model override (adapter maps to the CLI's --model)
	Prompt     string // user/content prompt (passed on stdin)
	System     string // optional system instruction
	JSONSchema any    // when non-nil, ask the CLI for structured JSON matching this schema

	// WorkDir is the directory the CLI process runs in. It must always be set to
	// the bridge's dedicated workspace and never left empty: a child process
	// otherwise inherits the bridge's own working directory, and on macOS the OS
	// then attributes the child's file access to the bridge — which is what makes
	// it prompt for Desktop/Documents/Downloads access. Jobs need none of the
	// user's files, so they run in an empty sandbox.
	WorkDir string
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

// ModelLister is implemented by adapters that can report which models they
// accept, so a consuming app can discover valid `model` values instead of
// guessing and finding out at job time.
//
// Optional on purpose: the CLIs are wildly inconsistent here. `cursor-agent`
// has --list-models and can be probed for the truth; `claude` documents a few
// aliases and no way to enumerate them; `codex` documents nothing. An adapter
// that cannot answer simply does not implement this, and the API reports an
// empty list rather than inventing one.
type ModelLister interface {
	// Models returns the accepted model identifiers. Probed is true when the
	// list came from the CLI itself, false when it is a static declaration that
	// may drift as the CLI changes — consumers should treat a declared list as
	// a hint, not a contract.
	Models(ctx context.Context) (models []string, defaultModel string, probed bool)
}
