// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Relayent relay — a small stateless job broker exposing the /v1
//   HTTP API. Consuming apps enqueue AI jobs; a bridge on the user's machine
//   long-polls, runs the local CLI, and posts results back. Auth is a per-tenant
//   bearer "pairing key"; all jobs are scoped to that key.
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/navjyotnishant/relayent/internal/api"
)

const (
	maxBodyBytes    = 1 << 20 // 1 MiB request cap
	defaultNextWait = 25 * time.Second
	maxNextWait     = 55 * time.Second
	fetchWait       = 90 * time.Second // how long GET /v1/jobs/{id} blocks for a result
	jobTTL          = 10 * time.Minute
	onlineWindow    = 40 * time.Second // bridge is "online" if it polled within this
)

// Version is the relay build version, overridable at link time.
var Version = "1.0.0"

type server struct {
	q         *Queue
	pairKey   string // the single accepted pairing key (from env); "" = allow any non-empty key
	startedAt time.Time
}

func main() {
	addr := envDefault("RELAYENT_LISTEN", ":8787")
	srv := &server{
		q:         NewQueue(jobTTL, onlineWindow),
		pairKey:   os.Getenv("RELAYENT_PAIRING_KEY"),
		startedAt: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", srv.health)
	mux.HandleFunc("POST /v1/jobs", srv.auth(srv.enqueue))
	mux.HandleFunc("GET /v1/jobs/next", srv.auth(srv.claimNext))
	mux.HandleFunc("POST /v1/jobs/{id}/result", srv.auth(srv.postResult))
	mux.HandleFunc("GET /v1/jobs/{id}", srv.auth(srv.fetch))
	mux.HandleFunc("GET /v1/bridge/online", srv.auth(srv.bridgeOnline))
	mux.HandleFunc("GET /v1/status", srv.auth(srv.status))
	mux.HandleFunc("GET /v1/bridge/capabilities", srv.auth(srv.getCapabilities))
	mux.HandleFunc("POST /v1/bridge/capabilities", srv.auth(srv.postCapabilities))
	mux.HandleFunc("GET /", srv.statusPage)

	log.Printf("[relayent-relay] listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
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

// auth wraps a handler, enforcing a non-empty pairing key. When RELAYENT_PAIRING_KEY
// is set, the key must match it exactly; otherwise any non-empty key is accepted
// (each distinct key gets its own isolated job namespace). The key is stashed on
// the request context via a header the handlers read back.
func (s *server) auth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := keyFromRequest(r)
		if key == "" {
			writeErr(w, http.StatusUnauthorized, "missing bearer pairing key")
			return
		}
		if s.pairKey != "" && key != s.pairKey {
			writeErr(w, http.StatusUnauthorized, "invalid pairing key")
			return
		}
		next(w, r, key)
	}
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) enqueue(w http.ResponseWriter, r *http.Request, key string) {
	var req api.EnqueueRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Backend) == "" || strings.TrimSpace(req.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "backend and prompt are required")
		return
	}
	id := newID()
	s.q.Enqueue(key, id, api.Job{
		ID:         id,
		Backend:    req.Backend,
		Model:      req.Model,
		Prompt:     req.Prompt,
		System:     req.System,
		JSONSchema: req.JSONSchema,
	})
	writeJSON(w, http.StatusAccepted, api.EnqueueResponse{JobID: id})
}

func (s *server) claimNext(w http.ResponseWriter, r *http.Request, key string) {
	wait := defaultNextWait
	if v := r.URL.Query().Get("wait"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			wait = time.Duration(secs) * time.Second
			if wait > maxNextWait {
				wait = maxNextWait
			}
		}
	}
	job, ok := s.q.ClaimNext(key, wait)
	if !ok {
		w.WriteHeader(http.StatusNoContent) // 204: no job within the wait window
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *server) postResult(w http.ResponseWriter, r *http.Request, key string) {
	id := r.PathValue("id")
	var res api.ResultRequest
	if !decode(w, r, &res) {
		return
	}
	if !s.q.SetResult(key, id, res) {
		writeErr(w, http.StatusNotFound, "unknown job for this pairing key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func (s *server) fetch(w http.ResponseWriter, r *http.Request, key string) {
	id := r.PathValue("id")
	wait := time.Duration(0)
	if r.URL.Query().Get("wait") == "1" || strings.EqualFold(r.URL.Query().Get("wait"), "true") {
		wait = fetchWait
	}
	res, ok := s.q.Fetch(key, id, wait)
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown job for this pairing key")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) bridgeOnline(w http.ResponseWriter, r *http.Request, key string) {
	writeJSON(w, http.StatusOK, api.BridgeOnlineResponse{Online: s.q.BridgeOnline(key)})
}

// status reports relay-level health and this pairing key's view of the system.
func (s *server) status(w http.ResponseWriter, r *http.Request, key string) {
	writeJSON(w, http.StatusOK, api.StatusResponse{
		Status:         "ok",
		Version:        Version,
		UptimeSeconds:  int64(time.Since(s.startedAt).Seconds()),
		BridgeOnline:   s.q.BridgeOnline(key),
		PendingJobs:    s.q.PendingCount(key),
		RequirePairing: s.pairKey != "",
	})
}

// getCapabilities returns what the bridge last reported it supports.
func (s *server) getCapabilities(w http.ResponseWriter, r *http.Request, key string) {
	caps, reportedAt, online := s.q.Capabilities(key)
	resp := api.CapabilitiesResponse{Online: online, Capabilities: caps}
	if !reportedAt.IsZero() {
		resp.ReportedAt = reportedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// postCapabilities lets a bridge register what backends it has available. The relay
// cannot see the user's machine, so the bridge is the source of truth.
func (s *server) postCapabilities(w http.ResponseWriter, r *http.Request, key string) {
	var caps api.BridgeCapabilities
	if !decode(w, r, &caps) {
		return
	}
	s.q.ReportCapabilities(key, caps)
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
