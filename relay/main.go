// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Relayent relay — a small stateless job broker exposing the /v1
//
//	HTTP API. Consuming apps enqueue AI jobs; a bridge on the user's machine
//	long-polls, runs the local CLI, and posts results back. Auth is a per-tenant
//	bearer "pairing key"; all jobs are scoped to that key.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ToolTropolis/Relayent/internal/api"
)

const (
	maxBodyBytes    = 1 << 20 // 1 MiB request cap
	defaultNextWait = 25 * time.Second
	maxNextWait     = 55 * time.Second
	fetchWait       = 90 * time.Second // how long GET /v1/jobs/{id} blocks for a result
	jobTTL          = 10 * time.Minute
	onlineWindow    = 40 * time.Second // bridge is "online" if it polled within this

	// Rate limits. Auth failures are limited per client IP to make credential
	// guessing impractical; the burst tolerates a human retyping a key.
	authRatePerSec = 0.2 // 1 failed auth per 5s, sustained
	authBurst      = 8.0

	// Job enqueues are limited per pairing key: a stolen or shared key still
	// cannot flood a bridge and burn the subscription quota. The bridge's own
	// long-poll is NOT limited — it reconnects every ~25s by design.
	jobRatePerSec = 1.0 // 1 job/sec sustained
	jobBurst      = 30.0
)

// Version is the relay build version, overridable at link time.
var Version = "1.0.0"

type server struct {
	q          *Queue
	keys       KeySet    // accepted pairing keys (primary + retiring); empty = any key
	store      *Store    // control-plane persistence; nil = legacy no-persistence mode
	oidc       *oidcAuth // human login via OIDC; nil = not configured
	adminToken string    // bootstrap admin bearer (RELAYENT_ADMIN_TOKEN); "" = disabled
	geoip      *geoIP    // offline IP→country for demo analytics; nil = no GeoIP DB configured
	startedAt  time.Time
	authLimit  *limiter // guards credential guessing (keyed by client IP)
	jobLimit   *limiter // guards job floods / quota burn (keyed by key fingerprint)
	trustProxy bool     // honour X-Forwarded-For (only behind a trusted reverse proxy)

	networkReachable bool // relay is not bound to loopback only
}

