// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: Principal — the authenticated identity behind a request. It
//
//	replaces the raw pairing-key string that every handler used to receive.
//	Historically the pairing key played three roles at once: credential,
//	tenant identity, and routing namespace. Principal separates them: auth
//	decides WHO you are (UserID, Kind, Scopes), and the queue routes purely by
//	UserID. This is the seam the multi-tenant work is built on.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

// Principal kinds. A request is made by exactly one of these.
const (
	KindAdmin      = "admin"       // a human operator (OIDC), manages users, sees activity
	KindUserBridge = "user-bridge" // a bridge bound to one user, claims/returns that user's jobs
	KindApp        = "app"         // a server-side consumer enqueuing jobs for a target user
	KindLegacy     = "legacy"      // the pre-multi-tenant shared pairing key (back-compat)
)

// Scopes gate what a principal may do. Kept minimal and explicit.
//
// Admin-console access is tiered (see RoleAdmin/RoleOperator/RoleViewer in
// store.go). ScopeAdminView is the floor: it reaches every read-only endpoint
// (Users list, Audit, Settings, Demo stats) and is granted to all three
// console roles. The write scopes below gate one area each; a role's scope
// set is computed by scopesForRole.
const (
	ScopeEnqueue      = "enqueue"       // POST /v1/jobs
	ScopeClaim        = "claim"         // GET /v1/jobs/next, POST result, report capabilities
	ScopeAdmin        = "admin"         // full admin — every /v1/admin/* endpoint (bootstrap token, RoleAdmin)
	ScopeAdminView    = "admin-view"    // read-only /v1/admin/* (all three console roles)
	ScopeUsersEdit    = "users-edit"    // manage user role/disable/delete, mint enroll tokens (RoleAdmin only)
	ScopeCredsEdit    = "creds-edit"    // create/revoke/delete app credentials (RoleAdmin, RoleOperator)
	ScopeBridgesEdit  = "bridges-edit"  // revoke bridges (RoleAdmin, RoleOperator)
	ScopeBackendsEdit = "backends-edit" // toggle backend enabled state (RoleAdmin, RoleOperator)
	ScopeDemoStats    = "demo-stats"    // POST /v1/demo/hit — write a content-free visitor hit, nothing else
)

// scopesForRole computes a console principal's scopes from their role. Called
// fresh on every request (principalFromSession re-reads the user), so a role
// change or disable takes effect immediately, not at next login.
func scopesForRole(role string) []string {
	switch role {
	case RoleAdmin:
		return []string{ScopeAdmin, ScopeAdminView, ScopeUsersEdit, ScopeCredsEdit, ScopeBridgesEdit, ScopeBackendsEdit}
	case RoleOperator:
		return []string{ScopeAdminView, ScopeCredsEdit, ScopeBridgesEdit, ScopeBackendsEdit}
	case RoleViewer:
		return []string{ScopeAdminView}
	default:
		return nil
	}
}

// Principal is the authenticated identity behind a request. It is produced by
// the auth middleware and passed to every handler in place of the old raw key.
//
// UserID is the ONLY thing the queue routes on. For a user-bridge it is the
// bound user's id; for an app it is the request's target user; for legacy mode
// it is the constant "legacy" so the single-namespace behaviour is preserved
// exactly. KeyFP is a non-reversible fingerprint of the presented credential,
// used for rate-limit bucketing and audit — never the secret itself.
type Principal struct {
	UserID   string
	Kind     string
	Scopes   []string
	KeyFP    string
	BridgeID string // set only for a user-bridge credential; empty for app/legacy
}

// Can reports whether the principal holds a scope.
func (p *Principal) Can(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// legacyPrincipal is the identity used in backward-compatible single-key mode:
// one shared pairing key maps to one synthetic user, reproducing today's
// behaviour where every caller on a key shares one namespace. It holds every
// scope because the legacy key was, in effect, unrestricted.
func legacyPrincipal(keyFP string) *Principal {
	return &Principal{
		UserID: KindLegacy,
		Kind:   KindLegacy,
		Scopes: []string{ScopeEnqueue, ScopeClaim},
		KeyFP:  keyFP,
	}
}
