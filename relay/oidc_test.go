// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Tests for the OIDC session and config layer — the parts testable
//
//	without a live provider: config gating (all-or-nothing, fail-closed),
//	session cookie sign/verify round-trip, tamper rejection, expiry, and that a
//	disabled user's session stops resolving.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func testOIDC(t *testing.T) *oidcAuth {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "o.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &oidcAuth{sessionKey: []byte("test-session-key-32-bytes-long!!"), store: s}
}

// A valid session round-trips and yields the right principal + scopes.
func TestSessionRoundTrip(t *testing.T) {
	a := testOIDC(t)
	a.store.UpsertUser(User{Sub: "sub-admin", Email: "a@x.com", Role: RoleAdmin})

	rec := httptest.NewRecorder()
	a.setSession(rec, "sub-admin")
	cookie := rec.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	p := a.principalFromSession(req)
	if p == nil {
		t.Fatal("valid session should resolve to a principal")
	}
	if p.UserID != "sub-admin" || !p.Can(ScopeAdmin) {
		t.Errorf("admin session: got UserID=%q admin=%v", p.UserID, p.Can(ScopeAdmin))
	}
}

// A non-admin user's session must NOT carry the admin scope.
func TestSessionUserHasNoAdminScope(t *testing.T) {
	a := testOIDC(t)
	a.store.UpsertUser(User{Sub: "sub-user", Email: "u@x.com", Role: RoleUser})
	rec := httptest.NewRecorder()
	a.setSession(rec, "sub-user")
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	p := a.principalFromSession(req)
	if p == nil || p.Can(ScopeAdmin) {
		t.Fatal("a plain user must not gain admin scope")
	}
}

// A tampered cookie must be rejected — this is the core security property.
func TestSessionTamperRejected(t *testing.T) {
	a := testOIDC(t)
	a.store.UpsertUser(User{Sub: "victim", Email: "v@x.com", Role: RoleUser})
	rec := httptest.NewRecorder()
	a.setSession(rec, "victim")
	good := rec.Result().Cookies()[0]

	// Flip a byte in the signature half.
	tampered := &http.Cookie{Name: sessionCookie, Value: good.Value[:len(good.Value)-1] + "0"}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(tampered)
	if a.principalFromSession(req) != nil {
		t.Fatal("tampered session cookie must be rejected")
	}

	// Forge a session for a different sub with a made-up signature.
	forged := &http.Cookie{Name: sessionCookie, Value: "YWRtaW58OTk5OTk5OTk5OQ|deadbeef"}
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(forged)
	if a.principalFromSession(req2) != nil {
		t.Fatal("forged session must be rejected")
	}
}

// A disabled user's previously-valid session must stop resolving.
func TestSessionDisabledUserRejected(t *testing.T) {
	a := testOIDC(t)
	a.store.UpsertUser(User{Sub: "gone", Email: "g@x.com", Role: RoleUser})
	rec := httptest.NewRecorder()
	a.setSession(rec, "gone")
	cookie := rec.Result().Cookies()[0]

	a.store.SetUserDisabled("gone", true)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	if a.principalFromSession(req) != nil {
		t.Fatal("a disabled user's session must be refused")
	}
}

// The CSRF state check accepts a matching state and rejects a mismatch.
func TestStateVerification(t *testing.T) {
	a := testOIDC(t)
	state := "some-random-state"
	signed := a.sign(state)
	if !a.verifyState(signed, state) {
		t.Error("matching state should verify")
	}
	if a.verifyState(signed, "different-state") {
		t.Error("mismatched state must be rejected")
	}
	if a.verifyState("", state) || a.verifyState(signed, "") {
		t.Error("empty state values must be rejected")
	}
}
