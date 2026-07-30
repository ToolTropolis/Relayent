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

// Available reports whether the CLI is installed. See LoggedIn for auth state.
func (a *GeminiAdapter) Available() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

// LoggedIn checks for gemini-cli's OAuth credentials file. The path is well
// documented and stable, unlike the CLI's auth subcommands (still in flux
// upstream), so a file check is the more reliable signal here.
func (a *GeminiAdapter) LoggedIn(ctx context.Context) (bool, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, false
	}
	_, err = os.Stat(home + "/.gemini/oauth_creds.json")
	if err != nil {
		if os.IsNotExist(err) {
			return false, true
		}
		return false, false
	}
	return true, true
}

func (a *GeminiAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return a.run(ctx, req, false)
}

// run performs one CLI invocation. retry=true marks a single JSON-repair retry so
// it doesn't recurse further.
func (a *GeminiAdapter) run(ctx context.Context, req Request, retry bool) (Result, error) {
	// Compose the prompt: prepend any system instruction, and for a schema request
	// echo the actual schema in the prompt (the CLI has no schema flag; we instruct
	// in-prompt and parse best-effort, matching the claude/codex/cursor adapters).
	prompt := req.Prompt
	if req.System != "" {
		prompt = req.System + "\n\n" + prompt
	}
	if req.JSONSchema != nil {
		schemaJSON, err := json.Marshal(req.JSONSchema)
		if err != nil {
			return Result{}, fmt.Errorf("marshal json schema: %w", err)
		}
		prompt += "\n\nYou MUST reply with ONLY a single valid JSON object that conforms" +
			" to this JSON Schema. No prose, no explanation, no markdown code fences —" +
			" output raw JSON and nothing else.\nJSON Schema:\n" + string(schemaJSON)
	}

	// The prompt goes on stdin, not as a -p arg — the CLI's own help documents
	// -p's value as "appended to input on stdin (if any)", and passing a large
	// prompt as an arg would hit the OS ARG_MAX ceiling. --output-format json
	// returns a structured envelope we unwrap below. -m selects a model when
	// the caller named one.
	args := []string{"--output-format", "json"}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}

	cmd := exec.CommandContext(ctx, a.Bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
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
	res := finalize(text, req.JSONSchema != nil)
	// If JSON was required but the model replied with prose, retry once with a
	// curt, forceful re-prompt before giving up and returning the text.
	if req.JSONSchema != nil && !res.IsJSON && !retry {
		schemaJSON, _ := json.Marshal(req.JSONSchema)
		retryReq := req
		retryReq.System = ""
		retryReq.Prompt = "Convert the following into a single raw JSON object matching this" +
			" schema and output ONLY that JSON (no prose, no fences).\nSchema:\n" +
			string(schemaJSON) + "\n\nContent:\n" + text
		return a.run(ctx, retryReq, true)
	}
	return res, nil
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
