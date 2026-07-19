// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Relay security: pairing-key validation (constant-time, entropy
//
//	floor), per-key + per-IP rate limiting, and security response headers.
//	The relay is the only internet-facing component, so it is the only place
//	an attacker can reach — everything here exists to keep an unauthenticated
//	caller from reaching a user's CLI subscription.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ToolTropolis/Relayent/internal/api"
)

// minKeyLen is the shortest pairing key accepted when the relay is reachable off
// the loopback interface. 24 chars of base64url is ~144 bits — far past brute
// force, while still short enough to paste. Keys are compared in constant time,
// so the only attack left is guessing, and this makes guessing hopeless.
const minKeyLen = 24

// GenerateKey returns a cryptographically random pairing key (256 bits of
// entropy, base64url, no padding). This is what `relayent-relay keygen` prints.
func GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// keyFingerprint returns a short, non-reversible identifier for a key, safe to
// log or display. The raw key is a credential and must never be logged.
func keyFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:8]
}

// isLoopbackAddr reports whether a listen address binds only to loopback.
// Binding to localhost means the relay is unreachable from the network, which
// is the one case where a weak/absent pairing key is not a real exposure.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.TrimSuffix(addr, ":")
	}
	switch host {
	case "localhost":
		return true
	case "", "0.0.0.0", "::", "[::]":
		// Empty host means "all interfaces" — reachable from the network.
		return false
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// validateKeyPolicy enforces the startup security policy: a relay reachable from
// the network MUST have a strong fixed pairing key. Without one, any caller can
// invent a key, get their own namespace, and route jobs to whichever bridge is
// polling with it — spending the user's subscription. Refusing to start is the
// only safe default; a warning would be ignored precisely when it matters most.
func validateKeyPolicy(pairKey, listenAddr string, allowInsecure bool) error {
	if isLoopbackAddr(listenAddr) {
		return nil // not reachable off-box; nothing to protect against
	}
	if allowInsecure {
		return nil // explicit, deliberate opt-out (e.g. auth handled upstream)
	}
	if pairKey == "" {
		return fmt.Errorf(
			"refusing to start: RELAYENT_PAIRING_KEY is not set and %s is reachable from the network.\n"+
				"  Without a fixed key, ANY caller can use this relay and spend your CLI subscription.\n"+
				"  Generate one:  relayent-relay keygen\n"+
				"  Then:          RELAYENT_PAIRING_KEY=<key> relayent-relay\n"+
				"  Localhost-only instead:  RELAYENT_LISTEN=127.0.0.1:8787\n"+
				"  To bypass (only if auth is enforced upstream): RELAYENT_ALLOW_INSECURE=1",
			listenAddr)
	}
	if len(pairKey) < minKeyLen {
		return fmt.Errorf(
			"refusing to start: RELAYENT_PAIRING_KEY is too short (%d chars, need >= %d) for a network-reachable relay.\n"+
				"  A guessable key is the same as no key. Generate a strong one:  relayent-relay keygen",
			len(pairKey), minKeyLen)
	}
	return nil
}

// checkKey compares a presented key against the configured one in constant time.
// A plain == would leak the key byte-by-byte through timing; over enough requests
// that is a practical attack, and it costs nothing to avoid.
func checkKey(presented, configured string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(configured)) == 1
}

// --- machine credentials (bridge + app) ---

// A machine credential is "<id>.<secret>": the id is public and locates the
// stored record; the secret is verified against a stored sha256 hash. Splitting
// them lets the relay find the right record in O(1) without scanning, while
// never storing the secret — a stolen store yields only useless hashes.

