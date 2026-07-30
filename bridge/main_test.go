// Primary author: Navjyot Nishant
// Created on: 2026-07-29
// Last updated: 2026-07-29
// Description: Proves flattenMessages turns a conversation history into the
//
//	single prompt string every adapter already knows how to consume — none of
//	the four CLI backends accept a structured message array in headless mode.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"testing"

	"github.com/ToolTropolis/Relayent/internal/api"
)

func TestFlattenMessages(t *testing.T) {
	got := flattenMessages([]api.Message{
		{Role: "system", Content: "Be terse."},
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello"},
		{Role: "user", Content: "What's 2+2?"},
	})
	want := "System: Be terse.\n\nUser: Hi\n\nAssistant: Hello\n\nUser: What's 2+2?"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFlattenMessages_EmptyRoleDefaultsToUser(t *testing.T) {
	got := flattenMessages([]api.Message{{Content: "hi"}})
	if got != "User: hi" {
		t.Fatalf("got %q, want %q", got, "User: hi")
	}
}

func TestFlattenMessages_Empty(t *testing.T) {
	if got := flattenMessages(nil); got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}
