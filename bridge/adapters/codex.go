// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Codex adapter. Runs `codex exec` non-interactively using the user's
//   Codex subscription (no API key). Reads the prompt from stdin.
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package adapters

import (
	"bytes"
	"context"
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

func (a *CodexAdapter) Run(ctx context.Context, req Request) (Result, error) {
	// `codex exec -` reads the prompt from stdin and runs non-interactively.
	args := []string{"exec"}
	if req.Model != "" {
		args = append(args, "-c", "model="+req.Model)
	}
	args = append(args, "-") // read prompt from stdin

	// When a schema/JSON is requested, steer Codex to emit JSON only. Codex has no
	// dedicated schema flag, so we instruct it in the prompt and parse best-effort.
	prompt := req.Prompt
	if req.System != "" {
		prompt = req.System + "\n\n" + prompt
	}
	if req.JSONSchema != nil {
		prompt += "\n\nReturn ONLY a valid JSON object, no markdown fences, no commentary."
	}

	cmd := exec.CommandContext(ctx, a.Bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("codex cli: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return finalize(stdout.String(), req.JSONSchema != nil), nil
}
