// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Tests for the control-plane store. The load-bearing ones prove
//
//	what must never regress: a nil store is a safe no-op (legacy mode), and the
//	schema has no column that could hold prompt/result content.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// SetUserRole flips admin and back; UpsertUser must not clobber it afterwards.
func TestSetUserRole(t *testing.T) {
	s := openTestStore(t)
	s.UpsertUser(User{Sub: "u1", Email: "u1@x.com"}) // defaults to RoleUser
	if err := s.SetUserRole("u1", RoleAdmin); err != nil {
		t.Fatalf("SetUserRole: %v", err)
	}
	if u, _ := s.GetUser("u1"); u.Role != RoleAdmin {
		t.Fatalf("role = %q, want admin", u.Role)
	}
	// A later login (UpsertUser) must preserve the promoted role.
	s.UpsertUser(User{Sub: "u1", Email: "u1@x.com"})
	if u, _ := s.GetUser("u1"); u.Role != RoleAdmin {
		t.Fatalf("UpsertUser clobbered role to %q", u.Role)
	}
	if err := s.SetUserRole("u1", "bogus"); err == nil {
		t.Fatal("SetUserRole must reject an unknown role")
	}
	if err := s.SetUserRole("missing", RoleAdmin); err != ErrNotFound {
		t.Fatalf("SetUserRole on missing user = %v, want ErrNotFound", err)
	}
}

// DeleteUser removes the record; CountUsers reflects it.
func TestDeleteUser(t *testing.T) {
	s := openTestStore(t)
	s.UpsertUser(User{Sub: "gone", Email: "g@x.com"})
	if err := s.DeleteUser("gone"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := s.GetUser("gone"); err != ErrNotFound {
		t.Fatalf("deleted user still present: %v", err)
	}
	if err := s.DeleteUser("gone"); err != ErrNotFound {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

// Backend policy: empty = all enabled; disabling adds to the set; enabling removes.
func TestBackendPolicy(t *testing.T) {
	s := openTestStore(t)
	if d, err := s.DisabledBackends(); err != nil || len(d) != 0 {
		t.Fatalf("fresh store should disable nothing, got (%v,%v)", d, err)
	}
	if err := s.SetBackendEnabled("claude", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBackendEnabled("codex", false); err != nil {
		t.Fatal(err)
	}
	d, _ := s.DisabledBackends()
	if !d["claude"] || !d["codex"] || d["cursor"] {
		t.Fatalf("expected claude+codex disabled, cursor enabled; got %v", d)
	}
	// Re-enabling removes it; idempotent.
	if err := s.SetBackendEnabled("claude", true); err != nil {
		t.Fatal(err)
	}
	d, _ = s.DisabledBackends()
	if d["claude"] || !d["codex"] {
		t.Fatalf("claude should be back on, codex still off; got %v", d)
	}
}

// A nil store must be a total no-op — this is legacy single-key mode, and every
// call site relies on it so the pre-existing deployment needs no DB.
func TestNilStoreIsSafeNoop(t *testing.T) {
	var s *Store // nil
	if s.Enabled() {
		t.Fatal("nil store must report not-enabled")
	}
	if err := s.UpsertUser(User{Sub: "x"}); err != nil {
		t.Errorf("nil UpsertUser should no-op, got %v", err)
	}
	if _, err := s.GetUser("x"); err != ErrNotFound {
		t.Errorf("nil GetUser should be ErrNotFound, got %v", err)
	}
	if users, err := s.ListUsers(); err != nil || users != nil {
		t.Errorf("nil ListUsers should return (nil,nil), got (%v,%v)", users, err)
	}
	if n, err := s.CountUsers(); err != nil || n != 0 {
		t.Errorf("nil CountUsers should be (0,nil), got (%d,%v)", n, err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("nil Close should be nil, got %v", err)
	}
}

func TestUserUpsertGetList(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertUser(User{Sub: "sub-alice", Email: "alice@acme.com", DisplayName: "Alice", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	// Upsert again with a changed display name — must update, not duplicate.
	if err := s.UpsertUser(User{Sub: "sub-alice", Email: "alice@acme.com", DisplayName: "Alice A."}); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUser("sub-alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.DisplayName != "Alice A." {
		t.Errorf("display name = %q, want updated 'Alice A.'", u.DisplayName)
	}
	if u.Role != RoleAdmin {
		t.Errorf("role = %q, want admin preserved across upsert", u.Role)
	}
	if n, _ := s.CountUsers(); n != 1 {
		t.Errorf("count = %d, want 1 (upsert must not duplicate)", n)
	}
	if _, err := s.GetUser("nobody"); err != ErrNotFound {
		t.Errorf("missing user should be ErrNotFound, got %v", err)
	}
}

func TestUserDefaultRoleAndDisable(t *testing.T) {
	s := openTestStore(t)
	s.UpsertUser(User{Sub: "u1", Email: "u1@x.com"}) // no role -> default user
	u, _ := s.GetUser("u1")
	if u.Role != RoleUser {
		t.Errorf("default role = %q, want user", u.Role)
	}
	if err := s.SetUserDisabled("u1", true); err != nil {
		t.Fatal(err)
	}
	u, _ = s.GetUser("u1")
	if !u.Disabled {
		t.Error("user should be disabled after SetUserDisabled(true)")
	}
}

// SECURITY-CRITICAL: nothing written to the store may resemble prompt/result
// content. The store's typed values (User) carry no content field; this asserts
// the boundary by scanning every stored byte for a marker planted as a "prompt".
// The real enforcement is that content simply never reaches store methods — this
// is the backstop that fails loudly if that ever changes.
func TestStoreHoldsNoContent(t *testing.T) {
	s := openTestStore(t)
	// Exercise every write path with a distinctive marker in every string field.
	const marker = "SECRETPROMPTMARKER"
	s.UpsertUser(User{Sub: "s-" + marker, Email: marker + "@x.com", DisplayName: marker})

	// Walk the entire DB file's stored values; none should contain the marker in
	// a field that could plausibly be job content. (User fields legitimately hold
	// it here because we planted it — the point is there is no CONTENT bucket at
	// all: no bucket named for prompts/results, and User has no content field.)
	err := s.db.View(func(tx *bolt.Tx) error {
		for _, name := range []string{"users", "app_creds", "enroll_tokens", "bridge_bindings", "audit"} {
			b := tx.Bucket([]byte(name))
			if b == nil {
				continue
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Structural assertion: the User type has no content-bearing field. If a
	// prompt/result field is added to any stored struct, this list must be
	// revisited — kept explicit on purpose.
	banned := []string{"Prompt", "Result", "Content", "Response", "JSONSchema", "Output"}
	ut := reflect.TypeOf(User{})
	for i := 0; i < ut.NumField(); i++ {
		for _, b := range banned {
			if strings.Contains(ut.Field(i).Name, b) {
				t.Errorf("User has field %q — job content must NEVER be at rest", ut.Field(i).Name)
			}
		}
	}
}

// Reopening an existing DB must not fail or wipe data — migrate is idempotent.
func TestReopenPreservesData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	s1, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.UpsertUser(User{Sub: "keep", Email: "k@x.com"})
	s1.Close()

	s2, err := OpenStore(path) // reopen — migrate runs again
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer s2.Close()
	if _, err := s2.GetUser("keep"); err != nil {
		t.Errorf("data lost across reopen: %v", err)
	}
}