// newMachineCredential mints a credential, returning the full "<id>.<secret>"
// to hand out ONCE, and the sha256(secret) to store. The id is 12 bytes and the
// secret 32 bytes of crypto-random, both base64url.
func newMachineCredential() (full, id, secretHash string, err error) {
	idBytes := make([]byte, 12)
	secretBytes := make([]byte, 32)
	if _, err = rand.Read(idBytes); err != nil {
		return "", "", "", fmt.Errorf("generate credential id: %w", err)
	}
	if _, err = rand.Read(secretBytes); err != nil {
		return "", "", "", fmt.Errorf("generate credential secret: %w", err)
	}
	id = base64.RawURLEncoding.EncodeToString(idBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	return id + "." + secret, id, hashSecret(secret), nil
}

// splitCredential parses "<id>.<secret>"; ok is false if malformed.
func splitCredential(cred string) (id, secret string, ok bool) {
	id, secret, ok = strings.Cut(cred, ".")
	if !ok || id == "" || secret == "" {
		return "", "", false
	}
	return id, secret, true
}

// hashSecret returns the base64url sha256 of a credential secret. This is what
// is stored; the raw secret never is.
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// verifySecret constant-time compares a presented secret against a stored hash.
func verifySecret(presentedSecret, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(hashSecret(presentedSecret)), []byte(storedHash)) == 1
}

// --- key rotation ---

// KeySet is the set of pairing keys a relay accepts. Rotation without downtime
// needs an overlap window: if only one key were ever valid, changing it would
// disconnect every bridge the instant the relay restarted. So the set holds one
// primary (the key to hand out now) plus any number of retiring keys that still
// work until their owners have moved across and you drop them.
//
// An empty KeySet means "accept any non-empty key, each in its own namespace" —
// only legal for a loopback relay (see validateKeyPolicy).
type KeySet struct {
	primary  string
	retiring []string
}

// ParseKeySet reads a comma-separated key list: the first is primary, the rest
// are retiring. Whitespace around entries is tolerated because these come from
// hand-edited env files and a stray space would otherwise silently break auth.
func ParseKeySet(raw string) KeySet {
	var ks KeySet
	for _, part := range strings.Split(raw, ",") {
		if k := strings.TrimSpace(part); k != "" {
			if ks.primary == "" {
				ks.primary = k
			} else {
				ks.retiring = append(ks.retiring, k)
			}
		}
	}
	return ks
}

// Empty reports whether no fixed key is configured (open-namespace mode).
func (ks KeySet) Empty() bool { return ks.primary == "" }

// All returns every accepted key, primary first.
func (ks KeySet) All() []string {
	if ks.primary == "" {
		return nil
	}
	return append([]string{ks.primary}, ks.retiring...)
}

// Accepts reports whether presented matches any key in the set. Every candidate
// is compared in constant time, and the loop deliberately does not short-circuit
// on the first match: bailing early would leak, through timing, *which* key
// matched and how many keys are configured.
func (ks KeySet) Accepts(presented string) bool {
	if presented == "" {
		return false
	}
	matched := false
	for _, k := range ks.All() {
		if checkKey(presented, k) {
			matched = true
		}
	}
	return matched
}

// IsRetiring reports whether presented is a valid but soon-to-be-removed key.
// Handlers use this to mark responses so an operator can see which bridges have
// yet to migrate before dropping the old key.
func (ks KeySet) IsRetiring(presented string) bool {
	for _, k := range ks.retiring {
		if checkKey(presented, k) {
			return true
		}
	}
	return false
}

// validateKeySetPolicy applies validateKeyPolicy to every key in the set: a
// rotation must not be a way to smuggle a weak key onto a public relay.
func validateKeySetPolicy(ks KeySet, listenAddr string, allowInsecure bool) error {
	if ks.Empty() {
		return validateKeyPolicy("", listenAddr, allowInsecure)
	}
	for _, k := range ks.All() {
		if err := validateKeyPolicy(k, listenAddr, allowInsecure); err != nil {
			return err
		}
	}
	return nil
}

