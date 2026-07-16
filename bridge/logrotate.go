// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Size-based log rotation for the bridge's own log files. launchd
//
//	and systemd do NOT rotate a service's StandardOutPath/StandardErrorPath, so
//	without this the bridge would append to one file forever and fill the user's
//	disk. Rotation is done in-process (no logrotate/newsyslog dependency, and no
//	admin setup) and is deliberately simple: when a file exceeds maxLogBytes it
//	is renamed to .1, older generations shift up, and the oldest is dropped.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	// maxLogBytes is when a log file is rotated. Small on purpose: this log is a
	// few lines per job, so 5 MiB is weeks of heavy use, and a user debugging a
	// problem should not have to page through a gigabyte.
	maxLogBytes = 5 << 20 // 5 MiB
	// maxLogFiles is how many generations to keep (bridge.err.log + .1 + .2).
	// Worst case on disk is maxLogBytes * maxLogFiles per stream.
	maxLogFiles = 3
	// rotateCheckInterval is how often the running bridge re-checks its size.
	rotateCheckInterval = 5 * time.Minute
)

// rotateIfLarge rotates path when it exceeds maxLogBytes, shifting generations:
// bridge.err.log → .1 → .2, oldest discarded. Returns whether it rotated.
//
// The service keeps its original file descriptor open across a rename (launchd
// and systemd hold the fd, not the path), so after rotating we truncate the
// original path back to zero rather than relying on the writer to reopen it —
// otherwise the service would keep writing into the renamed file and the fresh
// log would stay empty.
func rotateIfLarge(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if fi.Size() < maxLogBytes {
		return false, nil
	}

	// Drop the oldest generation, then shift the rest up.
	oldest := path + "." + strconv.Itoa(maxLogFiles-1)
	_ = os.Remove(oldest) // may not exist; nothing to do if so
	for i := maxLogFiles - 2; i >= 1; i-- {
		from := path + "." + strconv.Itoa(i)
		to := path + "." + strconv.Itoa(i+1)
		if _, err := os.Stat(from); err == nil {
			if err := os.Rename(from, to); err != nil {
				return false, fmt.Errorf("rotate %s: %w", from, err)
			}
		}
	}

	// Copy-then-truncate rather than rename: the service's open fd still points
	// at the original inode, so renaming alone would leave it writing to .1.
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.WriteFile(path+".1", data, 0o600); err != nil {
		return false, fmt.Errorf("write rotated log: %w", err)
	}
	// Truncate in place — keeps the inode, so the running service's writes land
	// in the now-empty file with no restart and no lost lines.
	if err := os.Truncate(path, 0); err != nil {
		return false, fmt.Errorf("truncate %s: %w", path, err)
	}
	return true, nil
}

// rotateLogs checks both of the bridge's log files once.
func rotateLogs() {
	out, errPath, err := logPaths()
	if err != nil {
		return
	}
	for _, p := range []string{out, errPath} {
		if _, err := rotateIfLarge(p); err != nil {
			// Never fail the bridge over logging: a rotation problem must not stop
			// jobs from running. Report it and carry on.
			fmt.Fprintf(os.Stderr, "[relayent-bridge] log rotation: %v\n", err)
		}
	}
}

// logRotationLoop rotates the bridge's logs periodically for the process's
// lifetime. Started by the daemon; a no-op when logs are not being written to
// files (foreground runs inherit the terminal's stdout/stderr).
func logRotationLoop(done <-chan struct{}) {
	rotateLogs() // check at startup: the file may already be oversized
	t := time.NewTicker(rotateCheckInterval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			rotateLogs()
		}
	}
}
