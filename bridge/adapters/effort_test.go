// Primary author: Navjyot Nishant
// Created on: 2026-07-29
// Last updated: 2026-07-29
// Description: Proves each adapter's actual per-backend effort/reasoning-depth
//
//	mapping — claude's --effort flag, codex's -c model_reasoning_effort=,
//	cursor's model-string bracket syntax (only when Model is also set), and
//	gemini's silent no-op since the CLI has no such control at all.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAdapter_EffortPassedAsFlag(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"type":"result","result":"ok","is_error":false}`)

	a := &ClaudeAdapter{Bin: script}
	if _, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, Effort: "high"}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if !strings.Contains(firstLine, "--effort high") {
		t.Fatalf("expected --effort high in argv, got: %s", firstLine)
	}
}

func TestCodexAdapter_EffortPassedAsConfigOverride(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"ok":true}`)

	a := &CodexAdapter{Bin: script}
	if _, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, Effort: "low"}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if !strings.Contains(firstLine, "-c model_reasoning_effort=low") {
		t.Fatalf("expected -c model_reasoning_effort=low in argv, got: %s", firstLine)
	}
}

func TestCursorAdapter_EffortFoldedIntoModelBracket(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"type":"result","result":"ok","is_error":false}`)

	a := &CursorAdapter{Bin: script}
	if _, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, Model: "sonnet-4", Effort: "max"}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if !strings.Contains(firstLine, "--model sonnet-4[effort=max]") {
		t.Fatalf("expected --model sonnet-4[effort=max] in argv, got: %s", firstLine)
	}
}

// Without a model, cursor-agent has nowhere to fold the effort bracket into —
// confirm it's simply omitted rather than producing a malformed flag.
func TestCursorAdapter_EffortIgnoredWithoutModel(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"type":"result","result":"ok","is_error":false}`)

	a := &CursorAdapter{Bin: script}
	if _, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, Effort: "max"}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if strings.Contains(firstLine, "effort") {
		t.Fatalf("expected no effort flag without a model, got: %s", firstLine)
	}
}

// gemini has no effort mechanism at all — confirm the field is accepted
// without error and simply produces no gemini-specific flag for it.
func TestGeminiAdapter_EffortSilentlyIgnored(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"response":"ok"}`)

	a := &GeminiAdapter{Bin: script}
	if _, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, Effort: "high"}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if strings.Contains(firstLine, "effort") {
		t.Fatalf("gemini has no effort mechanism, argv should not mention it: %s", firstLine)
	}
}
