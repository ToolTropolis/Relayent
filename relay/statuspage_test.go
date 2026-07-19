// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Tests for the root ("/") route's mode-aware behaviour. On a
//
//	multi-tenant relay the pairing-key status prompt is a dead end (OIDC
//	deployments hand out no such key), so "/" routes by session: anonymous →
//	/login, admin → /admin, signed-in user → the status page (never a redirect,
//	or it would loop with /login). Single-key mode is unchanged: "/" always
//	serves the status page.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// oidcWithUser builds an oidcAuth backed by a store, seeded with one user, and
// returns it plus a signed session cookie for that user.
func oidcWithUser(t *testing.T, sub, role string) (*oidcAuth, *http.Cookie) {
	t.Helper()
	a := testOIDC(t)
	if err := a.store.UpsertUser(User{Sub: sub, Email: sub + "@x.com", Role: role}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	a.setSession(rec, sub)
	return a, rec.Result().Cookies()[0]
}

func getRoot(s *server, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	s.statusPage(rec, req)
	return rec
}

// Single-key mode (no store): "/" always serves the status page, never redirects.
func TestRoot_SingleKeyServesStatus(t *testing.T) {
	s := &server{} // nil store => legacy mode
	rec := getRoot(s, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("single-key root: want 200, got %d (Location=%q)", rec.Code, rec.Header().Get("Location"))
	}
}

// Multi-tenant + anonymous: "/" redirects to /login instead of an unfillable
// pairing-key prompt.
func TestRoot_MultiTenantAnonymousRedirectsToLogin(t *testing.T) {
	a, _ := oidcWithUser(t, "someone", RoleUser)
	s := &server{store: a.store, oidc: a}
	rec := getRoot(s, nil) // no session cookie
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("anon root: want 302 -> /login, got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

// Multi-tenant + admin session: "/" redirects to the admin console.
func TestRoot_MultiTenantAdminRedirectsToAdmin(t *testing.T) {
	a, cookie := oidcWithUser(t, "sub-admin", RoleAdmin)
	s := &server{store: a.store, oidc: a}
	rec := getRoot(s, cookie)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin" {
		t.Fatalf("admin root: want 302 -> /admin, got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

// Multi-tenant + signed-in non-admin: "/" must RENDER their own status page,
// not redirect — this is the target /login sends regular users to, so
// redirecting here would create a /login <-> / loop.
func TestRoot_MultiTenantUserSeesStatusNoLoop(t *testing.T) {
	a, cookie := oidcWithUser(t, "sub-user", RoleUser)
	s := &server{store: a.store, oidc: a}
	rec := getRoot(s, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed-in user root: want 200 (user page), got %d -> %q (a redirect here loops with /login)",
			rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), "Your Relayent status") {
		t.Errorf("signed-in user root should render the user status page, not the pairing-key page")
	}
}

// /status always serves the pairing-key global dashboard, even on a
// multi-tenant relay, for ops who authenticate with a key.
func TestClassicStatus_ServesKeyDashboard(t *testing.T) {
	a, _ := oidcWithUser(t, "someone", RoleUser)
	s := &server{store: a.store, oidc: a}
	req := httptest.NewRequest("GET", "/status", nil)
	rec := httptest.NewRecorder()
	s.classicStatusPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/status: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Pairing key") {
		t.Errorf("/status should render the pairing-key dashboard")
	}
}

// GET /v1/me requires an OIDC session — a request with no session is 401,
// never a data leak.
func TestMe_RequiresSession(t *testing.T) {
	a, _ := oidcWithUser(t, "sub-user", RoleUser)
	s := &server{store: a.store, oidc: a, q: NewQueue(time.Minute, time.Minute)}
	req := httptest.NewRequest("GET", "/v1/me", nil) // no cookie
	rec := httptest.NewRecorder()
	s.me(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/v1/me without session: want 401, got %d", rec.Code)
	}
}

// GET /v1/me is scoped to the caller's own session: the returned sub is the
// signed-in user's, and there is no request parameter to name another user.
func TestMe_ScopedToSession(t *testing.T) {
	a, cookie := oidcWithUser(t, "sub-user", RoleUser)
	s := &server{store: a.store, oidc: a, q: NewQueue(time.Minute, time.Minute)}
	// Even if a caller tries to name someone else, it is ignored — sub comes
	// from the session only.
	req := httptest.NewRequest("GET", "/v1/me?target_user=sub-admin", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.me(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/me with session: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"sub":"sub-user"`) {
		t.Errorf("/v1/me must report the caller's own sub, not a named one: %s", rec.Body.String())
	}
}
