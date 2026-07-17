// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: OIDC login for human principals (admins and users). The relay
//
//	never sees or stores a password: an OIDC provider (Google now; any issuer
//	later by changing one URL) authenticates the human, and the relay verifies
//	the returned id_token's signature locally against the provider's cached
//	public keys. From the verified token it records only NON-SECRET claims
//	(subject, email, name). This is what makes the stateful relay hold no human
//	credential at rest.
//
//	Sessions are a signed, httpOnly cookie carrying the user's subject — no
//	server-side session store needed. OIDC is entirely opt-in: with no
//	RELAYENT_OIDC_* config the auth object is nil and only legacy/pairing-key
//	auth exists, so the live deployment is unaffected.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// oidcAuth holds the configured OIDC provider and the session-signing secret.
// nil when OIDC is not configured.
type oidcAuth struct {
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	oauth        *oauth2.Config
	hostedDomain string // if set, only accounts in this Workspace domain (hd claim) are accepted
	providerName string // friendly issuer name for the UI, e.g. "Google"
	sessionKey   []byte // HMAC key for signing session cookies
	store        *Store
}

// providerName maps a known issuer URL to a friendly name for the login button,
// so it says "Sign in with Google" rather than a vague "SSO". Falls back to
// "SSO" for issuers we do not specifically recognise — truthful for any provider.
func providerName(issuer string) string {
	switch {
	case strings.Contains(issuer, "accounts.google.com"):
		return "Google"
	case strings.Contains(issuer, "login.microsoftonline.com"), strings.Contains(issuer, "sts.windows.net"):
		return "Microsoft"
	case strings.Contains(issuer, "okta.com"):
		return "Okta"
	case strings.Contains(issuer, "auth0.com"):
		return "Auth0"
	default:
		return "SSO"
	}
}

// sessionCookie is the cookie name for a logged-in human session.
const sessionCookie = "relayent_session"

// sessionTTL bounds how long a login lasts before re-authentication.
const sessionTTL = 12 * time.Hour

// setupOIDC configures OIDC from the environment, or returns (nil, nil) when it
// is not enabled. Requires a store — user records have nowhere to live without
// one, and a stateless OIDC relay would re-bootstrap admin on every login.
//
//	RELAYENT_OIDC_ISSUER        e.g. https://accounts.google.com (any OIDC issuer)
//	RELAYENT_OIDC_CLIENT_ID
//	RELAYENT_OIDC_CLIENT_SECRET
//	RELAYENT_OIDC_REDIRECT_URL  e.g. https://relay.example.com/v1/auth/callback
//	RELAYENT_OIDC_HOSTED_DOMAIN (optional) lock to a Google Workspace domain
func setupOIDC(ctx context.Context, store *Store) (*oidcAuth, error) {
	issuer := os.Getenv("RELAYENT_OIDC_ISSUER")
	clientID := os.Getenv("RELAYENT_OIDC_CLIENT_ID")
	clientSecret := os.Getenv("RELAYENT_OIDC_CLIENT_SECRET")
	redirect := os.Getenv("RELAYENT_OIDC_REDIRECT_URL")

	// All-or-nothing: if none set, OIDC is simply off.
	if issuer == "" && clientID == "" && clientSecret == "" && redirect == "" {
		return nil, nil
	}
	if issuer == "" || clientID == "" || clientSecret == "" || redirect == "" {
		return nil, fmt.Errorf("OIDC is partially configured: set all of " +
			"RELAYENT_OIDC_ISSUER, _CLIENT_ID, _CLIENT_SECRET, _REDIRECT_URL (or none)")
	}
	if !store.Enabled() {
		return nil, fmt.Errorf("OIDC requires RELAYENT_DATA_DIR: user accounts need somewhere to live")
	}

	// This is the one network call, at startup, to fetch the issuer's metadata
	// and JWKS. Verification thereafter is local against the cached keys.
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC issuer %q: %w", issuer, err)
	}

	sessionKey, err := loadOrCreateSessionKey()
	if err != nil {
		return nil, err
	}

	return &oidcAuth{
		provider:     provider,
		verifier:     provider.Verifier(&oidc.Config{ClientID: clientID}),
		hostedDomain: os.Getenv("RELAYENT_OIDC_HOSTED_DOMAIN"),
		providerName: providerName(issuer),
		sessionKey:   sessionKey,
		store:        store,
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirect,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
	}, nil
}

// loadOrCreateSessionKey returns a stable HMAC key for signing session cookies.
// From RELAYENT_SESSION_KEY if set; otherwise a random per-process key (sessions
// then reset on restart, which is acceptable — users just log in again).
func loadOrCreateSessionKey() ([]byte, error) {
	if v := os.Getenv("RELAYENT_SESSION_KEY"); v != "" {
		if len(v) < 16 {
			return nil, fmt.Errorf("RELAYENT_SESSION_KEY too short (need >= 16 chars)")
		}
		return []byte(v), nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}
	return b, nil
}

