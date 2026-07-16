// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Placeholder adapters for Gemini and Cursor CLIs. They register the
//   backend name but report unavailable until each CLI's non-interactive interface
//   is wired. Follow the ClaudeAdapter shape when implementing.
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package adapters

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// GeminiAdapter is a stub until the Gemini CLI headless invocation is wired.
type GeminiAdapter struct{ Bin string }

func NewGeminiAdapter() *GeminiAdapter {
	bin := os.Getenv("RELAYENT_GEMINI_BIN")
	if bin == "" {
		bin = "gemini"
	}
	return &GeminiAdapter{Bin: bin}
}

func (a *GeminiAdapter) Name() string { return "gemini" }

func (a *GeminiAdapter) Available() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

func (a *GeminiAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return Result{}, fmt.Errorf("gemini adapter not implemented yet")
}

// CursorAdapter is a stub until the Cursor agent/CLI headless interface is confirmed.
type CursorAdapter struct{ Bin string }

func NewCursorAdapter() *CursorAdapter {
	bin := os.Getenv("RELAYENT_CURSOR_BIN")
	if bin == "" {
		bin = "cursor-agent"
	}
	return &CursorAdapter{Bin: bin}
}

func (a *CursorAdapter) Name() string { return "cursor" }

func (a *CursorAdapter) Available() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

func (a *CursorAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return Result{}, fmt.Errorf("cursor adapter not implemented yet")
}
