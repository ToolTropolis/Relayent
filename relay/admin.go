// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: The ops-admin API (/v1/admin/*). An admin — a human with the
//
//	admin role, authenticated via an OIDC session — manages users, issues
//	one-time enrollment tokens and app credentials, and views per-user activity
//	and bridge presence. It deliberately NEVER exposes prompt/result content:
//	the admin is an operator, not an observer of what users ask. Every route is
//	gated by authorize(ScopeAdmin), so a non-admin principal gets 403.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ToolTropolis/Relayent/internal/api"
)

// adminListUsers returns every user with per-user activity — but no content.
func (s *server) adminListUsers(w http.ResponseWriter, r *http.Request, p *Principal) {
	users, err := s.store.ListUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list users")
		return
	}
	out := make([]api.AdminUser, 0, len(users))
	for _, u := range users {
		bridges, _ := s.store.ListBindingsForUser(u.Sub)
		out = append(out, api.AdminUser{
			Sub:          u.Sub,
			Email:        u.Email,
			DisplayName:  u.DisplayName,
			Role:         u.Role,
			Disabled:     u.Disabled,
			BridgeOnline: s.q.BridgeOnline(u.Sub),
			PendingJobs:  s.q.PendingCount(u.Sub),
			Bridges:      len(bridges),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// adminCreateUser creates a user directly. Normally the first OIDC login does
// this; the endpoint exists for bootstrap and for pre-provisioning.
func (s *server) adminCreateUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req api.CreateUserRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Sub) == "" || strings.TrimSpace(req.Email) == "" {
		writeErr(w, http.StatusBadRequest, "sub and email are required")
		return
	}
	role := req.Role
	if role != RoleAdmin {
		role = RoleUser
	}
	if err := s.store.UpsertUser(User{
		Sub: req.Sub, Email: req.Email, DisplayName: req.DisplayName, Role: role,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"sub": req.Sub})
}

// adminSetUserDisabled disables or re-enables a user. A disabled user's bridge
// and app-targeted jobs are refused immediately (role is re-checked per request).
func (s *server) adminSetUserDisabled(w http.ResponseWriter, r *http.Request, p *Principal) {
	sub := r.PathValue("sub")
	disabled := r.URL.Query().Get("disabled") != "false" // default true
	if err := s.store.SetUserDisabled(sub, disabled); err != nil {
		writeErr(w, http.StatusNotFound, "unknown user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sub": sub, "disabled": disabled})
}

// adminSetUserRole grants or revokes admin. Body: {"role":"admin"|"user"}. This
// is the only path that changes a role — a normal OIDC login can't self-promote,
// since UpsertUser preserves an existing role. An admin cannot demote themselves,
// so the deployment can't be locked out of its last admin by accident.
func (s *server) adminSetUserRole(w http.ResponseWriter, r *http.Request, p *Principal) {
	sub := r.PathValue("sub")
	var req api.SetUserRoleRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Role != RoleAdmin && req.Role != RoleUser {
		writeErr(w, http.StatusBadRequest, "role must be \"admin\" or \"user\"")
		return
	}
	if sub == p.UserID && req.Role != RoleAdmin {
		writeErr(w, http.StatusBadRequest, "an admin cannot demote themselves")
		return
	}
	if err := s.store.SetUserRole(sub, req.Role); err != nil {
		writeErr(w, http.StatusNotFound, "unknown user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sub": sub, "role": req.Role})
}

// adminDeleteUser removes a user record. An admin cannot delete themselves.
func (s *server) adminDeleteUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	sub := r.PathValue("sub")
	if sub == p.UserID {
		writeErr(w, http.StatusBadRequest, "an admin cannot delete themselves")
		return
	}
	if err := s.store.DeleteUser(sub); err != nil {
		writeErr(w, http.StatusNotFound, "unknown user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sub": sub, "status": "deleted"})
}