// requestIsTLS reports whether the request reached the relay over an encrypted
// channel. Behind a TLS-terminating proxy r.TLS is nil even though the client
// used HTTPS, so X-Forwarded-Proto is consulted — but only when the proxy is
// trusted, since any caller can otherwise forge that header and make an
// unencrypted relay claim it is secure.
func (s *server) requestIsTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if s.trustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "keygen":
			key, err := GenerateKey()
			if err != nil {
				log.Fatalf("[relayent-relay] keygen: %v", err)
			}
			fmt.Println(key)
			return
		case "rotate":
			// Rotation is easy to get wrong (drop the old key too early and every
			// bridge breaks), so print the whole overlap procedure, not just a key.
			if err := printRotationPlan(os.Getenv("RELAYENT_PAIRING_KEY")); err != nil {
				log.Fatalf("[relayent-relay] rotate: %v", err)
			}
			return
		case "-h", "--help", "help":
			fmt.Print(usage)
			return
		}
	}

	addr := envDefault("RELAYENT_LISTEN", ":8787")
	// Comma-separated: first key is primary, any others are retiring keys kept
	// valid during a rotation overlap. Bring your own key or use `keygen`.
	keys := ParseKeySet(os.Getenv("RELAYENT_PAIRING_KEY"))

	// Fail closed: a network-reachable relay without a strong key would let any
	// caller spend the user's CLI subscription. Refuse rather than warn.
	if err := validateKeySetPolicy(keys, addr, os.Getenv("RELAYENT_ALLOW_INSECURE") == "1"); err != nil {
		log.Fatalf("[relayent-relay] %v", err)
	}

	// The bootstrap admin token grants full admin scope, so on a network-reachable
	// relay it must be strong — the same floor as the pairing key. A weak admin
	// token would be a trivial path to minting credentials for any user.
	if tok := os.Getenv("RELAYENT_ADMIN_TOKEN"); tok != "" && !isLoopbackAddr(addr) &&
		os.Getenv("RELAYENT_ALLOW_INSECURE") != "1" && len(tok) < minKeyLen {
		log.Fatalf("[relayent-relay] RELAYENT_ADMIN_TOKEN is too short (%d chars, need >= %d) "+
			"for a network-reachable relay. Generate a strong one: relayent-relay keygen", len(tok), minKeyLen)
	}

	// Control-plane persistence is opt-in. With no RELAYENT_DATA_DIR the store is
	// nil and the relay runs exactly as before — no database, no disk state —
	// which is what the live single-key deployment does. Multi-tenant features
	// (users, enrollment, admin) require a store; without one they are simply
	// unavailable and the legacy pairing key remains the only auth.
	var store *Store
	if dir := os.Getenv("RELAYENT_DATA_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			log.Fatalf("[relayent-relay] create data dir: %v", err)
		}
		s, err := OpenStore(filepath.Join(dir, "relayent.db"))
		if err != nil {
			log.Fatalf("[relayent-relay] %v", err)
		}
		store = s
		defer store.Close()
	}

	// OIDC human login is opt-in (RELAYENT_OIDC_*). nil when unconfigured, so the
	// pairing key stays the only auth for the live deployment.
	oidcAuth, err := setupOIDC(context.Background(), store)
	if err != nil {
		log.Fatalf("[relayent-relay] %v", err)
	}

	// Offline IP→country for the demo visitor stats. Optional and best-effort: a
	// missing/unset DB just means no country breakdown (see geoip.go).
	geo := openGeoIP(os.Getenv("RELAYENT_GEOIP_DB"))
	if geo != nil {
		defer geo.Close()
	}

	srv := &server{
		q:          NewQueue(jobTTL, onlineWindow),
		keys:       keys,
		store:      store,
		oidc:       oidcAuth,
		adminToken: os.Getenv("RELAYENT_ADMIN_TOKEN"),
		geoip:      geo,
		startedAt:  time.Now(),
		authLimit:  newLimiter(authRatePerSec, authBurst),
		jobLimit:   newLimiter(jobRatePerSec, jobBurst),
		trustProxy: os.Getenv("RELAYENT_TRUST_PROXY") == "1",

		networkReachable: !isLoopbackAddr(addr),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", srv.health)
	mux.HandleFunc("POST /v1/jobs", srv.auth(srv.enqueue))
	mux.HandleFunc("GET /v1/jobs/next", srv.auth(srv.claimNext))
	mux.HandleFunc("POST /v1/jobs/{id}/result", srv.auth(srv.postResult))
	mux.HandleFunc("GET /v1/jobs/{id}", srv.auth(srv.fetch))
	mux.HandleFunc("DELETE /v1/jobs/{id}", srv.auth(srv.cancel))
	mux.HandleFunc("GET /v1/bridge/online", srv.auth(srv.bridgeOnline))
	mux.HandleFunc("GET /v1/status", srv.auth(srv.status))
	mux.HandleFunc("GET /v1/bridge/capabilities", srv.auth(srv.getCapabilities))
	mux.HandleFunc("POST /v1/bridge/capabilities", srv.auth(srv.postCapabilities))

	// Enrollment: a bridge redeems a one-time token for its credential. The token
	// is the auth, so this route is unauthenticated (but rate-limited + one-time).
	mux.HandleFunc("POST /v1/enroll", srv.enroll)

	// OIDC human login (only when configured). Unauthenticated by design — these
	// ARE the authentication flow.
	if srv.oidc != nil {
		mux.HandleFunc("GET /v1/auth/login", srv.oidc.handleLogin)
		mux.HandleFunc("GET /v1/auth/callback", srv.oidc.handleCallback)
		mux.HandleFunc("GET /v1/auth/logout", srv.oidc.handleLogout)
	}

	// Admin surface — every route requires the admin scope (an OIDC admin
	// session). A non-admin principal gets 403; an unauthenticated one gets 401.
	mux.HandleFunc("GET /v1/admin/users", srv.authorize(ScopeAdmin, srv.adminListUsers))
	mux.HandleFunc("POST /v1/admin/users", srv.authorize(ScopeAdmin, srv.adminCreateUser))
	mux.HandleFunc("POST /v1/admin/users/{sub}/disabled", srv.authorize(ScopeAdmin, srv.adminSetUserDisabled))
	mux.HandleFunc("POST /v1/admin/users/{sub}/role", srv.authorize(ScopeAdmin, srv.adminSetUserRole))
	mux.HandleFunc("DELETE /v1/admin/users/{sub}", srv.authorize(ScopeAdmin, srv.adminDeleteUser))
	mux.HandleFunc("POST /v1/admin/enroll-tokens", srv.authorize(ScopeAdmin, srv.adminIssueEnrollToken))
	mux.HandleFunc("GET /v1/admin/app-creds", srv.authorize(ScopeAdmin, srv.adminListAppCreds))
	mux.HandleFunc("POST /v1/admin/app-creds", srv.authorize(ScopeAdmin, srv.adminCreateAppCred))
	mux.HandleFunc("POST /v1/admin/app-creds/{id}/revoke", srv.authorize(ScopeAdmin, srv.adminRevokeAppCred))
	mux.HandleFunc("DELETE /v1/admin/app-creds/{id}", srv.authorize(ScopeAdmin, srv.adminDeleteAppCred))
	mux.HandleFunc("GET /v1/admin/audit", srv.authorize(ScopeAdmin, srv.adminAudit))
	mux.HandleFunc("GET /v1/admin/config", srv.authorize(ScopeAdmin, srv.adminConfig))
	mux.HandleFunc("GET /v1/admin/users/{sub}/bridges", srv.authorize(ScopeAdmin, srv.adminListUserBridges))
	mux.HandleFunc("DELETE /v1/admin/bridges/{id}", srv.authorize(ScopeAdmin, srv.adminRevokeBridge))
	mux.HandleFunc("GET /v1/admin/backends", srv.authorize(ScopeAdmin, srv.adminListBackends))
	mux.HandleFunc("POST /v1/admin/backends/{name}", srv.authorize(ScopeAdmin, srv.adminSetBackend))
	mux.HandleFunc("GET /v1/admin/demo-stats", srv.authorize(ScopeAdmin, srv.adminDemoStats))

	// Demo visitor analytics ingest. Authed by an app credential scoped to
	// demo-stats ONLY — it can write a content-free hit and nothing else.
	mux.HandleFunc("POST /v1/demo/hit", srv.authorize(ScopeDemoStats, srv.demoIngest))

	// The admin dashboard (multi-tenant only). The page itself is public HTML —
	// it authenticates its /v1/admin/* XHRs via the OIDC session cookie or a
	// pasted admin token, exactly as the status page does with the pairing key.
	mux.HandleFunc("GET /admin", srv.adminPage)

	// /login is the single human sign-in surface: OIDC button + bootstrap-token
	// entry, redirecting by role on success. Multi-tenant only (guarded in-handler).
	mux.HandleFunc("GET /login", srv.loginPage)

	// A signed-in regular user's own status, scoped to their OIDC session
	// (no target_user — you only ever see yourself). Backs the user status page.
	mux.HandleFunc("GET /v1/me", srv.me)

	// The pairing-key global status dashboard, always reachable here for ops who
	// authenticate with a key. On "/" it is served directly only in single-key mode.
	mux.HandleFunc("GET /status", srv.classicStatusPage)

	mux.HandleFunc("GET /", srv.statusPage)

	// Explicit timeouts: the default zero-value server has none, leaving it open
	// to slowloris-style connection exhaustion once it faces the internet.
	// WriteTimeout must exceed fetchWait, since long-polls legitimately hold a
	// response open for up to 90s.
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      fetchWait + 30*time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	mode := "network-reachable"
	if isLoopbackAddr(addr) {
		mode = "loopback-only"
	}
	switch {
	case keys.Empty():
		log.Printf("[relayent-relay] listening on %s (%s), NO fixed pairing key — any key accepted",
			addr, mode)
	case len(keys.retiring) > 0:
		// Surfacing fingerprints (never the keys) lets an operator confirm a
		// rotation is in progress and tell which key a bridge is still using.
		fps := make([]string, 0, len(keys.retiring))
		for _, k := range keys.retiring {
			fps = append(fps, keyFingerprint(k))
		}
		log.Printf("[relayent-relay] listening on %s (%s), primary key=%s, ROTATING — %d retiring key(s) still accepted: %s",
			addr, mode, keyFingerprint(keys.primary), len(keys.retiring), strings.Join(fps, ", "))
	default:
		log.Printf("[relayent-relay] listening on %s (%s), pairing key fingerprint=%s",
			addr, mode, keyFingerprint(keys.primary))
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("[relayent-relay] server error: %v", err)
	}
}

