// Primary author: Navjyot Nishant
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
	"testing"
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

// Multi-tenant + signed-in non-admin: "/" must RENDER the status page, not
// redirect — this is the target /login sends regular users to, so redirecting
// here would create a /login <-> / loop.
func TestRoot_MultiTenantUserSeesStatusNoLoop(t *testing.T) {
	a, cookie := oidcWithUser(t, "sub-user", RoleUser)
	s := &server{store: a.store, oidc: a}
	rec := getRoot(s, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed-in user root: want 200 (status page), got %d -> %q (a redirect here loops with /login)",
			rec.Code, rec.Header().Get("Location"))
	}
}
