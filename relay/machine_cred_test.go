// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Tests for machine credentials (bridge + app) — generation,
//
//	constant-time verification, the store round-trips, one-time enrollment
//	token redemption, and that the raw secret is never stored.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"strings"
	"testing"
	"time"
)

func TestNewMachineCredentialShape(t *testing.T) {
	full, id, hash, err := newMachineCredential()
	if err != nil {
		t.Fatal(err)
	}
	gotID, secret, ok := splitCredential(full)
	if !ok || gotID != id {
		t.Fatalf("split(%q) = (%q,_,%v), want id %q", full, gotID, ok, id)
	}
	// The stored hash must match the secret, and must NOT be the secret.
	if !verifySecret(secret, hash) {
		t.Error("secret should verify against its own hash")
	}
	if strings.Contains(hash, secret) || hash == secret {
		t.Error("stored hash must not contain/equal the raw secret")
	}
	if verifySecret("wrong-secret", hash) {
		t.Error("a wrong secret must not verify")
	}
}

func TestCredentialsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		full, _, _, err := newMachineCredential()
		if err != nil {
			t.Fatal(err)
		}
		if seen[full] {
			t.Fatal("duplicate credential generated")
		}
		seen[full] = true
	}
}

func TestSplitCredentialRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "nodot", ".onlysecret", "onlyid.", "a"} {
		if _, _, ok := splitCredential(bad); ok {
			t.Errorf("splitCredential(%q) should be malformed", bad)
		}
	}
	// A legacy pairing key (no dot) must be rejected as a machine credential —
	// this is what keeps the two bearer schemes unambiguous.
	if _, _, ok := splitCredential("legacy-pairing-key-no-dot"); ok {
		t.Error("a dotless pairing key must not parse as a machine credential")
	}
}

func TestBridgeBindingRoundTrip(t *testing.T) {
	s := openTestStore(t)
	_, id, hash, _ := newMachineCredential()
	s.UpsertUser(User{Sub: "alice", Email: "a@x.com"})
	if err := s.PutBinding(BridgeBinding{BridgeID: id, UserSub: "alice", CredHash: hash}); err != nil {
		t.Fatal(err)
	}
	b, err := s.GetBinding(id)
	if err != nil || b.UserSub != "alice" {
		t.Fatalf("GetBinding = (%+v,%v)", b, err)
	}
}

func TestEnrollTokenIsOneTime(t *testing.T) {
	s := openTestStore(t)
	s.UpsertUser(User{Sub: "bob", Email: "b@x.com"})
	th := hashSecret("the-token")
	s.PutEnrollToken(th, EnrollToken{UserSub: "bob", ExpiresAt: time.Now().Add(time.Hour)})

	sub, err := s.RedeemEnrollToken(th)
	if err != nil || sub != "bob" {
		t.Fatalf("first redeem = (%q,%v), want bob", sub, err)
	}
	// Second redemption must fail — one-time.
	if _, err := s.RedeemEnrollToken(th); err == nil {
		t.Fatal("a token must not be redeemable twice")
	}
}

func TestEnrollTokenExpires(t *testing.T) {
	s := openTestStore(t)
	s.UpsertUser(User{Sub: "carol", Email: "c@x.com"})
	th := hashSecret("expired-token")
	s.PutEnrollToken(th, EnrollToken{UserSub: "carol", ExpiresAt: time.Now().Add(-time.Minute)})
	if _, err := s.RedeemEnrollToken(th); err == nil {
		t.Fatal("an expired token must not redeem")
	}
}
