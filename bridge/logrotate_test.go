// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Tests for log rotation. The property that matters: a bridge
//
//	installed as a login service must never grow its log without bound, and
//	rotating must not lose the writer's file descriptor (launchd/systemd hold
//	the fd, not the path).
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

func TestRotateIfLargeSkipsSmallAndMissingFiles(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "nope.log")
	if rotated, err := rotateIfLarge(missing); err != nil || rotated {
		t.Errorf("missing file: rotated=%v err=%v, want false/nil", rotated, err)
	}

	small := filepath.Join(dir, "small.log")
	if err := os.WriteFile(small, []byte("one line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rotated, err := rotateIfLarge(small); err != nil || rotated {
		t.Errorf("small file: rotated=%v err=%v, want false/nil", rotated, err)
	}
	if _, err := os.Stat(small + ".1"); !os.IsNotExist(err) {
		t.Error("small file should not have produced a .1 generation")
	}
}

func TestRotateIfLargeRotatesAndTruncatesInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bridge.err.log")

	content := strings.Repeat("x", maxLogBytes+100)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// A writer holds the file open across rotation, exactly as launchd does.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rotated, err := rotateIfLarge(path)
	if err != nil {
		t.Fatalf("rotateIfLarge: %v", err)
	}
	if !rotated {
		t.Fatal("oversized file should have rotated")
	}

	// The old content must be preserved in .1 ...
	old, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("rotated generation missing: %v", err)
	}
	if len(old) != len(content) {
		t.Errorf(".1 has %d bytes, want %d", len(old), len(content))
	}

	// ... and the live path must be empty again, not deleted.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("live log should still exist: %v", err)
	}
	if fi.Size() != 0 {
		t.Errorf("live log size = %d, want 0 after truncation", fi.Size())
	}

	// The critical property: the still-open descriptor must land in the NEW file.
	// If rotation had merely renamed, this write would vanish into .1 and the
	// service's logs would silently stop appearing.
	if _, err := f.WriteString("after rotation\n"); err != nil {
		t.Fatalf("write after rotation: %v", err)
	}
	live, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(live), "after rotation") {
		t.Error("writes from the open fd must land in the live log after rotation")
	}
}

func TestRotateKeepsBoundedGenerations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bridge.err.log")
	big := strings.Repeat("y", maxLogBytes+1)

	// Rotate more times than we keep generations.
	for i := 0; i < maxLogFiles+3; i++ {
		if err := os.WriteFile(path, []byte(big), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := rotateIfLarge(path); err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
	}

	// Disk usage must stay bounded: no generation beyond maxLogFiles-1 survives.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > maxLogFiles {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("kept %d files (%v), want at most %d", len(entries), names, maxLogFiles)
	}
	if _, err := os.Stat(path + "." + string(rune('0'+maxLogFiles))); err == nil {
		t.Errorf("generation .%d should have been discarded", maxLogFiles)
	}
}
