// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: The relay's control-plane store — the FIRST durable state the
//
//	relay has ever held. It persists identity and bindings ONLY: users (from
//	OIDC, non-secret), issued machine credentials (stored HASHED), one-time
//	enrollment tokens (hashed), bridge→user bindings, and an append-only audit
//	log. The DATA plane — the job queue, prompts, results — stays in memory and
//	is NEVER written here, so no prompt or result content is ever at rest.
//
//	Backed by bbolt (go.etcd.io/bbolt): a pure-Go embedded key/value store, no
//	cgo and no libc emulation, so the relay stays a single static
//	CGO_ENABLED=0 binary. (An earlier attempt on modernc.org/sqlite was
//	abandoned: its pure-Go libc blocked indefinitely in some environments.)
//	Values are JSON — bbolt sees only opaque bytes. What we store is already
//	hashed or non-secret, so there is no plaintext credential at rest; the
//	store itself does no hashing (callers hash before writing) and no
//	encryption (there is nothing secret left to encrypt).
//
//	A nil *Store means legacy single-key mode with no persistence at all —
//	every method is a safe no-op — so the pre-existing deployment runs exactly
//	as before.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ErrNotFound is returned when a lookup has no row. Callers map it to 401/404 as
// appropriate; it is never surfaced verbatim to an unauthenticated caller.
var ErrNotFound = errors.New("not found")

// Bucket names. Each is a flat key/value namespace.
var (
	bktUsers    = []byte("users")           // sub          -> User (JSON)
	bktAppCreds = []byte("app_creds")       // id           -> AppCred (JSON)
	bktEnroll   = []byte("enroll_tokens")   // sha256(token)-> EnrollToken (JSON)
	bktBindings = []byte("bridge_bindings") // bridge_id    -> BridgeBinding (JSON)
	bktAudit    = []byte("audit")           // seq (uint64) -> AuditEvent (JSON)
	allBuckets  = [][]byte{bktUsers, bktAppCreds, bktEnroll, bktBindings, bktAudit}
)

// Store is the relay's durable control plane. All content (prompts, results) is
// deliberately excluded. A nil *Store is valid and means "legacy mode, no
// persistence"; every method tolerates it.
type Store struct {
	db *bolt.DB
}

// User is an OIDC-authenticated human. Nothing here is a secret: Sub is the
// stable OIDC subject id, and there is no password because OIDC never gives the
// relay one.
type User struct {
	Sub         string    `json:"sub"` // OIDC subject — stable id (email can change, sub cannot)
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"` // "admin" | "user"
	Disabled    bool      `json:"disabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// Roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// OpenStore opens (and initialises) the bbolt control-plane store at path.
// It never returns a nil store with a nil error.
func OpenStore(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open control-plane store: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("init control-plane store: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database. Safe on a nil store.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Enabled reports whether a real store backs this relay (vs legacy no-persistence).
func (s *Store) Enabled() bool { return s != nil && s.db != nil }

// --- users ---

// UpsertUser creates or updates a user from an OIDC login. Role, if empty,
// defaults to "user". An existing user's role is preserved (role changes are an
// explicit admin action, not a side effect of logging in again).
func (s *Store) UpsertUser(u User) error {
	if !s.Enabled() {
		return nil
	}
	if u.Role == "" {
		u.Role = RoleUser
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktUsers)
		if existing := b.Get([]byte(u.Sub)); existing != nil {
			var prev User
			if json.Unmarshal(existing, &prev) == nil {
				u.Role = prev.Role           // preserve role across re-login
				u.CreatedAt = prev.CreatedAt // preserve original creation time
			}
		}
		if u.CreatedAt.IsZero() {
			u.CreatedAt = time.Now()
		}
		return b.Put([]byte(u.Sub), mustJSON(u))
	})
}

// GetUser returns a user by OIDC subject, or ErrNotFound.
func (s *Store) GetUser(sub string) (User, error) {
	if !s.Enabled() {
		return User{}, ErrNotFound
	}
	var u User
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bktUsers).Get([]byte(sub))
		if v == nil {
			return ErrNotFound
		}
		return json.Unmarshal(v, &u)
	})
	return u, err
}

// ListUsers returns all users, newest first — for the admin surface.
func (s *Store) ListUsers() ([]User, error) {
	if !s.Enabled() {
		return nil, nil
	}
	var out []User
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bktUsers).ForEach(func(_, v []byte) error {
			var u User
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			out = append(out, u)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// CountUsers reports how many users exist — used to make the first-ever login an
// admin (bootstrap), and nowhere else.
func (s *Store) CountUsers() (int, error) {
	if !s.Enabled() {
		return 0, nil
	}
	var n int
	err := s.db.View(func(tx *bolt.Tx) error {
		n = tx.Bucket(bktUsers).Stats().KeyN
		return nil
	})
	return n, err
}

// SetUserDisabled disables/enables a user (admin action). A disabled user's
// access is refused, but the record is kept for audit continuity.
func (s *Store) SetUserDisabled(sub string, disabled bool) error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktUsers)
		v := b.Get([]byte(sub))
		if v == nil {
			return ErrNotFound
		}
		var u User
		if err := json.Unmarshal(v, &u); err != nil {
			return err
		}
		u.Disabled = disabled
		return b.Put([]byte(sub), mustJSON(u))
	})
}

// --- helpers ---

// mustJSON marshals a value that is known to be serialisable (our own structs);
// a failure here is a programming error, not a runtime condition.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("control-plane store: marshal %T: %v", v, err))
	}
	return b
}

// itob encodes a uint64 as a big-endian key, so bbolt's byte-ordered iteration
// matches numeric order (used for the append-only audit sequence).
func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}
