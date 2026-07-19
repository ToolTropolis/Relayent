// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Tests for routeTarget — which user's namespace a job enqueues
//
//	into. The security-critical properties: an app must name a valid target,
//	and a self-routing principal (bridge/legacy) can NEVER enqueue for another
//	user.
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

func routeTestServer(t *testing.T) *server {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.UpsertUser(User{Sub: "alice", Email: "a@x.com"})
	s.UpsertUser(User{Sub: "bob", Email: "b@x.com"})
	return &server{store: s}
}

// An app must name a valid target user; it routes there.
func TestRoute_AppNamesTarget(t *testing.T) {
	s := routeTestServer(t)
	app := &Principal{Kind: KindApp, Scopes: []string{ScopeEnqueue}}

	got, err := s.routeTarget(app, "alice")
	if err != nil || got != "alice" {
		t.Fatalf("app->alice = (%q,%v), want alice", got, err)
	}
	if _, err := s.routeTarget(app, ""); err == nil {
		t.Error("app with no target_user must be rejected")
	}
	if _, err := s.routeTarget(app, "nobody"); err == nil {
		t.Error("app naming an unknown user must be rejected")
	}
}

// An app cannot route to a disabled user.
func TestRoute_AppCannotTargetDisabled(t *testing.T) {
	s := routeTestServer(t)
	s.store.SetUserDisabled("bob", true)
	app := &Principal{Kind: KindApp, Scopes: []string{ScopeEnqueue}}
	if _, err := s.routeTarget(app, "bob"); err == nil {
		t.Error("app must not enqueue for a disabled user")
	}
}

// SECURITY-CRITICAL: a bridge/legacy principal can only enqueue for ITSELF.
func TestRoute_SelfRoutingCannotSpoof(t *testing.T) {
	s := routeTestServer(t)
	bridge := &Principal{Kind: KindUserBridge, UserID: "alice", Scopes: []string{ScopeEnqueue}}

	// No target -> routes to self.
	if got, err := s.routeTarget(bridge, ""); err != nil || got != "alice" {
		t.Fatalf("bridge self-route = (%q,%v), want alice", got, err)
	}
	// Target == self -> ok.
	if got, err := s.routeTarget(bridge, "alice"); err != nil || got != "alice" {
		t.Fatalf("bridge target=self = (%q,%v), want alice", got, err)
	}
	// Target == someone else -> REJECTED. This is the anti-spoofing guard.
	if _, err := s.routeTarget(bridge, "bob"); err == nil {
		t.Fatal("a bridge credential must NOT enqueue for a different user")
	}
}

// A legacy principal routes to the legacy namespace and can't spoof either.
func TestRoute_LegacySelfOnly(t *testing.T) {
	s := routeTestServer(t)
	leg := legacyPrincipal("fp")
	if got, _ := s.routeTarget(leg, ""); got != KindLegacy {
		t.Errorf("legacy routes to %q, want legacy", got)
	}
	if _, err := s.routeTarget(leg, "alice"); err == nil {
		t.Error("legacy principal must not enqueue for a named user")
	}
}

// newReadReq builds a GET carrying ?target_user=<v> (omitted when empty), as the
// read-side handlers see it.
func newReadReq(target string) *http.Request {
	url := "/v1/jobs/x"
	if target != "" {
		url += "?target_user=" + target
	}
	return httptest.NewRequest("GET", url, nil)
}

// REGRESSION: an app enqueues into target_user's namespace, so it must resolve
// the SAME namespace to read/cancel/check presence — otherwise every read 404s.
func TestReadTarget_AppResolvesNamedUser(t *testing.T) {
	s := routeTestServer(t)
	app := &Principal{Kind: KindApp, Scopes: []string{ScopeEnqueue}}

	got, err := s.resolveReadTarget(app, newReadReq("alice"))
	if err != nil || got != "alice" {
		t.Fatalf("app read alice = (%q,%v), want alice", got, err)
	}
	if _, err := s.resolveReadTarget(app, newReadReq("")); err == nil {
		t.Error("app read with no target_user must be rejected")
	}
	if _, err := s.resolveReadTarget(app, newReadReq("nobody")); err == nil {
		t.Error("app read naming an unknown user must be rejected")
	}
	s.store.SetUserDisabled("bob", true)
	if _, err := s.resolveReadTarget(app, newReadReq("bob")); err == nil {
		t.Error("app must not read a disabled user's jobs")
	}
}

// SECURITY-CRITICAL: a self-routing principal reads only its own namespace and
// cannot name another user on the read side either.
func TestReadTarget_SelfRoutingCannotSpoof(t *testing.T) {
	s := routeTestServer(t)
	bridge := &Principal{Kind: KindUserBridge, UserID: "alice", Scopes: []string{ScopeClaim}}

	if got, err := s.resolveReadTarget(bridge, newReadReq("")); err != nil || got != "alice" {
		t.Fatalf("bridge self read = (%q,%v), want alice", got, err)
	}
	if got, err := s.resolveReadTarget(bridge, newReadReq("alice")); err != nil || got != "alice" {
		t.Fatalf("bridge read target=self = (%q,%v), want alice", got, err)
	}
	if _, err := s.resolveReadTarget(bridge, newReadReq("bob")); err == nil {
		t.Fatal("a bridge credential must NOT read a different user's jobs")
	}
}
