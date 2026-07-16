// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Tests for bridge configuration — the relay URL policy (never ship
//
//	the pairing key over plaintext) and the job workspace (jobs must never run in
//	the user's home directory, which is what triggers macOS file-access prompts).
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRelayURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
		why     string
	}{
		{"https://relay.example.com", false, "https is always fine"},
		{"https://relay.example.com:8443", false, "https with port"},
		{"http://localhost:8787", false, "plaintext to loopback stays on-box"},
		{"http://127.0.0.1:8787", false, "plaintext to loopback stays on-box"},
		{"http://relay.example.com", true, "plaintext to a remote host exposes the key"},
		{"http://192.168.1.50:8787", true, "plaintext on a LAN is still the network"},
		{"relay.example.com", true, "missing scheme"},
		{"ftp://relay.example.com", true, "unsupported scheme"},
	}
	for _, c := range cases {
		err := validateRelayURL(c.url)
		if (err != nil) != c.wantErr {
			t.Errorf("validateRelayURL(%q) error = %v, wantErr %v — %s",
				c.url, err, c.wantErr, c.why)
		}
	}
}

// The workspace is a security boundary: jobs run there and nowhere else.
func TestResolveWorkspaceDefaultsToSandboxNotHome(t *testing.T) {
	ws, err := resolveWorkspace("")
	if err != nil {
		t.Fatalf("resolveWorkspace: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory in this environment")
	}
	if ws == home {
		t.Fatal("default workspace must NOT be the home directory — running there makes " +
			"macOS attribute CLI file access to the bridge and prompt for Desktop/Documents")
	}
	if !strings.HasPrefix(ws, filepath.Join(home, configDirName)) {
		t.Errorf("default workspace = %q, want it under ~/%s", ws, configDirName)
	}
	if fi, err := os.Stat(ws); err != nil {
		t.Errorf("default workspace should be created: %v", err)
	} else if !fi.IsDir() {
		t.Error("workspace should be a directory")
	}
}

func TestResolveWorkspaceHonoursOverride(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "my-workspace")
	ws, err := resolveWorkspace(custom)
	if err != nil {
		t.Fatalf("resolveWorkspace: %v", err)
	}
	if ws != custom {
		t.Errorf("workspace = %q, want %q", ws, custom)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("override workspace should be created: %v", err)
	}
}

func TestResolveWorkspaceReturnsAbsolutePath(t *testing.T) {
	// A relative path would resolve against whatever cwd the process inherited —
	// exactly the ambiguity the workspace exists to remove.
	ws, err := resolveWorkspace("./relative-ws")
	if err != nil {
		t.Fatalf("resolveWorkspace: %v", err)
	}
	defer os.RemoveAll(ws)
	if !filepath.IsAbs(ws) {
		t.Errorf("workspace = %q, want an absolute path", ws)
	}
}
