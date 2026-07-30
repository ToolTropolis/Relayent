package main

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/ToolTropolis/Relayent/internal/api"
)

func TestWriteAttachments_DecodesAndWrites(t *testing.T) {
	dir := t.TempDir()
	data := []byte("fake png bytes")
	att := api.Attachment{Name: "photo.png", Data: base64.StdEncoding.EncodeToString(data)}

	paths, cleanup, err := writeAttachments(dir, "job1", []api.Attachment{att})
	if err != nil {
		t.Fatalf("writeAttachments: %v", err)
	}
	defer cleanup()
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1", len(paths))
	}
	if !strings.HasSuffix(paths[0], ".png") {
		t.Fatalf("path %q should keep the .png extension", paths[0])
	}
	got, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestWriteAttachments_CleanupRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	att := api.Attachment{Name: "a.png", Data: base64.StdEncoding.EncodeToString([]byte("x"))}
	paths, cleanup, err := writeAttachments(dir, "job2", []api.Attachment{att})
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed after cleanup")
	}
}

func TestWriteAttachments_InvalidBase64Errors(t *testing.T) {
	dir := t.TempDir()
	att := api.Attachment{Name: "a.png", Data: "not-valid-base64!!!"}
	_, _, err := writeAttachments(dir, "job3", []api.Attachment{att})
	if err == nil {
		t.Fatal("expected an error for invalid base64")
	}
}

func TestWriteAttachments_MaliciousNameDoesNotEscapeWorkspace(t *testing.T) {
	dir := t.TempDir()
	att := api.Attachment{Name: "../../../etc/passwd", Data: base64.StdEncoding.EncodeToString([]byte("x"))}
	paths, cleanup, err := writeAttachments(dir, "job4", []api.Attachment{att})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !strings.HasPrefix(paths[0], dir) {
		t.Fatalf("attachment path %q escaped workspace %q", paths[0], dir)
	}
}

func TestWriteAttachments_Empty(t *testing.T) {
	paths, cleanup, err := writeAttachments(t.TempDir(), "job5", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(paths) != 0 {
		t.Fatalf("got %d paths, want 0", len(paths))
	}
}