// keyFromRequest extracts the pairing key from the Authorization: Bearer header.
func keyFromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// auth wraps a handler, authenticating the request and passing a *Principal to
// the handler. Historically the handler received the raw pairing key; it now
// receives an identity, so routing can be per-user instead of per-key.
//
// In this (phase 1) build the only auth scheme is still the pairing key, which
// yields a legacy Principal — a single shared namespace, exactly as before.
// Later phases add OIDC sessions, app keys, and bridge credentials by producing
// richer principals here; nothing downstream changes.
//
// When RELAYENT_PAIRING_KEY is set the key must match it (constant-time);
// otherwise any non-empty key is accepted with its own namespace — a mode only
// permitted for a loopback relay, enforced at startup by validateKeyPolicy.
// Failed attempts are rate-limited per client IP, and a 401 never discloses
// whether the key was missing vs wrong.
func (s *server) auth(next func(http.ResponseWriter, *http.Request, *Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := s.authenticate(r)
		if p == nil {
			ip := clientIP(r, s.trustProxy)
			if !s.authLimit.allow(ip) {
				writeErr(w, http.StatusTooManyRequests, "too many failed attempts; slow down")
				return
			}
			writeErr(w, http.StatusUnauthorized, "invalid or missing credentials")
			return
		}
		next(w, r, p)
	}
}

