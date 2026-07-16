// Package api defines the shared wire types for the Relayent /v1 HTTP contract.
//
// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Request/response structs shared by the Relayent relay and bridge.
//
//	The /v1 HTTP API (see openapi.yaml) is the only integration surface consumers
//	depend on; these types mirror that contract.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package api

// Job statuses.
const (
	StatusPending = "pending" // enqueued, not yet claimed or still running on the bridge
	StatusDone    = "done"    // bridge returned a successful result
	StatusError   = "error"   // bridge returned an error, or the job failed/expired
)

// EnqueueRequest is the body of POST /v1/jobs — a consuming app asks the bridge
// (via the relay) to run one AI generation on the user's local CLI subscription.
type EnqueueRequest struct {
	Backend    string `json:"backend"`               // "claude" | "codex" | "gemini" | "cursor"
	Model      string `json:"model,omitempty"`       // optional model override passed to the CLI
	Prompt     string `json:"prompt"`                // the user/content prompt
	System     string `json:"system,omitempty"`      // optional system instruction
	JSONSchema any    `json:"json_schema,omitempty"` // optional JSON Schema for structured output
}

// EnqueueResponse is returned by POST /v1/jobs.
type EnqueueResponse struct {
	JobID string `json:"job_id"`
}

// Job is what the bridge claims from GET /v1/jobs/next. It is the EnqueueRequest
// plus the server-assigned id.
type Job struct {
	ID         string `json:"id"`
	Backend    string `json:"backend"`
	Model      string `json:"model,omitempty"`
	Prompt     string `json:"prompt"`
	System     string `json:"system,omitempty"`
	JSONSchema any    `json:"json_schema,omitempty"`
}

// ResultRequest is the body of POST /v1/jobs/{id}/result — the bridge reporting
// back after running the CLI. Exactly one of JSON/Text is meaningful on success;
// Error is set (with OK=false) on failure.
type ResultRequest struct {
	OK    bool   `json:"ok"`
	JSON  any    `json:"json,omitempty"` // parsed object when json_schema was requested
	Text  string `json:"text,omitempty"` // raw text otherwise
	Error string `json:"error,omitempty"`
}

// JobResult is returned by GET /v1/jobs/{id} to the consuming app.
type JobResult struct {
	ID     string `json:"id"`
	Status string `json:"status"` // pending | done | error
	JSON   any    `json:"json,omitempty"`
	Text   string `json:"text,omitempty"`
	Error  string `json:"error,omitempty"`
}

// BridgeOnlineResponse is returned by GET /v1/bridge/online — lets an app
// fail fast when no bridge is currently polling for its pairing key.
type BridgeOnlineResponse struct {
	Online bool `json:"online"`
}

// BackendInfo describes one backend adapter as seen by a bridge.
// Ready == Supported && Installed: only then will jobs for it succeed.
type BackendInfo struct {
	Name      string `json:"name"`      // "claude" | "codex" | "gemini" | "cursor"
	Installed bool   `json:"installed"` // is the backing CLI present on the bridge host?
	Supported bool   `json:"supported"` // is the adapter implemented (not a stub)?
	Ready     bool   `json:"ready"`     // can this backend actually run jobs now?
	Model     string `json:"model,omitempty"`
}

// BridgeCapabilities is what a bridge reports about itself. The bridge sends this
// (via the ?caps query on the poll, or a dedicated register) so the relay — which
// cannot see the user's machine — can surface what backends are available.
type BridgeCapabilities struct {
	Version  string        `json:"version"`
	Hostname string        `json:"hostname,omitempty"`
	Backends []BackendInfo `json:"backends"`
}

// CapabilitiesResponse is returned by GET /v1/bridge/capabilities. Online is false
// (and Backends empty) when no bridge has reported for this pairing key recently.
type CapabilitiesResponse struct {
	Online       bool               `json:"online"`
	ReportedAt   string             `json:"reported_at,omitempty"` // RFC3339 of last report
	Capabilities BridgeCapabilities `json:"capabilities"`
}

// StatusResponse is returned by GET /v1/status — relay-level health/introspection.
type StatusResponse struct {
	Status         string `json:"status"`          // "ok"
	Version        string `json:"version"`         // relay build version
	UptimeSeconds  int64  `json:"uptime_seconds"`  // since process start
	BridgeOnline   bool   `json:"bridge_online"`   // is a bridge polling for the caller's key
	PendingJobs    int    `json:"pending_jobs"`    // queued jobs for the caller's key
	RequirePairing bool   `json:"require_pairing"` // is a fixed pairing key enforced

	// KeyFingerprint identifies the caller's key without revealing it, so a user
	// can confirm which key they are on. Never contains key material.
	KeyFingerprint string `json:"key_fingerprint,omitempty"`
	// KeyRetiring is true when the caller authenticated with a key that is being
	// rotated out — a warning that it will stop working once the operator drops it.
	KeyRetiring bool `json:"key_retiring,omitempty"`
	// RotationActive is true while the relay accepts more than one key.
	RotationActive bool `json:"rotation_active,omitempty"`
	// TLS reports whether this request arrived over an encrypted channel. Shown on
	// the status page so a user can see at a glance that their key and prompts are
	// not crossing the network in plaintext.
	TLS bool `json:"tls"`
	// NetworkReachable is true when the relay is not bound to loopback only.
	NetworkReachable bool `json:"network_reachable"`
}

// ErrorResponse is the uniform error envelope for 4xx/5xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
