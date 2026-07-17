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

	// TargetUser routes the job to a specific user's bridge/subscription. Required
	// for an app credential serving many users; ignored for a bridge or legacy
	// principal, which already carry their own identity. It is the OIDC subject of
	// the target user (as an app learns from the admin/user directory).
	TargetUser string `json:"target_user,omitempty"`
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

	// Models lists identifiers accepted as EnqueueRequest.Model, so a consumer
	// can discover valid values rather than guess and fail at job time. Empty
	// when the backend cannot report them — that does not mean models are
	// unsupported, only undiscoverable; pass a name you know and it will work.
	Models []string `json:"models,omitempty"`
	// DefaultModel is what runs when Model is empty, when the backend reports one.
	DefaultModel string `json:"default_model,omitempty"`
	// ModelsProbed distinguishes a list obtained FROM the CLI (true — accurate
	// for this install) from a static declaration (false — a hint that may drift
	// with CLI releases). Treat a declared list as advisory, not exhaustive.
	ModelsProbed bool `json:"models_probed,omitempty"`
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

// --- admin surface (/v1/admin/*) ---

// AdminUser is a user as shown on the admin surface, with per-user activity but
// NEVER prompt/result content. Enrollment/credential secrets are never included.
type AdminUser struct {
	Sub          string `json:"sub"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	Disabled     bool   `json:"disabled"`
	BridgeOnline bool   `json:"bridge_online"` // is any bound bridge polling?
	PendingJobs  int    `json:"pending_jobs"`  // queued for this user
	Bridges      int    `json:"bridges"`       // number of enrolled bridges
}

// CreateUserRequest creates a user directly (for non-OIDC bootstrap/testing);
// normally users are created by their first OIDC login.
type CreateUserRequest struct {
	Sub         string `json:"sub"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role,omitempty"` // "admin" | "user" (default user)
}

// EnrollTokenRequest asks the relay to mint a one-time enrollment token for a
// user, which the admin sends to that user out-of-band.
type EnrollTokenRequest struct {
	UserSub    string `json:"user_sub"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"` // default 900 (15 min)
}

// EnrollTokenResponse returns the one-time token — shown ONCE.
type EnrollTokenResponse struct {
	Token     string `json:"token"`      // give this to the user; not recoverable
	ExpiresAt string `json:"expires_at"` // RFC3339
}

// CreateAppCredRequest issues an app credential (e.g. for EngageHub).
type CreateAppCredRequest struct {
	AppID  string   `json:"app_id"`
	Scopes []string `json:"scopes,omitempty"` // default ["enqueue"]
}

// CreateAppCredResponse returns the app credential — the secret is shown ONCE.
type CreateAppCredResponse struct {
	AppID      string `json:"app_id"`
	Credential string `json:"credential"` // "<id>.<secret>" — save this now
}

// ErrorResponse is the uniform error envelope for 4xx/5xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// EnrollRequest is the body of POST /v1/enroll — a bridge redeeming a one-time
// enrollment token (issued by an admin for a specific user) to obtain its own
// long-lived credential. The token is the only authentication; the endpoint is
// otherwise unauthenticated and rate-limited.
type EnrollRequest struct {
	Token string `json:"token"` // the one-time enrollment token
}

// EnrollResponse returns the bridge's newly issued credential. The full
// "<id>.<secret>" is shown ONCE and never recoverable — the relay stores only a
// hash. The bridge must save it (it replaces the pairing key).
type EnrollResponse struct {
	BridgeCredential string `json:"bridge_credential"` // "<id>.<secret>" — save this now
	UserEmail        string `json:"user_email"`        // who this bridge is bound to (for confirmation)
}

// CancelResponse is returned by DELETE /v1/jobs/{id}.
//
// Cancelled reports whether anything was actually stopped, and WasStatus says
// what the job was doing when the request arrived — the two together are what
// let a caller tell "we saved the work" from "we were too late". A job already
// claimed by a bridge cannot have its CLI killed (the relay cannot reach an
// outbound-only bridge), so Cancelled=true with WasStatus="running" means the
// caller stops waiting but the quota is already spent.
type CancelResponse struct {
	ID        string `json:"id"`
	Cancelled bool   `json:"cancelled"`
	WasStatus string `json:"was_status"` // pending | running | done | error
	Detail    string `json:"detail"`
}