// authorize wraps auth and additionally requires a scope, for endpoints not
// every principal may reach (e.g. admin). Returns 403 when authenticated but
// unscoped — distinct from 401, since the caller IS known.
func (s *server) authorize(scope string, next func(http.ResponseWriter, *http.Request, *Principal)) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		if !p.Can(scope) {
			writeErr(w, http.StatusForbidden, "this credential is not permitted to do that")
			return
		}
		next(w, r, p)
	})
}

// authenticate resolves a request to a Principal, or nil if unauthenticated.
// Schemes are tried in order of specificity:
//  1. OIDC session cookie      → a human (admin/user) principal
//  2. Bearer machine credential → a bridge or app principal (shape "<id>.<secret>")
//  3. Bearer pairing key        → a legacy principal (single shared namespace)
//
// A machine credential always contains a ".", which a pairing key never does, so
// the two bearer schemes are unambiguous. Each scheme just yields a richer
// Principal; nothing downstream changes.
func (s *server) authenticate(r *http.Request) *Principal {
	// A logged-in human, via the session cookie.
	if s.oidc != nil {
		if p := s.oidc.principalFromSession(r); p != nil {
			return p
		}
	}

	bearer := keyFromRequest(r)
	if bearer == "" {
		return nil
	}

	// Bootstrap admin token (RELAYENT_ADMIN_TOKEN): grants admin scope directly.
	// For initial setup before any OIDC admin exists, and for orgs not using
	// OIDC at all. Compared constant-time. Deliberately checked before the
	// machine-credential and pairing-key schemes.
	if s.adminToken != "" && checkKey(bearer, s.adminToken) {
		return &Principal{
			UserID: "bootstrap-admin", Kind: KindAdmin,
			Scopes: []string{ScopeAdmin}, KeyFP: keyFingerprint(bearer),
		}
	}

	// A relay-issued machine credential ("<id>.<secret>"), when a store exists.
	if s.store.Enabled() {
		if id, secret, ok := splitCredential(bearer); ok {
			if p := s.machinePrincipal(id, secret); p != nil {
				return p
			}
			// A dotted bearer that is not a valid credential is a failure — do
			// NOT fall through to the pairing key (which never contains a dot).
			return nil
		}
	}

	// Bearer pairing key (legacy / single-tenant).
	if s.keys.Empty() {
		// Loopback open-namespace: each distinct key is its own namespace.
		p := legacyPrincipal(keyFingerprint(bearer))
		p.UserID = bearer // preserve prior per-key isolation
		return p
	}
	if s.keys.Accepts(bearer) {
		return legacyPrincipal(keyFingerprint(bearer))
	}
	return nil
}