// printRotationPlan generates a new primary key and prints the exact two-phase
// procedure for adopting it without downtime. Rotation's failure mode is dropping
// the old key while bridges still use it, so the tool spells out the overlap
// rather than just handing over a key and hoping.
func printRotationPlan(currentRaw string) error {
	newKey, err := GenerateKey()
	if err != nil {
		return err
	}
	cur := ParseKeySet(currentRaw)

	fmt.Println("New pairing key (primary):")
	fmt.Println()
	fmt.Println("  " + newKey)
	fmt.Println()

	if cur.Empty() {
		fmt.Println("No current RELAYENT_PAIRING_KEY is set in this environment, so there is")
		fmt.Println("nothing to rotate away from. Start the relay with:")
		fmt.Println()
		fmt.Printf("  RELAYENT_PAIRING_KEY=%s relayent-relay\n", newKey)
		fmt.Println()
		fmt.Println("Then give the same key to each bridge. Keep it secret: anyone holding it")
		fmt.Println("can spend the CLI subscription of any bridge paired with it.")
		return nil
	}

	fmt.Println("Rotate WITHOUT downtime — the old key keeps working until you remove it.")
	fmt.Println()
	fmt.Println("Step 1 — restart the relay accepting BOTH keys (new first, old second):")
	fmt.Println()
	fmt.Printf("  RELAYENT_PAIRING_KEY=%s,%s relayent-relay\n", newKey, strings.Join(cur.All(), ","))
	fmt.Println()
	fmt.Println("Step 2 — update each bridge to the new key and restart it:")
	fmt.Println()
	fmt.Printf("  RELAYENT_PAIRING_KEY=%s relayent-bridge\n", newKey)
	fmt.Println()
	fmt.Println("  Watch GET /v1/status (or the status page): while a bridge still uses an")
	fmt.Println("  old key, key_retiring is true. Once every bridge reports false, continue.")
	fmt.Println()
	fmt.Println("Step 3 — drop the old key(s) and restart the relay:")
	fmt.Println()
	fmt.Printf("  RELAYENT_PAIRING_KEY=%s relayent-relay\n", newKey)
	fmt.Println()
	fmt.Println("Rotate immediately (skipping the overlap) if a key may be compromised:")
	fmt.Println("that disconnects every bridge at once, which is the correct trade there.")
	fmt.Println()
	fmt.Printf("Current key fingerprint(s) being retired: %s\n", strings.Join(fingerprints(cur.All()), ", "))
	return nil
}

// fingerprints maps keys to their short display fingerprints.
func fingerprints(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyFingerprint(k))
	}
	return out
}

// usage is the relay's CLI help.
const usage = `relayent-relay — job broker for the Relayent /v1 API

USAGE
  relayent-relay            Start the relay
  relayent-relay keygen     Print a new cryptographically random pairing key
  relayent-relay rotate     Generate a new key and print the zero-downtime rotation steps
  relayent-relay help       Show this help

ENVIRONMENT
  RELAYENT_LISTEN          Listen address (default :8787). Use 127.0.0.1:8787 for
                           localhost-only, which is the safest default.
  RELAYENT_PAIRING_KEY     Accepted pairing key(s), comma-separated. The first is
                           primary; the rest are retiring keys still honoured during
                           a rotation. Bring your own key or use ` + "`keygen`" + `.
                           REQUIRED (min 24 chars) when the relay is network-reachable.
  RELAYENT_TRUST_PROXY     Set to 1 ONLY behind a reverse proxy you control; makes the
                           relay honour X-Forwarded-For / X-Forwarded-Proto.
  RELAYENT_ALLOW_INSECURE  Set to 1 to bypass the pairing-key requirement on a
                           network-reachable relay. Only when auth is enforced upstream.

SECURITY
  The pairing key is the ONLY thing standing between the internet and the CLI
  subscription of every bridge paired with it. Treat it like a password: never
  commit it, never log it, and serve the relay over TLS in production.
`

// knownBackends is the closed set of backend names the relay will store. The
// relay knows every valid name, so there is no reason to accept anything else.
var knownBackends = map[string]bool{
	"claude": true, "codex": true, "gemini": true, "cursor": true,
}

// knownBackendList is the same set in a stable display order, for the admin
// backend-policy UI/API (a map has no order).
var knownBackendList = []string{"claude", "codex", "gemini", "cursor"}

// supportedBackends are those with a real (non-stub) adapter. A stub (gemini) can
// never run a job regardless of policy or whether its CLI is installed. This
// mirrors the bridge's adapter set so the admin UI can say "not supported" even
// when no bridge is online to report it.
var supportedBackends = map[string]bool{
	"claude": true, "codex": true, "cursor": true, // gemini is a stub
}

