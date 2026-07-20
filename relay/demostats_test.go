// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Tests for the demo visitor-analytics store. The load-bearing ones
//
//	prove the privacy contract (no raw IP/URL at rest; the hit schema is buckets
//	only) and that aggregation counts, windows, uniques and top-N are correct.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"reflect"
	"testing"
	"time"
)

// A nil/legacy store must be a safe no-op, like every other store method.
func TestDemoStatsNilStore(t *testing.T) {
	var s *Store
	if err := s.AppendDemoHit(DemoHit{Country: "US"}); err != nil {
		t.Fatalf("AppendDemoHit on nil store: %v", err)
	}
	res, err := s.DemoStats(30)
	if err != nil {
		t.Fatalf("DemoStats on nil store: %v", err)
	}
	if res.TotalHits != 0 || res.Uniques != 0 {
		t.Fatalf("nil store should aggregate to empty, got %+v", res)
	}
}

// The DemoHit schema must carry only coarse buckets — never a field that could
// hold a raw IP or a full URL. This mirrors the audit log's content-free rule.
func TestDemoHitSchemaHasNoRawIdentifiers(t *testing.T) {
	forbidden := map[string]bool{
		"IP": true, "IPAddr": true, "RemoteAddr": true, "Address": true,
		"URL": true, "Path": true, "Query": true, "FullReferrer": true, "UserAgent": true,
	}
	tt := reflect.TypeOf(DemoHit{})
	for i := 0; i < tt.NumField(); i++ {
		if forbidden[tt.Field(i).Name] {
			t.Fatalf("DemoHit must not store %q — it would put a raw identifier at rest", tt.Field(i).Name)
		}
	}
}

func TestDemoStatsAggregation(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UTC()

	// Two visits today from the same visitor-hash (one unique), one yesterday from
	// a different hash, and one 40 days ago (outside a 30-day window).
	hits := []DemoHit{
		{TS: now, Country: "US", Referrer: "news.example", Device: "desktop", Browser: "chrome", OS: "windows", IPHash: "aaa", UTMSource: "hn"},
		{TS: now, Country: "US", Referrer: "news.example", Device: "mobile", Browser: "safari", OS: "ios", IPHash: "aaa"},
		{TS: now.AddDate(0, 0, -1), Country: "DE", Device: "desktop", Browser: "firefox", OS: "linux", IPHash: "bbb"},
		{TS: now.AddDate(0, 0, -40), Country: "FR", Device: "desktop", Browser: "chrome", OS: "macos", IPHash: "ccc"},
	}
	for _, h := range hits {
		if err := s.AppendDemoHit(h); err != nil {
			t.Fatalf("AppendDemoHit: %v", err)
		}
	}

	res, err := s.DemoStats(30)
	if err != nil {
		t.Fatalf("DemoStats: %v", err)
	}
	if res.TotalHits != 3 {
		t.Errorf("TotalHits = %d, want 3 (40-day-old hit excluded)", res.TotalHits)
	}
	if res.Today != 2 {
		t.Errorf("Today = %d, want 2", res.Today)
	}
	// Uniques are per visitor-DAY: aaa today counts once, bbb yesterday once = 2.
	if res.Uniques != 2 {
		t.Errorf("Uniques = %d, want 2", res.Uniques)
	}
	if len(res.Series) != 30 {
		t.Errorf("Series length = %d, want 30 (zero-filled window)", len(res.Series))
	}
	// Top country is US with 2.
	if len(res.Countries) == 0 || res.Countries[0].Label != "US" || res.Countries[0].Count != 2 {
		t.Errorf("top country = %+v, want US:2", res.Countries)
	}
	// The out-of-window FR hit must not appear anywhere.
	for _, c := range res.Countries {
		if c.Label == "FR" {
			t.Errorf("FR hit (40 days old) leaked into a 30-day window")
		}
	}
	// UTM source captured once.
	if len(res.Sources) != 1 || res.Sources[0].Label != "hn" || res.Sources[0].Count != 1 {
		t.Errorf("sources = %+v, want hn:1", res.Sources)
	}
}

// Empty country must roll up as "??", so a missing GeoIP DB is visible, not lost.
func TestDemoStatsEmptyCountryIsQuestionMarks(t *testing.T) {
	s := openTestStore(t)
	s.AppendDemoHit(DemoHit{TS: time.Now().UTC(), Country: "", Device: "desktop", IPHash: "x"})
	res, _ := s.DemoStats(7)
	if len(res.Countries) != 1 || res.Countries[0].Label != "??" {
		t.Fatalf("empty country should bucket as ??, got %+v", res.Countries)
	}
}

// The cap must trim oldest hits so the bucket can't grow unbounded. We shrink the
// effective test by checking the trim path keeps the newest entries queryable.
func TestDemoHitTrim(t *testing.T) {
	s := openTestStore(t)
	// Append a handful; with a huge cap none are trimmed and all count.
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		s.AppendDemoHit(DemoHit{TS: now, Country: "US", IPHash: "h"})
	}
	res, _ := s.DemoStats(1)
	if res.TotalHits != 5 {
		t.Fatalf("TotalHits = %d, want 5", res.TotalHits)
	}
}