// machinePrincipal resolves a machine credential to a bridge or app principal,
// or nil. A bridge credential (its id is a known binding) is scoped to claim its
// bound user's jobs; an app credential is scoped as issued and routes by the
// target user named in each request (added in a later phase). The disabled/
// revoked checks make revocation take effect on the very next request.
func (s *server) machinePrincipal(id, secret string) *Principal {
	// Bridge credential?
	if b, err := s.store.GetBinding(id); err == nil {
		if !verifySecret(secret, b.CredHash) {
			return nil
		}
		if u, err := s.store.GetUser(b.UserSub); err != nil || u.Disabled {
			return nil
		}
		_ = s.store.TouchBinding(id) // best-effort presence
		return &Principal{
			UserID: b.UserSub, Kind: KindUserBridge,
			Scopes: []string{ScopeClaim}, KeyFP: keyFingerprint(id),
		}
	}
	// App credential?
	if c, err := s.store.GetAppCred(id); err == nil {
		if c.Revoked || !verifySecret(secret, c.KeyHash) {
			return nil
		}
		return &Principal{
			// UserID is set per-request from the target user (later phase); until
			// then an app principal has no default namespace of its own.
			Kind: KindApp, Scopes: c.Scopes, KeyFP: keyFingerprint(id),
		}
	}
	return nil
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// enroll redeems a one-time enrollment token and issues the bridge its own
// credential. The token IS the authentication (an admin issued it for a specific
// user), so this endpoint is otherwise unauthenticated — but rate-limited per IP
// and one-time, so a leaked or guessed token is single-use and slow to brute.
//
// The returned "<id>.<secret>" credential is shown ONCE; the relay stores only
// its hash. On success the bridge is permanently bound to the token's user.
func (s *server) enroll(w http.ResponseWriter, r *http.Request) {
	if !s.store.Enabled() {
		writeErr(w, http.StatusNotFound, "enrollment is not enabled on this relay")
		return
	}
	// Rate-limit like auth: a token is a credential, guessing it must be slow.
	ip := clientIP(r, s.trustProxy)
	if !s.authLimit.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts; slow down")
		return
	}

	var req api.EnrollRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		writeErr(w, http.StatusBadRequest, "token is required")
		return
	}

	// Redeem is atomic and one-time; a used/expired/unknown token all fail here.
	userSub, err := s.store.RedeemEnrollToken(hashSecret(req.Token))
	if err != nil {
		// Do not distinguish unknown / expired / used — all are "invalid token".
		writeErr(w, http.StatusUnauthorized, "invalid or expired enrollment token")
		return
	}
	user, err := s.store.GetUser(userSub)
	if err != nil || user.Disabled {
		writeErr(w, http.StatusForbidden, "the user for this token is not active")
		return
	}

	full, id, credHash, err := newMachineCredential()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue credential")
		return
	}
	if err := s.store.PutBinding(BridgeBinding{
		BridgeID: id, UserSub: userSub, CredHash: credHash,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not record binding")
		return
	}
	_ = s.store.Append(AuditEvent{
		ActorSub: userSub, Event: EvEnroll, TargetSub: userSub,
	})
	writeJSON(w, http.StatusOK, api.EnrollResponse{
		BridgeCredential: full,
		UserEmail:        user.Email,
	})
}

func (s *server) enqueue(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !p.Can(ScopeEnqueue) {
		writeErr(w, http.StatusForbidden, "this credential may not enqueue jobs")
		return
	}
	var req api.EnqueueRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Backend) == "" || strings.TrimSpace(req.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "backend and prompt are required")
		return
	}

	// Enforce the global backend policy at enqueue, not just in the UI: a disabled
	// backend is refused for everyone, so a direct API caller can't bypass it.
	if disabled, _ := s.store.DisabledBackends(); disabled[strings.ToLower(strings.TrimSpace(req.Backend))] {
		writeErr(w, http.StatusForbidden, "backend is disabled on this relay")
		return
	}

	// Resolve which user's bridge this job routes to.
	target, err := s.routeTarget(p, req.TargetUser)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Limit per target user, not per IP: the cost being protected is that user's
	// subscription quota, bound to the user regardless of which app called.
	if !s.jobLimit.allow(target) {
		writeErr(w, http.StatusTooManyRequests, "job rate limit exceeded")
		return
	}

	id := newID()
	s.q.Enqueue(target, id, api.Job{
		ID:         id,
		Backend:    req.Backend,
		Model:      req.Model,
		Prompt:     req.Prompt,
		System:     req.System,
		JSONSchema: req.JSONSchema,
	})
	// Audit: IDs, backend, model, and the prompt's LENGTH — never the prompt.
	_ = s.store.Append(AuditEvent{
		ActorSub: p.UserID, Event: EvEnqueue, JobID: id, TargetSub: target,
		Backend: req.Backend, Model: req.Model, PromptLen: len(req.Prompt),
	})
	writeJSON(w, http.StatusAccepted, api.EnqueueResponse{JobID: id})
}

