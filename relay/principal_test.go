// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Tests for Principal and the legacy-mode mapping — the identity
//
//	seam that replaces the raw pairing key. Verifies scope checks and that
//	legacy mode reproduces the single-namespace behaviour.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import "testing"

func TestPrincipalCan(t *testing.T) {
	p := &Principal{Scopes: []string{ScopeEnqueue, ScopeClaim}}
	if !p.Can(ScopeEnqueue) || !p.Can(ScopeClaim) {
		t.Error("principal should hold its granted scopes")
	}
	if p.Can(ScopeAdmin) {
		t.Error("principal must not hold a scope it was not granted")
	}
	if (&Principal{}).Can(ScopeEnqueue) {
		t.Error("a principal with no scopes can do nothing")
	}
}

func TestLegacyPrincipalHasJobScopesNotAdmin(t *testing.T) {
	p := legacyPrincipal("fp123456")
	if p.Kind != KindLegacy || p.UserID != KindLegacy {
		t.Errorf("legacy principal identity = %q/%q, want legacy/legacy", p.Kind, p.UserID)
	}
	if !p.Can(ScopeEnqueue) || !p.Can(ScopeClaim) {
		t.Error("legacy principal must retain the enqueue+claim it always had")
	}
	if p.Can(ScopeAdmin) {
		t.Error("legacy principal must NOT gain admin — that is new, gated surface")
	}
	if p.KeyFP != "fp123456" {
		t.Errorf("KeyFP = %q, want the fingerprint passed in", p.KeyFP)
	}
}
