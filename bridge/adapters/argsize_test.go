// Primary author: Navjyot Nishant
// Created on: 2026-07-29
// Last updated: 2026-07-29
// Description: Proves cursor and gemini pass large prompts via stdin, not argv —
//
//	an argv-based prompt hits the OS ARG_MAX ceiling regardless of the relay's
//	own body-size cap, so this is the property that actually protects big jobs.
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

// fakeCLIScript writes a tiny shell script that dumps its argv and stdin to
// outPath, then prints a minimal JSON envelope matching what the real CLI
// would return so the adapter's own unwrap logic doesn't error.
func fakeCLIScript(t *testing.T, dir, outPath, jsonEnvelope string) string {
	t.Helper()
	script := filepath.Join(dir, "fake-cli.sh")
	body := "#!/bin/sh\n" +
		"printf 'ARGS:%s\\n' \"$*\" > \"" + outPath + "\"\n" +
		"cat >> \"" + outPath + "\"\n" + // append stdin
		"cat <<'JSONEOF'\n" + jsonEnvelope + "\nJSONEOF\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	return script
}

// A prompt this large would exceed typical OS ARG_MAX (commonly a few hundred
// KB to ~2MB) if ever passed as a command-line argument, so the test both
// proves the arg list stays empty of prompt content AND that a large prompt
// doesn't cause exec itself to fail.
func bigPrompt() string { return strings.Repeat("x", 512*1024) }

func TestCursorAdapter_PromptViaStdinNotArgs(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"type":"result","result":"ok","is_error":false}`)

	a := &CursorAdapter{Bin: script}
	prompt := bigPrompt()
	_, err := a.Run(context.Background(), Request{Prompt: prompt, WorkDir: dir})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if strings.Contains(firstLine, "x") {
		t.Fatalf("prompt leaked into argv: %s", firstLine)
	}
	if !strings.Contains(string(got), prompt) {
		t.Fatal("prompt was not delivered via stdin")
	}
}

func TestGeminiAdapter_PromptViaStdinNotArgs(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"response":"ok"}`)

	a := &GeminiAdapter{Bin: script}
	prompt := bigPrompt()
	_, err := a.Run(context.Background(), Request{Prompt: prompt, WorkDir: dir})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if strings.Contains(firstLine, "x") {
		t.Fatalf("prompt leaked into argv: %s", firstLine)
	}
	if !strings.Contains(string(got), prompt) {
		t.Fatal("prompt was not delivered via stdin")
	}
}
