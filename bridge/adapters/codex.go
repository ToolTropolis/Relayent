// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Codex adapter. Runs `codex exec` non-interactively using the user's
//
//	Codex subscription (no API key). Reads the prompt from stdin.
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

// CodexAdapter shells out to the `codex` CLI in exec mode.
type CodexAdapter struct {
	Bin string
}

func NewCodexAdapter() *CodexAdapter {
	bin := os.Getenv("RELAYENT_CODEX_BIN")
	if bin == "" {
		bin = "codex"
	}
	return &CodexAdapter{Bin: bin}
}

func (a *CodexAdapter) Name() string { return "codex" }

func (a *CodexAdapter) Available() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

// LoggedIn checks for the presence of Codex's own auth file rather than
// shelling out — cheap, and the path is well documented, unlike the other
// three CLIs. ok is false only if we can't even stat $HOME (a genuine local
// error, not a "logged out" signal).
func (a *CodexAdapter) LoggedIn(ctx context.Context) (bool, bool) {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, false
		}
		dir = home + "/.codex"
	}
	_, err := os.Stat(dir + "/auth.json")
	if err != nil {
		if os.IsNotExist(err) {
			return false, true
		}
		return false, false
	}
	return true, true
}

func (a *CodexAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return a.run(ctx, req, false)
}

// run performs one CLI invocation. retry=true marks a single JSON-repair retry so
// it doesn't recurse further.
func (a *CodexAdapter) run(ctx context.Context, req Request, retry bool) (Result, error) {
	// `codex exec -` reads the prompt from stdin and runs non-interactively.
	args := []string{"exec"}
	if req.Model != "" {
		args = append(args, "-c", "model="+req.Model)
	}
	if req.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+req.Effort)
	}
	// codex has a real local-file attach flag, verified via --help ("-i,
	// --image <FILE>... Optional image(s) to attach to the initial prompt").
	for _, p := range req.AttachmentPaths {
		args = append(args, "-i", p)
	}
	args = append(args, "-") // read prompt from stdin

	// When a schema/JSON is requested, steer Codex to emit JSON only. Codex has no
	// dedicated schema flag, so we echo the actual schema in the prompt and parse
	// best-effort, matching the claude/cursor adapters.
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

	cmd := exec.CommandContext(ctx, a.Bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	// Run in the bridge's sandbox, never the inherited cwd — see Request.WorkDir.
	cmd.Dir = req.WorkDir
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("codex cli: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	text := stdout.String()
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

// Models reports nothing: `codex --model` documents no aliases and the CLI has
// no enumerate command, so any list here would be invention. An empty list with
// probed=false is the honest answer — the model field still works, callers just
// have to know the name they want.
func (a *CodexAdapter) Models(ctx context.Context) ([]string, string, bool) {
	return nil, "", false
}
