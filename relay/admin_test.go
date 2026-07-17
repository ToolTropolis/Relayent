// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Tests for the admin surface — that admin actions work and, above
//
//	all, that the activity view carries NO prompt/result content (the
//	operator-not-observer boundary).
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToolTropolis/Relayent/internal/api"
)

func adminTestServer(t *testing.T) *server {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &server{store: st, q: NewQueue(time.Minute, time.Minute)}
}

func adminReq(method, body string) *http.Request {
	return httptest.NewRequest(method, "/", strings.NewReader(body))
}

// SECURITY-CRITICAL: the admin activity view must NEVER contain job content.
func TestAdminActivityHasNoContent(t *testing.T) {
	s := adminTestServer(t)
	s.store.UpsertUser(User{Sub: "alice", Email: "a@x.com"})
	s.q.Enqueue("alice", "j1", api.Job{ID: "j1", Backend: "cursor", Prompt: "TOPSECRETPROMPT"})

	admin := &Principal{Kind: KindAdmin, Scopes: []string{ScopeAdmin}}
	rec := httptest.NewRecorder()
	s.adminListUsers(rec, adminReq("GET", ""), admin)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "TOPSECRETPROMPT") {
		t.Fatal("admin activity leaked prompt content — must never happen")
	}
	// But the pending count IS visible (activity without content).
	if !strings.Contains(rec.Body.String(), `"pending_jobs":1`) {
		t.Errorf("expected pending_jobs:1 in the activity view, got %s", rec.Body)
	}
}

func TestAdminCreateUserAndIssueToken(t *testing.T) {
	s := adminTestServer(t)
	admin := &Principal{Kind: KindAdmin, Scopes: []string{ScopeAdmin}}

	rec := httptest.NewRecorder()
	s.adminCreateUser(rec, adminReq("POST", `{"sub":"bob","email":"b@x.com"}`), admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body)
	}
	rec2 := httptest.NewRecorder()
	s.adminIssueEnrollToken(rec2, adminReq("POST", `{"user_sub":"bob"}`), admin)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), `"token"`) {
		t.Fatalf("issue token: %d %s", rec2.Code, rec2.Body)
	}
	rec3 := httptest.NewRecorder()
	s.adminIssueEnrollToken(rec3, adminReq("POST", `{"user_sub":"ghost"}`), admin)
	if rec3.Code != http.StatusBadRequest {
		t.Errorf("token for unknown user should be 400, got %d", rec3.Code)
	}
}

// An issued app credential lists WITHOUT its hash/secret.
func TestAdminAppCredListHasNoSecret(t *testing.T) {
	s := adminTestServer(t)
	admin := &Principal{Kind: KindAdmin, Scopes: []string{ScopeAdmin}}

	rec := httptest.NewRecorder()
	s.adminCreateAppCred(rec, adminReq("POST", `{"app_id":"engagehub"}`), admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("create app cred: %d %s", rec.Code, rec.Body)
	}
	rec2 := httptest.NewRecorder()
	s.adminListAppCreds(rec2, adminReq("GET", ""), admin)
	body := rec2.Body.String()
	if strings.Contains(body, "key_hash") && !strings.Contains(body, `"key_hash":""`) {
		t.Error("app cred listing must not expose the key hash")
	}
	if !strings.Contains(body, "engagehub") {
		t.Errorf("expected the app in the listing, got %s", body)
	}
}