// --- login flow ---

// handleLogin starts the OIDC dance: redirect to the provider with a signed
// state parameter (CSRF protection) carried in a short-lived cookie.
func (a *oidcAuth) handleLogin(w http.ResponseWriter, r *http.Request) {
	state := randToken()
	http.SetCookie(w, &http.Cookie{
		Name: "relayent_oidc_state", Value: a.sign(state), Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 300,
	})
	opts := []oauth2.AuthCodeOption{}
	if a.hostedDomain != "" {
		opts = append(opts, oauth2.SetAuthURLParam("hd", a.hostedDomain))
	}
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, opts...), http.StatusFound)
}

// handleCallback completes the flow: verify state, exchange the code, verify the
// id_token, upsert the user (first-ever login becomes admin), set a session.
func (a *oidcAuth) handleCallback(w http.ResponseWriter, r *http.Request) {
	// CSRF: the state in the query must match the one we signed into the cookie.
	c, err := r.Cookie("relayent_oidc_state")
	if err != nil || !a.verifyState(c.Value, r.URL.Query().Get("state")) {
		writeErr(w, http.StatusBadRequest, "invalid or expired login state")
		return
	}
	oauth2Token, err := a.oauth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "OIDC code exchange failed")
		return
	}
	rawID, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		writeErr(w, http.StatusBadGateway, "OIDC response had no id_token")
		return
	}
	// Local signature + audience + expiry verification.
	idToken, err := a.verifier.Verify(r.Context(), rawID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "OIDC token verification failed")
		return
	}
	var claims struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		HD       string `json:"hd"`
		Verified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		writeErr(w, http.StatusBadGateway, "OIDC claims unreadable")
		return
	}
	// Optional Workspace-domain lock: reject accounts outside it.
	if a.hostedDomain != "" && claims.HD != a.hostedDomain {
		writeErr(w, http.StatusForbidden, "account is not in the permitted domain")
		return
	}

	// First user to ever log in bootstraps as admin; everyone after is a user.
	role := RoleUser
	if n, _ := a.store.CountUsers(); n == 0 {
		role = RoleAdmin
	}
	if err := a.store.UpsertUser(User{
		Sub: idToken.Subject, Email: claims.Email, DisplayName: claims.Name, Role: role,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not record user")
		return
	}

	a.setSession(w, idToken.Subject)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// handleLogout clears the session cookie.
func (a *oidcAuth) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: true, MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// --- session ---

// setSession issues a signed cookie: base64(sub|expiry)|hmac. Tamper-evident,
// so a modified sub or a past expiry is rejected without any server-side store.
func (a *oidcAuth) setSession(w http.ResponseWriter, sub string) {
	payload := fmt.Sprintf("%s|%d", sub, time.Now().Add(sessionTTL).Unix())
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "|" + a.sign(payload)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()),
	})
}

// principalFromSession resolves a valid session cookie to a Principal, or nil.
// The user's current role is read from the store each time, so a disable or a
// role change takes effect on the next request, not at next login.
//
// The cookie is base64(payload)|hmac(payload). It is rejected unless: it splits
// cleanly, the HMAC matches (constant-time), the payload parses, and the expiry
// is in the future. Each check is a separate early return so the logic is
// obvious — this is security-critical and must not be clever.
func (a *oidcAuth) principalFromSession(r *http.Request) *Principal {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	b64, mac, ok := strings.Cut(c.Value, "|")
	if !ok {
		return nil
	}
	rawBytes, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	payload := string(rawBytes)
	if !hmac.Equal([]byte(mac), []byte(a.sign(payload))) {
		return nil // tampered or forged
	}
	sub, expStr, ok := strings.Cut(payload, "|")
	if !ok {
		return nil
	}
	var exp int64
	if _, err := fmt.Sscanf(expStr, "%d", &exp); err != nil || time.Now().Unix() > exp {
		return nil // unparseable or expired
	}
	u, err := a.store.GetUser(sub)
	if err != nil || u.Disabled {
		return nil
	}
	var scopes []string
	if u.Role == RoleAdmin {
		scopes = []string{ScopeAdmin}
	}
	return &Principal{UserID: sub, Kind: KindAdmin, Scopes: scopes, KeyFP: keyFingerprint(sub)}
}

// --- crypto helpers ---

// sign returns hex(HMAC-SHA256(sessionKey, msg)).
func (a *oidcAuth) sign(msg string) string {
	m := hmac.New(sha256.New, a.sessionKey)
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

// verifyState checks that the login-state cookie (which holds sign(state))
// matches the plaintext state echoed back in the callback query, constant-time.
// This is the CSRF guard on the OIDC callback.
func (a *oidcAuth) verifyState(signedCookie, plainFromQuery string) bool {
	if signedCookie == "" || plainFromQuery == "" {
		return false
	}
	return hmac.Equal([]byte(signedCookie), []byte(a.sign(plainFromQuery)))
}

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