// routeTarget decides which user's namespace a job enqueues into, and enforces
// that a principal cannot enqueue for a user it isn't allowed to:
//
//   - App principal (KindApp, no own UserID): MUST name target_user, which must
//     be an existing, enabled user. This is how one app serves many users.
//   - Bridge / legacy / OIDC principal (has its own UserID): routes to itself.
//     A target_user is accepted only if it equals its own id — naming a DIFFERENT
//     user is rejected, so a bridge credential can never spend another's quota.
func (s *server) routeTarget(p *Principal, targetUser string) (string, error) {
	targetUser = strings.TrimSpace(targetUser)

	if p.Kind == KindApp {
		if targetUser == "" {
			return "", fmt.Errorf("target_user is required for an app credential")
		}
		u, err := s.store.GetUser(targetUser)
		if err != nil || u.Disabled {
			return "", fmt.Errorf("target_user is not a known active user")
		}
		return targetUser, nil
	}

	// Self-routing principals: an explicit target must match, or be omitted.
	if targetUser != "" && targetUser != p.UserID {
		return "", fmt.Errorf("this credential may only enqueue for itself")
	}
	return p.UserID, nil
}

// resolveReadTarget decides whose namespace a READ/cancel/presence call operates
// on, mirroring routeTarget's authorization for the read side. An app enqueues
// into target_user's namespace, so it must name the SAME target_user (via the
// ?target_user= query) to fetch a result, cancel, or check presence — otherwise
// the lookup runs under the app's own id and every read 404s. Self-routing
// principals (bridge/legacy/OIDC) ignore the query and use their own id; naming a
// different user is rejected, so the anti-spoof guard is unchanged.
func (s *server) resolveReadTarget(p *Principal, r *http.Request) (string, error) {
	targetUser := strings.TrimSpace(r.URL.Query().Get("target_user"))

	if p.Kind == KindApp {
		if targetUser == "" {
			return "", fmt.Errorf("target_user query parameter is required for an app credential")
		}
		u, err := s.store.GetUser(targetUser)
		if err != nil || u.Disabled {
			return "", fmt.Errorf("target_user is not a known active user")
		}
		return targetUser, nil
	}

	if targetUser != "" && targetUser != p.UserID {
		return "", fmt.Errorf("this credential may only access its own jobs")
	}
	return p.UserID, nil
}

