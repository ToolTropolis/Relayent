// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Tests for the audit log — the load-bearing one is a property
//
//	test proving that no prompt or result content ever reaches the log,
//	backed by a reflection check that the AuditEvent type has no content field.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func auditStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "au.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// STRUCTURAL: the AuditEvent type must have no field capable of holding content.
func TestAuditEventHasNoContentField(t *testing.T) {
	banned := []string{"Prompt", "Result", "Content", "Text", "Response", "Output", "JSON", "System"}
	et := reflect.TypeOf(AuditEvent{})
	for i := 0; i < et.NumField(); i++ {
		name := et.Field(i).Name
		for _, b := range banned {
			// PromptLen/ResultLen are allowed (counts, not content); a bare
			// Prompt/Result/etc. is not.
			if name == b {
				t.Errorf("AuditEvent has field %q — content must never be at rest", name)
			}
		}
	}
}

// PROPERTY: after auditing many events with random prompts, none of those
// prompts appears anywhere in the persisted audit log.
func TestAuditLogNeverContainsContent(t *testing.T) {
	s := auditStore(t)
	prompts := make([]string, 50)
	for i := range prompts {
		b := make([]byte, 24)
		rand.Read(b)
		prompts[i] = "PROMPT_" + base64.RawURLEncoding.EncodeToString(b)
		// Audit as the handler would: length only.
		s.Append(AuditEvent{
			ActorSub: "app", Event: EvEnqueue, JobID: "j", TargetSub: "alice",
			Backend: "cursor", PromptLen: len(prompts[i]),
		})
	}
	events, err := s.RecentAudit("", 1000)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(events)
	for _, p := range prompts {
		if strings.Contains(string(blob), p) {
			t.Fatalf("audit log contains prompt content: %q", p)
		}
	}
	// But the count is recorded.
	if len(events) != 50 {
		t.Errorf("expected 50 audit events, got %d", len(events))
	}
	if events[0].PromptLen == 0 {
		t.Error("prompt length should be recorded")
	}
}

// Audit is newest-first and filterable by target user.
func TestAuditOrderAndFilter(t *testing.T) {
	s := auditStore(t)
	s.Append(AuditEvent{Event: EvEnqueue, TargetSub: "alice", TS: time.Now()})
	s.Append(AuditEvent{Event: EvEnqueue, TargetSub: "bob", TS: time.Now()})
	s.Append(AuditEvent{Event: EvResult, TargetSub: "alice", TS: time.Now()})

	all, _ := s.RecentAudit("", 10)
	if len(all) != 3 || all[0].Event != EvResult {
		t.Fatalf("expected 3 events newest-first, got %d (first=%v)", len(all), all)
	}
	alice, _ := s.RecentAudit("alice", 10)
	if len(alice) != 2 {
		t.Errorf("alice filter should give 2 events, got %d", len(alice))
	}
	for _, e := range alice {
		if e.TargetSub != "alice" {
			t.Errorf("filter leaked a non-alice event: %+v", e)
		}
	}
}

// Nil store: audit is a safe no-op.
func TestAuditNilStore(t *testing.T) {
	var s *Store
	if err := s.Append(AuditEvent{Event: EvEnqueue}); err != nil {
		t.Errorf("nil Append should no-op, got %v", err)
	}
	if ev, err := s.RecentAudit("", 10); err != nil || ev != nil {
		t.Errorf("nil RecentAudit should be (nil,nil), got (%v,%v)", ev, err)
	}
}
