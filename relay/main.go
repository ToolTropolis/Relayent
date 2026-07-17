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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	keys       KeySet // accepted pairing keys (primary + retiring); empty = any key
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

	srv := &server{
		q:          NewQueue(jobTTL, onlineWindow),
		keys:       keys,
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
// Phase 1: pairing-key only, mapping to a legacy principal. Open-namespace mode
// (empty KeySet, loopback only) accepts any key, each its own legacy-kind
// namespace keyed by the key itself so distinct keys stay isolated as before.
func (s *server) authenticate(r *http.Request) *Principal {
	key := keyFromRequest(r)
	if key == "" {
		return nil
	}
	if s.keys.Empty() {
		// Loopback open-namespace: each distinct key is its own namespace.
		p := legacyPrincipal(keyFingerprint(key))
		p.UserID = key // preserve prior per-key isolation
		return p
	}
	if s.keys.Accepts(key) {
		return legacyPrincipal(keyFingerprint(key))
	}
	return nil
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) enqueue(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !p.Can(ScopeEnqueue) {
		writeErr(w, http.StatusForbidden, "this credential may not enqueue jobs")
		return
	}
	// Limit per identity, not per IP: the cost being protected is the target
	// user's subscription quota, bound to the user regardless of caller origin.
	if !s.jobLimit.allow(p.UserID) {
		writeErr(w, http.StatusTooManyRequests, "job rate limit exceeded")
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
	id := newID()
	s.q.Enqueue(p.UserID, id, api.Job{
		ID:         id,
		Backend:    req.Backend,
		Model:      req.Model,
		Prompt:     req.Prompt,
		System:     req.System,
		JSONSchema: req.JSONSchema,
	})
	writeJSON(w, http.StatusAccepted, api.EnqueueResponse{JobID: id})
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func (s *server) fetch(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	wait := time.Duration(0)
	if r.URL.Query().Get("wait") == "1" || strings.EqualFold(r.URL.Query().Get("wait"), "true") {
		wait = fetchWait
	}
	res, ok := s.q.Fetch(p.UserID, id, wait)
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
	prev, ok := s.q.Cancel(p.UserID, id)
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
	writeJSON(w, http.StatusOK, api.BridgeOnlineResponse{Online: s.q.BridgeOnline(p.UserID)})
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

// getCapabilities returns what the bridge last reported it supports.
func (s *server) getCapabilities(w http.ResponseWriter, r *http.Request, p *Principal) {
	caps, reportedAt, online := s.q.Capabilities(p.UserID)
	resp := api.CapabilitiesResponse{Online: online, Capabilities: caps}
	if !reportedAt.IsZero() {
		resp.ReportedAt = reportedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
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
