// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: A small public demo of Relayent — a single chat page whose model
//
//	dropdown is populated from the relay's capabilities API and whose messages
//	run as Relayent jobs on a user's local CLI subscription. It is a thin,
//	credential-holding PROXY: the browser never sees the app credential or the
//	relay URL; every call goes demo-server -> relay. What backends appear is
//	whatever the relay is configured to expose (admin backend policy), so the
//	operator controls exposure centrally — e.g. cursor only, off paid quota.
//
//	Deliberately dependency-free (stdlib only) and single-binary, matching the
//	relay/bridge. No fallback to any paid API: if no bridge is online or the
//	backend is disabled, the UI says so.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// config is read once at startup from the environment. The credential and relay
// URL stay server-side and are never sent to the browser.
type config struct {
	listen     string // demo bind address
	relayURL   string // base URL of the relay
	appCred    string // "<id>.<secret>" app credential (bearer)
	targetUser string // whose bridge/subscription runs the jobs
	defBackend string // pre-selected backend in the dropdown
	title      string // page title / heading
	trustProxy bool   // honour X-Forwarded-For for the visitor IP (only behind a trusted proxy)
}

func loadConfig() (config, error) {
	c := config{
		listen:     envOr("DEMO_LISTEN", ":8080"),
		relayURL:   strings.TrimRight(os.Getenv("DEMO_RELAY_URL"), "/"),
		appCred:    os.Getenv("DEMO_APP_CREDENTIAL"),
		targetUser: os.Getenv("DEMO_TARGET_USER"),
		defBackend: envOr("DEMO_DEFAULT_BACKEND", "cursor"),
		title:      envOr("DEMO_TITLE", "Relayent Playground"),
		trustProxy: os.Getenv("DEMO_TRUST_PROXY") == "1",
	}
	if c.relayURL == "" || c.appCred == "" || c.targetUser == "" {
		return c, fmt.Errorf("DEMO_RELAY_URL, DEMO_APP_CREDENTIAL and DEMO_TARGET_USER are all required")
	}
	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type server struct {
	cfg  config
	http *http.Client
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("relayent-demo: %v", err)
	}
	s := &server{cfg: cfg, http: &http.Client{Timeout: 130 * time.Second}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("GET /", s.handlePage)

	srv := &http.Server{
		Addr:              cfg.listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("[relayent-demo] listening on %s -> relay %s (user %s, default %s)",
		cfg.listen, cfg.relayURL, cfg.targetUser, cfg.defBackend)
	log.Fatal(srv.ListenAndServe())
}

// relayReq builds a request to the relay with the app credential attached.
func (s *server) relayReq(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.relayURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.appCred)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// handleModels returns the enabled backends+models the relay exposes for the
// target user. The demo only ever offers what the relay reports — so the admin's
// backend policy is the single source of truth for what's available here.
func (s *server) handleModels(w http.ResponseWriter, r *http.Request) {
	req, err := s.relayReq(r.Context(), "GET",
		"/v1/bridge/capabilities?target_user="+urlQuery(s.cfg.targetUser), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	resp, err := s.http.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "relay unreachable"})
		return
	}
	defer resp.Body.Close()

	var caps struct {
		Online       bool `json:"online"`
		Capabilities struct {
			Backends []struct {
				Name         string   `json:"name"`
				Ready        bool     `json:"ready"`
				Models       []string `json:"models"`
				DefaultModel string   `json:"default_model"`
			} `json:"backends"`
		} `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&caps); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "bad relay response"})
		return
	}

	// Only surface backends that are actually runnable (ready). The relay has
	// already stripped admin-disabled backends, so this just drops not-installed
	// stubs (e.g. a gemini placeholder).
	type outBackend struct {
		Name         string   `json:"name"`
		Models       []string `json:"models"`
		DefaultModel string   `json:"default_model"`
	}
	out := struct {
		Online         bool         `json:"online"`
		DefaultBackend string       `json:"default_backend"`
		Backends       []outBackend `json:"backends"`
	}{Online: caps.Online, DefaultBackend: s.cfg.defBackend}
	for _, b := range caps.Capabilities.Backends {
		if !b.Ready {
			continue
		}
		out.Backends = append(out.Backends, outBackend{Name: b.Name, Models: b.Models, DefaultModel: b.DefaultModel})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleChat runs one message as a Relayent job: enqueue with the chosen backend
// and target_user, then long-poll for the result. It never falls back to a paid
// API — if nothing is online or the backend is disabled, the relay's error is
// surfaced as-is.
func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Backend string `json:"backend"`
		Model   string `json:"model"`
		Prompt  string `json:"prompt"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(in.Backend) == "" || strings.TrimSpace(in.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "backend and prompt are required"})
		return
	}

	// 1. Enqueue.
	enqBody, _ := json.Marshal(map[string]any{
		"backend":     in.Backend,
		"model":       in.Model,
		"prompt":      in.Prompt,
		"target_user": s.cfg.targetUser,
	})
	enqReq, _ := s.relayReq(r.Context(), "POST", "/v1/jobs", enqBody)
	enqResp, err := s.http.Do(enqReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "relay unreachable"})
		return
	}
	defer enqResp.Body.Close()
	if enqResp.StatusCode != http.StatusAccepted {
		writeJSON(w, enqResp.StatusCode, relayError(enqResp))
		return
	}
	var enq struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(enqResp.Body).Decode(&enq); err != nil || enq.JobID == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "bad relay response"})
		return
	}

	// 2. Long-poll for the result (relay caps a single wait at ~90s).
	fetchReq, _ := s.relayReq(r.Context(), "GET",
		"/v1/jobs/"+urlPath(enq.JobID)+"?wait=1&target_user="+urlQuery(s.cfg.targetUser), nil)
	fetchResp, err := s.http.Do(fetchReq)
	if err != nil {
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "timed out waiting for a result"})
		return
	}
	defer fetchResp.Body.Close()
	if fetchResp.StatusCode != http.StatusOK {
		writeJSON(w, fetchResp.StatusCode, relayError(fetchResp))
		return
	}
	var res struct {
		Status string          `json:"status"`
		Text   string          `json:"text"`
		JSON   json.RawMessage `json:"json"`
		Error  string          `json:"error"`
	}
	if err := json.NewDecoder(fetchResp.Body).Decode(&res); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "bad relay response"})
		return
	}

	switch res.Status {
	case "done":
		reply := res.Text
		if len(res.JSON) > 0 && string(res.JSON) != "null" {
			reply = string(res.JSON)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "done", "reply": reply})
	case "error":
		writeJSON(w, http.StatusOK, map[string]string{"status": "error", "error": orDefault(res.Error, "the job failed")})
	default: // still pending after the wait window
		writeJSON(w, http.StatusOK, map[string]string{"status": "pending",
			"error": "still working — the model is taking a while. Try again."})
	}
}

// relayError extracts the relay's {"error":...} envelope, or a generic message.
func relayError(resp *http.Response) map[string]string {
	var e struct {
		Error string `json:"error"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(b, &e)
	return map[string]string{"error": orDefault(e.Error, fmt.Sprintf("relay returned %d", resp.StatusCode))}
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// urlQuery / urlPath are minimal escapers for the two values we interpolate into
// relay URLs (a user sub and a job id — both server-controlled, but escaped on
// principle).
func urlQuery(s string) string {
	return strings.NewReplacer(" ", "%20", "&", "%26", "?", "%3F", "#", "%23").Replace(s)
}
func urlPath(s string) string {
	return strings.NewReplacer(" ", "%20", "/", "%2F", "?", "%3F", "#", "%23").Replace(s)
}