// sanitizeCapabilities filters a bridge's self-report down to values the relay is
// willing to serve back. Capabilities are attacker-controllable — anyone with a
// valid pairing key can POST them — and they are later rendered on the status
// page, so unknown or oversized values are dropped rather than stored.
//
// This is defence in depth: the status page also builds its DOM with textContent
// so injected markup cannot execute. Neither control should be removed on the
// assumption that the other one holds.
func sanitizeCapabilities(caps api.BridgeCapabilities) api.BridgeCapabilities {
	out := api.BridgeCapabilities{
		Version:  clampString(caps.Version, 64),
		Hostname: clampString(caps.Hostname, 253), // max DNS name length
	}
	seen := map[string]bool{}
	for _, b := range caps.Backends {
		name := strings.ToLower(strings.TrimSpace(b.Name))
		if !knownBackends[name] || seen[name] {
			continue // unknown backend, or a duplicate padding the list
		}
		seen[name] = true
		out.Backends = append(out.Backends, api.BackendInfo{
			Name:      name,
			Installed: b.Installed,
			Supported: b.Supported,
			Ready:     b.Ready,
			Model:     clampString(b.Model, 64),
			// Model names come from the same untrusted report as everything else
			// here and are rendered on the status page, so they get the same
			// treatment: bounded count, bounded length, control chars stripped.
			// This allowlist is what stopped the stored XSS — any field added to
			// BackendInfo must be added HERE too, or it silently never reaches
			// consumers (which is exactly what happened when models were added).
			Models:       clampStrings(b.Models, 64, 64),
			DefaultModel: clampString(b.DefaultModel, 64),
			ModelsProbed: b.ModelsProbed,
		})
	}
	return out
}

// clampStrings applies clampString to a list and caps how many entries survive,
// so a hostile report cannot flood the status page with thousands of entries.
// Empty results are dropped rather than kept as blanks.
func clampStrings(in []string, maxLen, maxCount int) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(in), maxCount))
	for _, v := range in {
		if c := clampString(v, maxLen); c != "" {
			out = append(out, c)
			if len(out) >= maxCount {
				break
			}
		}
	}
	return out
}

// clampString trims a value and caps its length, dropping control characters
// that have no place in a display string.
func clampString(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	if len(s) > max {
		return s[:max]
	}
	return s
}

// --- rate limiting ---

// limiter is a token-bucket rate limiter keyed by an arbitrary string (pairing
// key fingerprint or client IP). It bounds both credential-guessing attempts and
// job floods that would otherwise burn a user's subscription quota.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens added per second
	burst   float64 // maximum tokens
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(rate, burst float64) *limiter {
	l := &limiter{buckets: map[string]*bucket{}, rate: rate, burst: burst}
	go l.janitor()
	return l
}

// allow consumes a token for id, reporting whether the request may proceed.
func (l *limiter) allow(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[id]
	if !ok {
		l.buckets[id] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	// Refill for elapsed time, then spend one token.
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// janitor drops idle buckets so the map cannot grow without bound (itself a
// memory-exhaustion vector, since bucket ids come from untrusted input).
func (l *limiter) janitor() {
	for range time.Tick(5 * time.Minute) {
		l.mu.Lock()
		for id, b := range l.buckets {
			if time.Since(b.last) > 15*time.Minute {
				delete(l.buckets, id)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP extracts the caller's IP, honouring X-Forwarded-For only when the
// relay is explicitly told it sits behind a trusted proxy. Trusting that header
// unconditionally would let any caller spoof their identity and bypass limits.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// securityHeaders sets conservative response headers. The CSP forbids external
// loads outright — matching the "no external CDNs" rule.
//
// script-src is 'none' here: API responses must never execute anything. The
// status page overrides this header with a per-request nonce for its own script
// (see statusPage). Note that 'unsafe-inline' is deliberately NOT used for
// scripts — it would also authorise inline event handlers such as onerror=,
// which is exactly how injected markup gains execution.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
