// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Gemini adapter. Runs the Gemini CLI (github.com/google-gemini/
//
//	gemini-cli) non-interactively with the user's own Gemini auth (no API key).
//	The CLI takes the prompt via -p, an optional model via -m, and can emit a
//	structured envelope with --output-format json whose "response" field holds
//	the text; we unwrap that so callers get the model's answer, not the wrapper.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GeminiAdapter shells out to the `gemini` CLI in one-shot mode.
type GeminiAdapter struct{ Bin string }

func NewGeminiAdapter() *GeminiAdapter {
	bin := os.Getenv("RELAYENT_GEMINI_BIN")
	if bin == "" {
		bin = "gemini"
	}
	return &GeminiAdapter{Bin: bin}
}

func (a *GeminiAdapter) Name() string { return "gemini" }

// Available reports whether the CLI is installed. (Login is checked at run time —
// like the other adapters, an installed-but-logged-out CLI fails the job with the
// CLI's own error rather than being hidden here.)
func (a *GeminiAdapter) Available() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

func (a *GeminiAdapter) Run(ctx context.Context, req Request) (Result, error) {
	// Compose the prompt: prepend any system instruction, and for a schema request
	// steer JSON-only (the CLI has no schema flag; we instruct in-prompt and parse
	// best-effort, matching the codex/cursor adapters).
	prompt := req.Prompt
	if req.System != "" {
		prompt = req.System + "\n\n" + prompt
	}
	if req.JSONSchema != nil {
		prompt += "\n\nReturn ONLY a valid JSON object, no markdown fences, no commentary."
	}

	// -p passes the prompt; --output-format json returns a structured envelope we
	// unwrap below. -m selects a model when the caller named one.
	args := []string{"-p", prompt, "--output-format", "json"}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}

	cmd := exec.CommandContext(ctx, a.Bin, args...)
	// Run in the bridge's sandbox, never the inherited cwd — see Request.WorkDir.
	cmd.Dir = req.WorkDir
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("gemini cli: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	// --output-format json wraps the answer as {"response": "...", ...}. Unwrap to
	// the model's text; if the envelope isn't what we expect, fall back to the raw
	// output so a CLI change degrades to plain text rather than an error.
	text := unwrapGeminiJSON(stdout.String())
	return finalize(text, req.JSONSchema != nil), nil
}

// unwrapGeminiJSON extracts the "response" field from the CLI's JSON envelope,
// returning the raw string unchanged if it isn't that shape.
func unwrapGeminiJSON(out string) string {
	trimmed := strings.TrimSpace(out)
	if !strings.HasPrefix(trimmed, "{") {
		return out // not the JSON envelope; use as-is
	}
	var env struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal([]byte(trimmed), &env); err == nil && env.Response != "" {
		return env.Response
	}
	return out
}

// Models reports nothing to enumerate: the CLI has no list command, so any list
// here would be invention. The model field still works — pass a known name (e.g.
// gemini-2.5-flash). Empty + probed=false is the honest answer.
func (a *GeminiAdapter) Models(ctx context.Context) ([]string, string, bool) {
	return nil, "", false
}
