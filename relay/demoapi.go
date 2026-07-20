// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: The demo visitor-analytics API. Two endpoints:
//
//	POST /v1/demo/hit        — ingest one page view. Authed by an app credential
//	                           holding ONLY the demo-stats scope, so the demo can
//	                           write hits and do nothing else. The demo sends the
//	                           visitor IP (it is the only party that sees it); the
//	                           relay derives country + a daily-salted hash and then
//	                           DISCARDS the raw IP — it is never stored.
//	GET  /v1/admin/demo-stats — the aggregated rollup, admin-only, no raw rows.
//
//	Every string the demo sends is clamped and lower-cased here (defence in depth:
//	the store must never hold an oversized or attacker-shaped label even though
//	the demo already normalises).
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// demoHitIn is the payload the demo posts per page view. It carries coarse buckets
// the demo already parsed, plus the raw visitor IP — which the relay uses to
// derive country and a hash, then throws away (never stored, never logged).
type demoHitIn struct {
	IP          string `json:"ip"`           // visitor IP; used for GeoIP + hash, then discarded
	Referrer    string `json:"referrer"`     // referring host only
	UTMSource   string `json:"utm_source"`   //
	UTMMedium   string `json:"utm_medium"`   //
	UTMCampaign string `json:"utm_campaign"` //
	Device      string `json:"device"`       // desktop | mobile | tablet | bot
	Browser     string `json:"browser"`      //
	OS          string `json:"os"`           //
}

// demoFieldMax bounds any single label the demo sends, so a hostile or buggy demo
// can never write an oversized value into the store.
const demoFieldMax = 64

// demoIngest records one visitor hit. Best-effort by design: it always returns
// 204 once authenticated and shaped, even if the store is legacy/nil, so the demo
// treats analytics as fire-and-forget and never surfaces an analytics error to a
// visitor.
func (s *server) demoIngest(w http.ResponseWriter, r *http.Request, p *Principal) {
	var in demoHitIn
	if !decode(w, r, &in) {
		return
	}

	now := time.Now().UTC()
	hit := DemoHit{
		TS:          now,
		Country:     s.demoCountry(in.IP),
		Referrer:    clampField(in.Referrer),
		UTMSource:   clampField(in.UTMSource),
		UTMMedium:   clampField(in.UTMMedium),
		UTMCampaign: clampField(in.UTMCampaign),
		Device:      clampField(in.Device),
		Browser:     clampField(in.Browser),
		OS:          clampField(in.OS),
		IPHash:      demoIPHash(in.IP, now),
	}
	// Store errors are swallowed on purpose — a failed analytics write must never
	// become a visitor-facing failure. The response is the same either way.
	_ = s.store.AppendDemoHit(hit)
	w.WriteHeader(http.StatusNoContent)
}

// demoCountry resolves a country code from the visitor IP, or "??" when GeoIP is
// unavailable or the lookup misses. The raw IP goes no further than this call.
func (s *server) demoCountry(ip string) string {
	if c := s.geoip.Country(ip); c != "" {
		return strings.ToUpper(c)
	}
	return "??"
}

// demoIPHash produces a coarse, non-reversible visitor identifier that rotates
// daily. The salt is the UTC date, so the same IP hashes differently on different
// days: uniques are countable within a day but a hash can't track a visitor
// across days, and it can't be reversed to an IP. An empty IP yields "" (no
// unique contribution) rather than a hash of the salt alone.
func demoIPHash(ip string, now time.Time) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(now.Format("2006-01-02") + "|" + ip))
	return hex.EncodeToString(sum[:16]) // 128-bit prefix is ample for counting uniques
}

// clampField lower-cases, trims and length-limits a demo-supplied label.
func clampField(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) > demoFieldMax {
		v = v[:demoFieldMax]
	}
	return v
}

// adminDemoStats returns the aggregated visitor rollup. Admin-only (routed via
// authorize(ScopeAdmin)). ?days=<n> selects the window (default 30, capped 365).
func (s *server) adminDemoStats(w http.ResponseWriter, r *http.Request, p *Principal) {
	days := 0
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	stats, err := s.store.DemoStats(days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read demo stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
