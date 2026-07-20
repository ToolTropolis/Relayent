// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Tests for the demo analytics API. The load-bearing ones prove the
//
//	privacy contract end-to-end: the ingest handler never lets a raw IP reach the
//	store (only a hash + country), the hash rotates daily, and the demo-stats
//	scope is exactly and only what the ingest route requires.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Ingest must derive a hash + country and DISCARD the raw IP: the stored bytes
// must not contain the IP, and the stats must count the visit.
func TestDemoIngestDiscardsRawIP(t *testing.T) {
	s := adminTestServer(t) // geoip nil -> country "??"
	app := &Principal{Kind: KindApp, Scopes: []string{ScopeDemoStats}}

	rec := httptest.NewRecorder()
	body := `{"ip":"203.0.113.45","referrer":"news.example","device":"desktop","browser":"chrome","os":"macos"}`
	s.demoIngest(rec, adminReq("POST", body), app)
	if rec.Code != 204 {
		t.Fatalf("ingest status = %d, want 204: %s", rec.Code, rec.Body)
	}

	// The visit is counted...
	res, err := s.store.DemoStats(1)
	if err != nil {
		t.Fatalf("DemoStats: %v", err)
	}
	if res.TotalHits != 1 {
		t.Fatalf("TotalHits = %d, want 1", res.TotalHits)
	}
	// ...with a unique (hash present) and country "??" (no GeoIP DB in test).
	if res.Uniques != 1 {
		t.Errorf("Uniques = %d, want 1", res.Uniques)
	}
	if len(res.Countries) != 1 || res.Countries[0].Label != "??" {
		t.Errorf("country = %+v, want ?? (no GeoIP DB)", res.Countries)
	}

	// The raw IP must appear NOWHERE in what was stored.
	assertRawIPAbsent(t, s, "203.0.113.45")
}

// assertRawIPAbsent walks every stored demo hit and fails if the raw IP is in it.
func assertRawIPAbsent(t *testing.T, s *server, ip string) {
	t.Helper()
	err := s.store.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bktDemoHits).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if strings.Contains(string(v), ip) {
				t.Fatalf("raw IP %q found at rest in a demo hit: %s", ip, v)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan demo hits: %v", err)
	}
}

// The daily-salted hash must differ across days for the same IP (no cross-day
// tracking) and be non-empty for a real IP / empty for none.
func TestDemoIPHashDailyRotation(t *testing.T) {
	d1 := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	ip := "198.51.100.7"

	h1 := demoIPHash(ip, d1)
	h1b := demoIPHash(ip, d1)
	h2 := demoIPHash(ip, d2)

	if h1 == "" {
		t.Fatal("hash of a real IP should be non-empty")
	}
	if h1 != h1b {
		t.Fatal("hash must be stable within a day")
	}
	if h1 == h2 {
		t.Fatal("hash must rotate across days (no cross-day visitor tracking)")
	}
	if h1 == ip || strings.Contains(h1, ip) {
		t.Fatal("hash must not reveal the IP")
	}
	if demoIPHash("", d1) != "" {
		t.Fatal("empty IP should yield an empty hash (no unique contribution)")
	}
}

// The demo-stats scope must gate ingest: a principal without it cannot write, and
// admin scope alone does NOT grant it (least privilege).
func TestDemoStatsScopeIsDistinct(t *testing.T) {
	demoApp := &Principal{Scopes: []string{ScopeDemoStats}}
	if !demoApp.Can(ScopeDemoStats) {
		t.Fatal("demo app should hold demo-stats scope")
	}
	if demoApp.Can(ScopeEnqueue) || demoApp.Can(ScopeAdmin) {
		t.Fatal("demo-stats scope must not imply enqueue or admin")
	}
	admin := &Principal{Scopes: []string{ScopeAdmin}}
	if admin.Can(ScopeDemoStats) {
		t.Fatal("admin scope must not imply demo-stats (ingest is a separate credential)")
	}
}

// Oversized demo-supplied labels must be clamped, never stored raw.
func TestClampField(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := clampField("  " + strings.ToUpper(long) + "  ")
	if len(got) != demoFieldMax {
		t.Fatalf("clampField length = %d, want %d", len(got), demoFieldMax)
	}
	if got != strings.ToLower(got) {
		t.Fatal("clampField must lower-case")
	}
}
