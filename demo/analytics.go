// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Fire-and-forget visitor analytics for the demo. On each page view
//
//	the demo derives a few coarse, non-identifying signals — a device/browser/OS
//	family from the User-Agent, the referrer HOST (never the full URL), any UTM
//	campaign params, and the visitor IP — and POSTs them to the relay's
//	/v1/demo/hit. The relay turns the IP into a country + a daily hash and
//	discards it (see relay/demoapi.go); the demo never persists anything itself.
//
//	This is best-effort and MUST NOT affect the visitor: the report runs in a
//	background goroutine with its own timeout, obvious bots are skipped, and any
//	error is silently dropped. The chat page renders and works exactly the same
//	whether or not analytics succeeds.
//
//	Deliberately stdlib-only (no UA-parsing dependency): a coarse family match is
//	all the admin view needs, and it keeps the demo a single dependency-free
//	binary like the rest of Relayent.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// hitPayload is what the demo posts to the relay per page view. The IP is sent so
// the relay can do offline GeoIP + hashing; the demo keeps none of this.
type hitPayload struct {
	IP          string `json:"ip"`
	Referrer    string `json:"referrer"`
	UTMSource   string `json:"utm_source"`
	UTMMedium   string `json:"utm_medium"`
	UTMCampaign string `json:"utm_campaign"`
	Device      string `json:"device"`
	Browser     string `json:"browser"`
	OS          string `json:"os"`
}

// reportVisit fires a best-effort analytics hit for a page view in the background.
// It returns immediately; the visitor's request is never blocked or failed by it.
// Obvious bots/crawlers are dropped so the counts reflect real people.
func (s *server) reportVisit(r *http.Request) {
	ua := r.UserAgent()
	device, browser, os := parseUA(ua)
	if device == "bot" {
		return // don't count crawlers
	}
	hit := hitPayload{
		IP:          demoClientIP(r, s.cfg.trustProxy),
		Referrer:    referrerHost(r.Header.Get("Referer")),
		UTMSource:   firstQuery(r, "utm_source"),
		UTMMedium:   firstQuery(r, "utm_medium"),
		UTMCampaign: firstQuery(r, "utm_campaign"),
		Device:      device,
		Browser:     browser,
		OS:          os,
	}
	go s.sendHit(hit)
}

// sendHit POSTs one hit to the relay with its own short timeout, independent of
// the visitor's request context (which is already done). All errors are ignored:
// analytics is never allowed to surface.
func (s *server) sendHit(hit hitPayload) {
	body, err := json.Marshal(hit)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := s.relayReq(ctx, "POST", "/v1/demo/hit", body)
	if err != nil {
		return
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// demoClientIP extracts the visitor IP, honouring X-Forwarded-For only when the
// demo is explicitly told it sits behind a trusted proxy (DEMO_TRUST_PROXY=1) —
// otherwise any caller could spoof it. Mirrors the relay's clientIP.
func demoClientIP(r *http.Request, trustProxy bool) string {
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

// referrerHost reduces a Referer header to its host, dropping the path/query so no
// URL is ever forwarded. Same-origin and empty referrers yield "" (counted as
// direct). Returns "" on anything unparseable.
func referrerHost(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func firstQuery(r *http.Request, key string) string {
	return strings.TrimSpace(r.URL.Query().Get(key))
}

// parseUA classifies a User-Agent into coarse (device, browser, os) families. It
// is intentionally simple substring matching — enough for a traffic breakdown,
// not a fingerprint. Order matters: more specific tokens are checked first (e.g.
// Edg before Chrome, since Edge's UA also contains "Chrome").
func parseUA(ua string) (device, browser, os string) {
	u := strings.ToLower(ua)
	if u == "" {
		return "", "", ""
	}

	// Bots first: if it looks like a crawler, that's all the caller needs.
	for _, sig := range []string{"bot", "crawler", "spider", "slurp", "bingpreview", "facebookexternalhit", "curl", "wget", "python-requests", "go-http-client", "headlesschrome"} {
		if strings.Contains(u, sig) {
			return "bot", "", ""
		}
	}

	// OS family.
	switch {
	case strings.Contains(u, "iphone"), strings.Contains(u, "ipod"):
		os = "ios"
	case strings.Contains(u, "ipad"):
		os = "ios"
	case strings.Contains(u, "android"):
		os = "android"
	case strings.Contains(u, "windows"):
		os = "windows"
	case strings.Contains(u, "mac os x"), strings.Contains(u, "macintosh"):
		os = "macos"
	case strings.Contains(u, "cros"):
		os = "chromeos"
	case strings.Contains(u, "linux"):
		os = "linux"
	}

	// Device family. Tablets are called out before phones; anything else desktop.
	switch {
	case strings.Contains(u, "ipad"), strings.Contains(u, "tablet"):
		device = "tablet"
	case strings.Contains(u, "mobi"), strings.Contains(u, "iphone"), strings.Contains(u, "android"):
		device = "mobile"
	default:
		device = "desktop"
	}

	// Browser family (specific-before-generic).
	switch {
	case strings.Contains(u, "edg"):
		browser = "edge"
	case strings.Contains(u, "opr"), strings.Contains(u, "opera"):
		browser = "opera"
	case strings.Contains(u, "samsungbrowser"):
		browser = "samsung"
	case strings.Contains(u, "firefox"), strings.Contains(u, "fxios"):
		browser = "firefox"
	case strings.Contains(u, "chrome"), strings.Contains(u, "crios"):
		browser = "chrome"
	case strings.Contains(u, "safari"):
		browser = "safari"
	}
	return device, browser, os
}
