package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexAdapter_AttachmentPassedAsImageFlag(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"ok":true}`)
	imgPath := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(imgPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	a := &CodexAdapter{Bin: script}
	if _, err := a.Run(context.Background(), Request{Prompt: "describe this", WorkDir: dir, AttachmentPaths: []string{imgPath}}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(got), "\n", 2)[0]
	if !strings.Contains(firstLine, "-i "+imgPath) {
		t.Fatalf("expected -i %s in argv, got: %s", imgPath, firstLine)
	}
}

func TestGeminiAdapter_AttachmentFoldedIntoPromptAsAtPath(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	script := fakeCLIScript(t, dir, out, `{"response":"ok"}`)
	imgPath := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(imgPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	a := &GeminiAdapter{Bin: script}
	if _, err := a.Run(context.Background(), Request{Prompt: "describe this", WorkDir: dir, AttachmentPaths: []string{imgPath}}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// The prompt is on stdin (not argv), appended after the first ARGS: line.
	if !strings.Contains(string(got), "@"+imgPath) {
		t.Fatalf("expected @%s reference in the input, got: %s", imgPath, got)
	}
}

func TestClaudeAdapter_AttachmentsReturnError(t *testing.T) {
	dir := t.TempDir()
	a := &ClaudeAdapter{Bin: "should-never-run"}
	_, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, AttachmentPaths: []string{"/tmp/x.png"}})
	if err == nil {
		t.Fatal("expected an error — claude has no local attachment mechanism")
	}
}

func TestCursorAdapter_AttachmentsReturnError(t *testing.T) {
	dir := t.TempDir()
	a := &CursorAdapter{Bin: "should-never-run"}
	_, err := a.Run(context.Background(), Request{Prompt: "hi", WorkDir: dir, AttachmentPaths: []string{"/tmp/x.png"}})
	if err == nil {
		t.Fatal("expected an error — cursor has no attachment mechanism")
	}
}
