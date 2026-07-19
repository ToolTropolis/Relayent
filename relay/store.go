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
	"strings"
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
	bktSettings = []byte("settings")        // key          -> value (JSON) — global config
	allBuckets  = [][]byte{bktUsers, bktAppCreds, bktEnroll, bktBindings, bktAudit, bktSettings}
)

// settingDisabledBackends is the settings key holding the set of backend names an
// admin has switched OFF. A backend NOT in this set is enabled — so a fresh store
// exposes whatever bridges report, and disabling is an explicit, auditable action.
const settingDisabledBackends = "disabled_backends"

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

// AppCred is a credential issued to a server-side consumer (e.g. EngageHub). It
// authenticates the app and is scoped; the app names a target user per request.
// KeyHash is sha256 of the secret half — the raw secret is shown once at
// issuance and never stored.
type AppCred struct {
	ID        string    `json:"id"`       // public half; locates the record
	AppID     string    `json:"app_id"`   // human label, e.g. "engagehub"
	KeyHash   string    `json:"key_hash"` // sha256(secret); NEVER the raw secret
	Scopes    []string  `json:"scopes"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}

// BridgeBinding ties a bridge to exactly one user. A bridge presenting its
// credential is authenticated (CredHash) and resolved to UserSub — the only
// user whose jobs it may claim.
type BridgeBinding struct {
	BridgeID   string    `json:"bridge_id"` // public half of the bridge credential
	UserSub    string    `json:"user_sub"`  // the bound user
	CredHash   string    `json:"cred_hash"` // sha256(secret); NEVER the raw secret
	EnrolledAt time.Time `json:"enrolled_at"`
	LastSeen   time.Time `json:"last_seen"`
}

// EnrollToken is a one-time token an admin issues so a specific user's bridge
// can enrol. Stored under sha256(token); redeemed exactly once before expiry.
type EnrollToken struct {
	UserSub    string    `json:"user_sub"`
	ExpiresAt  time.Time `json:"expires_at"`
	RedeemedAt time.Time `json:"redeemed_at"` // zero until redeemed
}

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

// SetUserRole changes a user's role (admin|user). Unlike UpsertUser — which
// preserves an existing role so a normal login can't self-promote — this is the
// explicit, admin-only path to grant or revoke admin.
func (s *Store) SetUserRole(sub, role string) error {
	if !s.Enabled() {
		return nil
	}
	if role != RoleAdmin && role != RoleUser {
		return fmt.Errorf("invalid role %q", role)
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
		u.Role = role
		return b.Put([]byte(sub), mustJSON(u))
	})
}

// DeleteUser removes a user record. Their bridge bindings and app credentials
// are not touched here — revoke those separately. Used to clean up a
// mis-provisioned or test account.
func (s *Store) DeleteUser(sub string) error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktUsers)
		if b.Get([]byte(sub)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(sub))
	})
}

// --- backend policy (global) ---
//
// The relay stores the set of backend names an admin has DISABLED. A name absent
// from the set is enabled, so an empty/legacy store exposes whatever bridges
// report — disabling is always the explicit action. This gates what apps and the
// demo can reach (e.g. keep a public demo on `cursor` only, off paid subscriptions).

// DisabledBackends returns the set of disabled backend names (may be empty).
func (s *Store) DisabledBackends() (map[string]bool, error) {
	out := map[string]bool{}
	if !s.Enabled() {
		return out, nil
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bktSettings).Get([]byte(settingDisabledBackends))
		if v == nil {
			return nil
		}
		var names []string
		if err := json.Unmarshal(v, &names); err != nil {
			return err
		}
		for _, n := range names {
			out[n] = true
		}
		return nil
	})
	return out, err
}

// SetBackendEnabled flips one backend on or off, persisting the disabled set.
// nil store is a no-op (legacy mode has no admin to call this).
func (s *Store) SetBackendEnabled(name string, enabled bool) error {
	if !s.Enabled() {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("backend name is required")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktSettings)
		set := map[string]bool{}
		if v := b.Get([]byte(settingDisabledBackends)); v != nil {
			var names []string
			if err := json.Unmarshal(v, &names); err != nil {
				return err
			}
			for _, n := range names {
				set[n] = true
			}
		}
		if enabled {
			delete(set, name)
		} else {
			set[name] = true
		}
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		sort.Strings(names)
		return b.Put([]byte(settingDisabledBackends), mustJSON(names))
	})
}

// --- app credentials ---

// PutAppCred stores an issued app credential (KeyHash already computed by the
// caller — the store never sees the raw secret).
func (s *Store) PutAppCred(c AppCred) error {
	if !s.Enabled() {
		return nil
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktAppCreds).Put([]byte(c.ID), mustJSON(c))
	})
}

// GetAppCred returns an app credential by its public id, or ErrNotFound.
func (s *Store) GetAppCred(id string) (AppCred, error) {
	if !s.Enabled() {
		return AppCred{}, ErrNotFound
	}
	var c AppCred
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bktAppCreds).Get([]byte(id))
		if v == nil {
			return ErrNotFound
		}
		return json.Unmarshal(v, &c)
	})
	return c, err
}

// --- bridge bindings ---

// PutBinding stores a bridge→user binding (CredHash already computed by caller).
func (s *Store) PutBinding(b BridgeBinding) error {
	if !s.Enabled() {
		return nil
	}
	if b.EnrolledAt.IsZero() {
		b.EnrolledAt = time.Now()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktBindings).Put([]byte(b.BridgeID), mustJSON(b))
	})
}

// GetBinding returns a bridge binding by bridge id, or ErrNotFound.
func (s *Store) GetBinding(bridgeID string) (BridgeBinding, error) {
	if !s.Enabled() {
		return BridgeBinding{}, ErrNotFound
	}
	var b BridgeBinding
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bktBindings).Get([]byte(bridgeID))
		if v == nil {
			return ErrNotFound
		}
		return json.Unmarshal(v, &b)
	})
	return b, err
}

// TouchBinding records that a bridge was last seen now — best-effort presence.
// A failure here must not fail a job, so callers ignore its error.
func (s *Store) TouchBinding(bridgeID string) error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bktBindings)
		v := bkt.Get([]byte(bridgeID))
		if v == nil {
			return nil
		}
		var b BridgeBinding
		if err := json.Unmarshal(v, &b); err != nil {
			return err
		}
		b.LastSeen = time.Now()
		return bkt.Put([]byte(bridgeID), mustJSON(b))
	})
}

// ListBindingsForUser returns all bridges bound to a user — for the admin view.
func (s *Store) ListBindingsForUser(sub string) ([]BridgeBinding, error) {
	if !s.Enabled() {
		return nil, nil
	}
	var out []BridgeBinding
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bktBindings).ForEach(func(_, v []byte) error {
			var b BridgeBinding
			if err := json.Unmarshal(v, &b); err != nil {
				return err
			}
			if b.UserSub == sub {
				out = append(out, b)
			}
			return nil
		})
	})
	return out, err
}

// DeleteBinding removes a bridge binding by its public id — revoking that bridge.
// The bridge's credential stops resolving on its next request. Used to retire a
// lost/decommissioned bridge, or to clean up a stale binding.
func (s *Store) DeleteBinding(bridgeID string) error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bktBindings)
		if bkt.Get([]byte(bridgeID)) == nil {
			return ErrNotFound
		}
		return bkt.Delete([]byte(bridgeID))
	})
}

// ListAppCreds returns all app credentials (without secrets — only public ids
// and metadata) for the admin surface.
func (s *Store) ListAppCreds() ([]AppCred, error) {
	if !s.Enabled() {
		return nil, nil
	}
	var out []AppCred
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bktAppCreds).ForEach(func(_, v []byte) error {
			var c AppCred
			if err := json.Unmarshal(v, &c); err != nil {
				return err
			}
			c.KeyHash = "" // never expose the hash on the admin surface
			out = append(out, c)
			return nil
		})
	})
	return out, err
}

// RevokeAppCred marks an app credential revoked (admin action).
func (s *Store) RevokeAppCred(id string) error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bktAppCreds)
		v := bkt.Get([]byte(id))
		if v == nil {
			return ErrNotFound
		}
		var c AppCred
		if err := json.Unmarshal(v, &c); err != nil {
			return err
		}
		c.Revoked = true
		return bkt.Put([]byte(id), mustJSON(c))
	})
}

// --- enrollment tokens ---

// PutEnrollToken stores a one-time token under tokenHash (sha256 of the token).
func (s *Store) PutEnrollToken(tokenHash string, tok EnrollToken) error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktEnroll).Put([]byte(tokenHash), mustJSON(tok))
	})
}

// RedeemEnrollToken atomically checks and consumes a one-time token. It returns
// the bound user on success, or an error if the token is unknown, expired, or
// already redeemed. The redeem is a single write transaction, so two concurrent
// redemptions cannot both succeed.
func (s *Store) RedeemEnrollToken(tokenHash string) (userSub string, err error) {
	if !s.Enabled() {
		return "", ErrNotFound
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bktEnroll)
		v := bkt.Get([]byte(tokenHash))
		if v == nil {
			return ErrNotFound
		}
		var tok EnrollToken
		if err := json.Unmarshal(v, &tok); err != nil {
			return err
		}
		if !tok.RedeemedAt.IsZero() {
			return errors.New("enrollment token already used")
		}
		if time.Now().After(tok.ExpiresAt) {
			return errors.New("enrollment token expired")
		}
		tok.RedeemedAt = time.Now()
		userSub = tok.UserSub
		return bkt.Put([]byte(tokenHash), mustJSON(tok))
	})
	return userSub, err
}

// --- audit log ---

// AuditEvent is one entry in the append-only audit log. Its fields are IDs,
// enums, and bounded counters — there is DELIBERATELY no field through which a
// prompt or result could pass. "No content at rest" is a property of this type,
// not of discipline at the call site: you physically cannot log content because
// there is nowhere to put it.
type AuditEvent struct {
	Seq       uint64    `json:"seq"`
	TS        time.Time `json:"ts"`
	ActorSub  string    `json:"actor_sub"`  // who acted (user/app/admin id)
	Event     string    `json:"event"`      // enqueue | claim | result | cancel | enroll | admin_action
	JobID     string    `json:"job_id"`     // opaque id, never content
	TargetSub string    `json:"target_sub"` // whose namespace it affected
	Backend   string    `json:"backend"`    // e.g. "cursor"
	Model     string    `json:"model"`      // e.g. "auto"
	Status    string    `json:"status"`     // done | error | ""
	PromptLen int       `json:"prompt_len"` // BYTE COUNT only — never the bytes
	ResultLen int       `json:"result_len"` // BYTE COUNT only — never the bytes
}

// Audit events.
const (
	EvEnqueue = "enqueue"
	EvClaim   = "claim"
	EvResult  = "result"
	EvCancel  = "cancel"
	EvEnroll  = "enroll"
	EvAdmin   = "admin_action"
)

// Append writes an audit event with a monotonic sequence. Best-effort: audit
// must never fail a job, so callers ignore the error (but it is returned for
// tests). The append is a single write transaction.
func (s *Store) Append(e AuditEvent) error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bktAudit)
		seq, _ := bkt.NextSequence()
		e.Seq = seq
		if e.TS.IsZero() {
			e.TS = time.Now()
		}
		return bkt.Put(itob(seq), mustJSON(e))
	})
}

// RecentAudit returns up to limit newest audit events, optionally filtered to a
// target user (empty = all). For the admin activity/history view.
func (s *Store) RecentAudit(targetSub string, limit int) ([]AuditEvent, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var out []AuditEvent
	err := s.db.View(func(tx *bolt.Tx) error {
		cur := tx.Bucket(bktAudit).Cursor()
		for k, v := cur.Last(); k != nil && len(out) < limit; k, v = cur.Prev() {
			var e AuditEvent
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			if targetSub == "" || e.TargetSub == targetSub {
				out = append(out, e)
			}
		}
		return nil
	})
	return out, err
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
