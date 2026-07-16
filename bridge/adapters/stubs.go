// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Placeholder adapter for the Gemini CLI. It registers the backend
//   name but reports unavailable until the CLI's non-interactive interface is
//   wired. Follow the ClaudeAdapter/CursorAdapter shape when implementing.
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

// Available reports false: the CLI may be installed, but this adapter cannot drive
// it yet, and claiming otherwise would let jobs route here and fail.
func (a *GeminiAdapter) Available() bool { return false }

// BinPresent reports whether the CLI itself exists, independent of adapter support.
func (a *GeminiAdapter) BinPresent() bool {
	_, err := exec.LookPath(a.Bin)
	return err == nil
}

func (a *GeminiAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return Result{}, fmt.Errorf("gemini adapter not implemented yet")
}

