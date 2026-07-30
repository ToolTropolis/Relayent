// Primary author: Navjyot Nishant
// Created on: 2026-07-29
// Last updated: 2026-07-29
// Description: Proves the schema-echo + one-shot repair-retry path for codex and
//
//	gemini — neither CLI has a native structured-output flag, so both rely on
//	echoing the actual JSON Schema in the prompt and retrying once with a
//	curt re-prompt if the first reply wasn't valid JSON, matching the
//	claude/cursor pattern this brings them up to.
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

// proseThenJSONScript writes a shell script that replies with plain prose on
// its first invocation and a JSON envelope on every invocation after, using a
// counter file in dir to track calls. This exercises the schema-echo +
// one-shot repair-retry path without a real CLI or network call.
func proseThenJSONScript(t *testing.T, dir, jsonEnvelope string) string {
	t.Helper()
	counter := filepath.Join(dir, "calls")
	script := filepath.Join(dir, "fake-cli.sh")
	body := "#!/bin/sh\n" +
		"n=$(cat \"" + counter + "\" 2>/dev/null || echo 0)\n" +
		"echo $((n+1)) > \"" + counter + "\"\n" +
		"cat > /dev/null\n" + // drain stdin
		"if [ \"$n\" = \"0\" ]; then\n" +
		"  echo 'sure, here you go: not json at all'\n" +
		"else\n" +
		"  cat <<'JSONEOF'\n" + jsonEnvelope + "\nJSONEOF\n" +
		"fi\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	return script
}

func callCount(t *testing.T, dir string) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	n := strings.TrimSpace(string(b))
	switch n {
	case "1":
		return 1
	case "2":
		return 2
	default:
		t.Fatalf("unexpected call count %q", n)
		return -1
	}
}

var testSchema = map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}}

func TestCodexAdapter_SchemaRetryOnProse(t *testing.T) {
	dir := t.TempDir()
	script := proseThenJSONScript(t, dir, `{"ok":true}`)
	a := &CodexAdapter{Bin: script}
	res, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, JSONSchema: testSchema})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !res.IsJSON {
		t.Fatal("expected JSON result after retry, got prose")
	}
	if callCount(t, dir) != 2 {
		t.Fatal("expected exactly one retry (two CLI invocations)")
	}
}

func TestGeminiAdapter_SchemaRetryOnProse(t *testing.T) {
	dir := t.TempDir()
	script := proseThenJSONScript(t, dir, `{"response":"{\"ok\":true}"}`)
	a := &GeminiAdapter{Bin: script}
	res, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, JSONSchema: testSchema})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !res.IsJSON {
		t.Fatal("expected JSON result after retry, got prose")
	}
	if callCount(t, dir) != 2 {
		t.Fatal("expected exactly one retry (two CLI invocations)")
	}
}
