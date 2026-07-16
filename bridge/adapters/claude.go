// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Claude Code adapter. Runs `claude -p` in headless mode, using the
//
//	user's Claude subscription (no API key). Requests JSON output and, when a
//	schema is supplied, passes it via --json-schema for structured results.
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

// ClaudeAdapter shells out to the `claude` CLI.
type ClaudeAdapter struct {
	Bin string // path/name of the claude binary; defaults to "claude"
}

func NewClaudeAdapter() *ClaudeAdapter {
	bin := os.Getenv("RELAYENT_CLAUDE_BIN")
	if bin == "" {
		bin = "claude"
	}
	return &ClaudeAdapter{Bin: bin}
}

func (a *ClaudeAdapter) Name() string { return "claude" }

func (a *ClaudeAdapter) Available() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

// claudePrintJSON matches the envelope of `claude -p --output-format json`, whose
// top-level object carries the assistant text in `result`.
type claudePrintJSON struct {
	Result  string `json:"result"`
	Type    string `json:"type"`
	IsError bool   `json:"is_error"`
}

func (a *ClaudeAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return a.run(ctx, req, false)
}

// run performs one CLI invocation. retry=true marks a single JSON-repair retry so
// it doesn't recurse further.
func (a *ClaudeAdapter) run(ctx context.Context, req Request, retry bool) (Result, error) {
	// Headless, single-shot JSON envelope. --json-schema constrains the model's
	// answer to the requested structure when a schema was provided. Note: the flag
	// takes the schema as an INLINE JSON string argument (not a file path).
	args := []string{"-p", "--output-format", "json"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	prompt := req.Prompt
	if req.JSONSchema != nil {
		schemaJSON, err := json.Marshal(req.JSONSchema)
		if err != nil {
			return Result{}, fmt.Errorf("marshal json schema: %w", err)
		}
		args = append(args, "--json-schema", string(schemaJSON))
		// Reinforce structured output in the prompt — schema validation alone does
		// not reliably shape the `result` text across CLI versions, so we instruct
		// explicitly and echo the schema the answer must conform to.
		prompt += "\n\nYou MUST reply with ONLY a single valid JSON object that conforms" +
			" to this JSON Schema. No prose, no explanation, no markdown code fences —" +
			" output raw JSON and nothing else.\nJSON Schema:\n" + string(schemaJSON)
	}
	if req.System != "" {
		args = append(args, "--append-system-prompt", req.System)
	}

	cmd := exec.CommandContext(ctx, a.Bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	// Run in the bridge's sandbox, never the inherited cwd — see Request.WorkDir.
	cmd.Dir = req.WorkDir
	// Do NOT inject any ANTHROPIC_API_KEY — the CLI uses its own subscription auth.
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("claude cli: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	// Unwrap the print-mode envelope to get the assistant's actual text.
	var env claudePrintJSON
	text := stdout.String()
	if err := json.Unmarshal(stdout.Bytes(), &env); err == nil && env.Result != "" {
		if env.IsError {
			return Result{}, fmt.Errorf("claude cli reported error: %s", env.Result)
		}
		text = env.Result
	}

	res := finalize(text, req.JSONSchema != nil)
	// The CLI's structured-output enforcement is inconsistent across versions; if
	// JSON was required but the model replied with prose, retry once with a curt,
	// forceful re-prompt before giving up and returning the text.
	if req.JSONSchema != nil && !res.IsJSON && !retry {
		schemaJSON, _ := json.Marshal(req.JSONSchema)
		retryReq := req
		retryReq.Prompt = "Convert the following into a single raw JSON object matching this" +
			" schema and output ONLY that JSON (no prose, no fences).\nSchema:\n" +
			string(schemaJSON) + "\n\nContent:\n" + text
		return a.run(ctx, retryReq, true)
	}
	return res, nil
}

// finalize turns raw CLI text into a Result. When JSON was requested it tries to
// parse the text (after stripping code fences) into a structured object; on failure
// it falls back to returning the text so the caller still gets something usable.
func finalize(text string, wantJSON bool) Result {
	if !wantJSON {
		return Result{Text: strings.TrimSpace(text)}
	}
	if obj, ok := parseJSON(text); ok {
		return Result{IsJSON: true, JSON: obj}
	}
	return Result{Text: strings.TrimSpace(text)}
}

// Models reports the aliases `claude --model` accepts. Declared, NOT probed:
// the CLI has no enumerate command, and it also accepts full model names
// (e.g. claude-opus-4-8) which cannot be listed exhaustively. Consumers get
// probed=false so they know this is a hint that may drift with CLI releases,
// not a contract. Full names still work even though they are not listed.
func (a *ClaudeAdapter) Models(ctx context.Context) ([]string, string, bool) {
	return []string{"fable", "opus", "sonnet", "haiku"}, "", false
}