func (s *server) claimNext(w http.ResponseWriter, r *http.Request, p *Principal) {
	wait := defaultNextWait
	if v := r.URL.Query().Get("wait"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			wait = time.Duration(secs) * time.Second
			if wait > maxNextWait {
				wait = maxNextWait
			}
		}
	}
	job, ok := s.q.ClaimNext(p.UserID, wait)
	if !ok {
		w.WriteHeader(http.StatusNoContent) // 204: no job within the wait window
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *server) postResult(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	var res api.ResultRequest
	if !decode(w, r, &res) {
		return
	}
	if !s.q.SetResult(p.UserID, id, res) {
		writeErr(w, http.StatusNotFound, "unknown job for this identity")
		return
	}
	// Audit: the result's outcome + LENGTH — never the result text/json.
	status := api.StatusDone
	if !res.OK {
		status = api.StatusError
	}
	_ = s.store.Append(AuditEvent{
		ActorSub: p.UserID, Event: EvResult, JobID: id, TargetSub: p.UserID,
		Status: status, ResultLen: len(res.Text) + jsonLen(res.JSON),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

// jsonLen returns the serialised byte length of a value, for audit metrics. It
// returns only a COUNT — the bytes are computed and discarded.
func jsonLen(v any) int {
	if v == nil {
		return 0
	}
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}

func (s *server) fetch(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	target, err := s.resolveReadTarget(p, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	wait := time.Duration(0)
	if r.URL.Query().Get("wait") == "1" || strings.EqualFold(r.URL.Query().Get("wait"), "true") {
		wait = fetchWait
	}
	res, ok := s.q.Fetch(target, id, wait)
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown job for this identity")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// cancel abandons a job. See Queue.Cancel for the important caveat: a job a
// bridge has already claimed cannot be stopped — the relay has no way to reach
// an outbound-only bridge mid-job. The response says which happened so the
// caller is not misled about whether work (and quota) was actually saved.
func (s *server) cancel(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	target, err := s.resolveReadTarget(p, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	prev, ok := s.q.Cancel(target, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown job for this identity")
		return
	}
	switch prev {
	case api.StatusDone, api.StatusError:
		writeJSON(w, http.StatusOK, api.CancelResponse{
			ID: id, Cancelled: false, WasStatus: prev,
			Detail: "job had already finished; nothing to cancel",
		})
	case "pending":
		writeJSON(w, http.StatusOK, api.CancelResponse{
			ID: id, Cancelled: true, WasStatus: "pending",
			Detail: "job was still queued and has been removed; no bridge will run it",
		})
	default: // running
		writeJSON(w, http.StatusOK, api.CancelResponse{
			ID: id, Cancelled: true, WasStatus: "running",
			Detail: "a bridge had already claimed this job. The CLI it started cannot be " +
				"stopped by the relay, so the work and any quota it uses are already spent. " +
				"The job is marked cancelled and any late result will be discarded.",
		})
	}
}

func (s *server) bridgeOnline(w http.ResponseWriter, r *http.Request, p *Principal) {
	target, err := s.resolveReadTarget(p, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api.BridgeOnlineResponse{Online: s.q.BridgeOnline(target)})
}

// status reports relay-level health and this principal's view of the system,
// including its security posture (TLS, exposure, key rotation) so the status page
// can tell the user plainly whether their setup is safe.
//
// Rotation fields (KeyRetiring, RotationActive) are meaningful only for the
// legacy pairing-key scheme; for other principals they are simply false, since
// rotation is a property of the shared key, not of a user identity.
func (s *server) status(w http.ResponseWriter, r *http.Request, p *Principal) {
	resp := api.StatusResponse{
		Status:           "ok",
		Version:          Version,
		UptimeSeconds:    int64(time.Since(s.startedAt).Seconds()),
		BridgeOnline:     s.q.BridgeOnline(p.UserID),
		PendingJobs:      s.q.PendingCount(p.UserID),
		RequirePairing:   !s.keys.Empty(),
		KeyFingerprint:   p.KeyFP,
		TLS:              s.requestIsTLS(r),
		NetworkReachable: s.networkReachable,
	}
	if p.Kind == KindLegacy && !s.keys.Empty() {
		if key := keyFromRequest(r); key != "" {
			resp.KeyRetiring = s.keys.IsRetiring(key)
		}
		resp.RotationActive = len(s.keys.retiring) > 0
	}
	writeJSON(w, http.StatusOK, resp)
}

// getCapabilities returns what the bridge last reported it supports, minus any
// backend an admin has disabled globally. An app credential names the target user
// (?target_user=) as with the other read endpoints; a self-routing principal sees
// its own. Disabled backends are omitted entirely so a consumer (e.g. the demo's
// model dropdown) only ever sees what it is allowed to use.
func (s *server) getCapabilities(w http.ResponseWriter, r *http.Request, p *Principal) {
	target, err := s.resolveReadTarget(p, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	caps, reportedAt, online := s.q.Capabilities(target)
	caps.Backends = s.filterDisabledBackends(caps.Backends)
	resp := api.CapabilitiesResponse{Online: online, Capabilities: caps}
	if !reportedAt.IsZero() {
		resp.ReportedAt = reportedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// filterDisabledBackends drops backends an admin has switched off. In legacy mode
// (no store) there is no policy, so the list passes through. A store read error
// returns the list unchanged rather than hiding everything — enqueue independently
// enforces the policy, so a stale read here cannot let a disabled backend run.
func (s *server) filterDisabledBackends(in []api.BackendInfo) []api.BackendInfo {
	if !s.store.Enabled() {
		return in
	}
	disabled, err := s.store.DisabledBackends()
	if err != nil || len(disabled) == 0 {
		return in
	}
	out := in[:0:0]
	for _, b := range in {
		if !disabled[b.Name] {
			out = append(out, b)
		}
	}
	return out
}

// postCapabilities lets a bridge register what backends it has available. The relay
// cannot see the user's machine, so the bridge is the source of truth.
func (s *server) postCapabilities(w http.ResponseWriter, r *http.Request, p *Principal) {
	var caps api.BridgeCapabilities
	if !decode(w, r, &caps) {
		return
	}
	// Any authenticated caller can post this, and it is rendered on the status
	// page — store only known backends and bounded strings.
	s.q.ReportCapabilities(p.UserID, sanitizeCapabilities(caps))
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

// --- helpers ---

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		if err == io.EOF {
			writeErr(w, http.StatusBadRequest, "empty request body")
		} else {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		}
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, api.ErrorResponse{Error: msg})
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