// adminIssueEnrollToken mints a one-time enrollment token for a user. The admin
// sends it to that user out-of-band; the user's bridge redeems it at /v1/enroll.
func (s *server) adminIssueEnrollToken(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req api.EnrollTokenRequest
	if !decode(w, r, &req) {
		return
	}
	if _, err := s.store.GetUser(req.UserSub); err != nil {
		writeErr(w, http.StatusBadRequest, "unknown user_sub")
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	token := randToken()
	expires := time.Now().Add(ttl)
	if err := s.store.PutEnrollToken(hashSecret(token), EnrollToken{
		UserSub: req.UserSub, ExpiresAt: expires,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, api.EnrollTokenResponse{
		Token:     token,
		ExpiresAt: expires.UTC().Format(time.RFC3339),
	})
}

// adminCreateAppCred issues an app credential (e.g. for EngageHub). The secret
// is returned once; the relay stores only its hash.
func (s *server) adminCreateAppCred(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req api.CreateAppCredRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.AppID) == "" {
		writeErr(w, http.StatusBadRequest, "app_id is required")
		return
	}
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{ScopeEnqueue}
	}
	full, id, keyHash, err := newMachineCredential()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue credential")
		return
	}
	if err := s.store.PutAppCred(AppCred{
		ID: id, AppID: req.AppID, KeyHash: keyHash, Scopes: scopes,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not record credential")
		return
	}
	writeJSON(w, http.StatusOK, api.CreateAppCredResponse{AppID: req.AppID, Credential: full})
}

// adminListAppCreds lists app credentials (ids + metadata, no secrets/hashes).
func (s *server) adminListAppCreds(w http.ResponseWriter, r *http.Request, p *Principal) {
	creds, err := s.store.ListAppCreds()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"app_creds": creds})
}

// adminAudit returns the recent audit log — per-user history without content.
// Optional ?user=<sub> filters to one user; ?limit=<n> bounds the count.
func (s *server) adminAudit(w http.ResponseWriter, r *http.Request, p *Principal) {
	user := r.URL.Query().Get("user")
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	events, err := s.store.RecentAudit(user, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read audit log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// adminConfig returns a read-only snapshot of the relay's effective config for
// the Configure view. It exposes NO secrets — only booleans for whether each
// secret is set, plus non-secret identifiers (issuer, client id, redirect). The
// values are env/compose-driven; the UI shows them but cannot change them.
func (s *server) adminConfig(w http.ResponseWriter, r *http.Request, p *Principal) {
	cfg := api.AdminConfig{
		Version:       Version,
		Listen:        envDefault("RELAYENT_LISTEN", ":8787"),
		TrustProxy:    s.trustProxy,
		StoreEnabled:  s.store.Enabled(),
		DataDir:       os.Getenv("RELAYENT_DATA_DIR"),
		PairingKeySet: !s.keys.Empty(),
		AdminTokenSet: s.adminToken != "",
		OIDCEnabled:   s.oidc != nil,
	}
	if s.oidc != nil {
		cfg.OIDCIssuer = s.oidc.issuer
		cfg.OIDCClientID = s.oidc.oauth.ClientID
		cfg.OIDCRedirect = s.oidc.oauth.RedirectURL
		cfg.OIDCProvider = s.oidc.providerName
		cfg.HostedDomain = s.oidc.hostedDomain
	}
	writeJSON(w, http.StatusOK, cfg)
}

// adminListBackends reports every known backend and whether it is enabled. This
// is the global exposure policy — a disabled backend is hidden from apps and the
// demo and refused at enqueue, so a public demo can be kept to (say) cursor only,
// off the operator's paid subscriptions.
func (s *server) adminListBackends(w http.ResponseWriter, r *http.Request, p *Principal) {
	disabled, err := s.store.DisabledBackends()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read backend policy")
		return
	}
	out := make([]api.AdminBackend, 0, len(knownBackendList))
	for _, name := range knownBackendList {
		out = append(out, api.AdminBackend{Name: name, Enabled: !disabled[name]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"backends": out})
}

// adminSetBackend enables or disables one backend globally. Body: {"enabled":bool}.
func (s *server) adminSetBackend(w http.ResponseWriter, r *http.Request, p *Principal) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if !knownBackends[name] {
		writeErr(w, http.StatusNotFound, "unknown backend")
		return
	}
	var req api.SetBackendRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.SetBackendEnabled(name, req.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not update backend policy")
		return
	}
	writeJSON(w, http.StatusOK, api.AdminBackend{Name: name, Enabled: req.Enabled})
}

// adminRevokeAppCred revokes an app credential by its public id.
func (s *server) adminRevokeAppCred(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.store.RevokeAppCred(id); err != nil {
		writeErr(w, http.StatusNotFound, "unknown credential")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "revoked"})
}
