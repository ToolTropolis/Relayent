// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Cursor adapter. Runs `cursor-agent -p --output-format json` in
//
//	headless mode using the user's Cursor subscription (no API key). Uses
//	--mode ask (read-only Q&A) so generation jobs can never edit files or run
//	shell commands, and --trust to satisfy the headless workspace-trust prompt.
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

// CursorAdapter shells out to the `cursor-agent` CLI.
type CursorAdapter struct {
	Bin string
}

func NewCursorAdapter() *CursorAdapter {
	bin := os.Getenv("RELAYENT_CURSOR_BIN")
	if bin == "" {
		bin = "cursor-agent"
	}
	return &CursorAdapter{Bin: bin}
}

func (a *CursorAdapter) Name() string { return "cursor" }

func (a *CursorAdapter) Available() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

// cursorPrintJSON matches the envelope of `cursor-agent -p --output-format json`.
type cursorPrintJSON struct {
	Type    string `json:"type"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

func (a *CursorAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return a.run(ctx, req, false)
}

// run performs one CLI invocation. retry=true marks a single JSON-repair retry so
// it doesn't recurse further.
func (a *CursorAdapter) run(ctx context.Context, req Request, retry bool) (Result, error) {
	// --mode ask keeps this read-only (no edits/shell); --trust is required for
	// non-interactive use in an untrusted workspace.
	args := []string{"-p", "--output-format", "json", "--mode", "ask", "--trust"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	prompt := req.Prompt
	if req.System != "" {
		prompt = req.System + "\n\n" + prompt
	}
	if req.JSONSchema != nil {
		schemaJSON, err := json.Marshal(req.JSONSchema)
		if err != nil {
			return Result{}, fmt.Errorf("marshal json schema: %w", err)
		}
		// Cursor has no schema flag — instruct it explicitly and parse best-effort.
		prompt += "\n\nYou MUST reply with ONLY a single valid JSON object that conforms" +
			" to this JSON Schema. No prose, no explanation, no markdown code fences —" +
			" output raw JSON and nothing else.\nJSON Schema:\n" + string(schemaJSON)
	}
	// cursor-agent takes the prompt as an argument, not on stdin.
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, a.Bin, args...)
	// Run in the bridge's sandbox, never the inherited cwd — see Request.WorkDir.
	cmd.Dir = req.WorkDir
	// Do NOT inject CURSOR_API_KEY — the CLI uses its own subscription session.
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("cursor cli: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	// Unwrap the print-mode envelope to get the assistant's actual text.
	var env cursorPrintJSON
	text := stdout.String()
	if err := json.Unmarshal(stdout.Bytes(), &env); err == nil && env.Result != "" {
		if env.IsError {
			return Result{}, fmt.Errorf("cursor cli reported error: %s", env.Result)
		}
		text = env.Result
	}

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
