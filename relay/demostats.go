// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Admin-only visitor analytics for the public demo. The demo server
//
//	reports one content-free hit per page view; this file stores those hits and
//	serves an aggregated rollup to the admin. It follows the audit log's
//	discipline exactly: what is at rest is IDs, coarse buckets and counts —
//	NEVER a raw IP, a full URL, or anything that identifies a person. Uniques are
//	a daily-rotating salted hash, so a hash cannot be reversed to an IP and does
//	not correlate a visitor across days. The bucket is append-only and capped;
//	old hits are trimmed so it can never grow without bound.
//
//	"Demographics" here means privacy-preserving buckets only — country (from an
//	offline GeoIP DB, "??" when none is configured), traffic source, and coarse
//	device/browser/OS family. No third-party tracker is ever contacted; the demo
//	stays a credential-holding proxy that makes no external calls.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// bktDemoHits holds one DemoHit per key (seq uint64 -> DemoHit JSON), append-only
// and capped like the audit log. Registered in allBuckets (store.go).
var bktDemoHits = []byte("demo_hits")

// demoHitsCap bounds the raw hit log. Aggregation reads the whole bucket, so this
// also bounds a stats query's cost. Well above a small demo's traffic; trimmed on
// append once exceeded.
const demoHitsCap = 50000

// DemoHit is one page view, reduced to non-identifying buckets. There is
// deliberately no raw IP and no full URL: IPHash is a daily-salted digest (coarse
// uniques only) and Referrer is a host, not a path. This mirrors AuditEvent's
// "IDs and counts, never content" rule.
type DemoHit struct {
	TS          time.Time `json:"ts"`
	Country     string    `json:"country"`      // ISO-3166 alpha-2, or "??" when GeoIP is unavailable
	Referrer    string    `json:"referrer"`     // referring HOST only (no path/query), "" if none/direct
	UTMSource   string    `json:"utm_source"`   // campaign attribution, if present
	UTMMedium   string    `json:"utm_medium"`   //
	UTMCampaign string    `json:"utm_campaign"` //
	Device      string    `json:"device"`       // desktop | mobile | tablet | bot | ""
	Browser     string    `json:"browser"`      // coarse family: chrome | safari | firefox | edge | ...
	OS          string    `json:"os"`           // coarse family: windows | macos | ios | android | linux | ...
	IPHash      string    `json:"ip_hash"`      // sha256(ip + daily salt); NEVER the raw IP
}

// AppendDemoHit records one visit. Best-effort: a nil/legacy store is a silent
// no-op so the demo never breaks when analytics has nowhere to land. Trims the
// oldest entries once the cap is exceeded.
func (s *Store) AppendDemoHit(h DemoHit) error {
	if !s.Enabled() {
		return nil
	}
	if h.TS.IsZero() {
		h.TS = time.Now()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bktDemoHits)
		seq, _ := bkt.NextSequence()
		if err := bkt.Put(itob(seq), mustJSON(h)); err != nil {
			return err
		}
		// Trim from the front (oldest keys sort first) until back under the cap.
		for bkt.Stats().KeyN > demoHitsCap {
			c := bkt.Cursor()
			k, _ := c.First()
			if k == nil {
				break
			}
			if err := bkt.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- aggregation ---

// DemoBucket is one label with its visit count, for a top-N breakdown.
type DemoBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// DemoDayCount is one day of the visits time series (UTC date, YYYY-MM-DD).
type DemoDayCount struct {
	Date   string `json:"date"`
	Visits int    `json:"visits"`
}

// DemoStatsResult is the aggregated rollup the admin view renders. It contains no
// per-visitor rows — only totals, a daily series, and top-N buckets.
type DemoStatsResult struct {
	Days      int            `json:"days"`       // window requested
	TotalHits int            `json:"total_hits"` // visits in the window
	Uniques   int            `json:"uniques"`    // distinct daily IP-hashes in the window
	Today     int            `json:"today"`      // visits so far today (UTC)
	Series    []DemoDayCount `json:"series"`     // one entry per day in the window, oldest first
	Countries []DemoBucket   `json:"countries"`  // top-N by visits
	Referrers []DemoBucket   `json:"referrers"`  // top-N by visits (direct excluded)
	Devices   []DemoBucket   `json:"devices"`    //
	Browsers  []DemoBucket   `json:"browsers"`   //
	OSes      []DemoBucket   `json:"oses"`       //
	Sources   []DemoBucket   `json:"sources"`    // top UTM sources
	OldestTS  string         `json:"oldest_ts"`  // RFC3339 of the earliest counted hit, "" if none
}

const demoTopN = 10

// DemoStats aggregates the last `days` of hits into a rollup. days<=0 defaults to
// 30 and is capped at 365. All bucketing is done here so the API never hands the
// admin raw rows. A nil/legacy store yields an empty (non-nil) result.
func (s *Store) DemoStats(days int) (DemoStatsResult, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	res := DemoStatsResult{Days: days}
	if !s.Enabled() {
		return res, nil
	}

	now := time.Now().UTC()
	todayKey := now.Format("2006-01-02")
	// The window is the last `days` calendar days INCLUDING today: cutoff is the
	// start of the earliest day, so a hit anywhere on that day still counts.
	firstDay := now.AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)
	cutoff := firstDay

	// Pre-seed the series with zero-filled days (firstDay .. today) so the chart
	// has no gaps.
	dayIdx := map[string]int{}
	for d := 0; d < days; d++ {
		date := firstDay.AddDate(0, 0, d).Format("2006-01-02")
		dayIdx[date] = len(res.Series)
		res.Series = append(res.Series, DemoDayCount{Date: date})
	}

	country := map[string]int{}
	referrer := map[string]int{}
	device := map[string]int{}
	browser := map[string]int{}
	os := map[string]int{}
	source := map[string]int{}
	uniques := map[string]struct{}{} // key: date|ip_hash → distinct visitor-days
	var oldest time.Time

	err := s.db.View(func(tx *bolt.Tx) error {
		cur := tx.Bucket(bktDemoHits).Cursor()
		// Scan the whole (capped) bucket and filter by the window. We don't rely on
		// key order matching time order — a clock skew or out-of-order append must
		// not truncate the scan and silently drop hits.
		for k, v := cur.First(); k != nil; k, v = cur.Next() {
			var h DemoHit
			if err := json.Unmarshal(v, &h); err != nil {
				return err
			}
			ts := h.TS.UTC()
			if ts.Before(cutoff) {
				continue
			}
			res.TotalHits++
			if oldest.IsZero() || ts.Before(oldest) {
				oldest = ts
			}
			date := ts.Format("2006-01-02")
			if i, ok := dayIdx[date]; ok {
				res.Series[i].Visits++
			}
			if date == todayKey {
				res.Today++
			}
			if h.IPHash != "" {
				uniques[date+"|"+h.IPHash] = struct{}{}
			}
			bump(country, orQ(h.Country))
			if h.Referrer != "" {
				bump(referrer, h.Referrer)
			}
			bump(device, h.Device)
			bump(browser, h.Browser)
			bump(os, h.OS)
			if h.UTMSource != "" {
				bump(source, h.UTMSource)
			}
		}
		return nil
	})
	if err != nil {
		return res, err
	}

	res.Uniques = len(uniques)
	res.Countries = topN(country, demoTopN)
	res.Referrers = topN(referrer, demoTopN)
	res.Devices = topN(device, demoTopN)
	res.Browsers = topN(browser, demoTopN)
	res.OSes = topN(os, demoTopN)
	res.Sources = topN(source, demoTopN)
	if !oldest.IsZero() {
		res.OldestTS = oldest.Format(time.RFC3339)
	}
	return res, nil
}

func bump(m map[string]int, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	m[key]++
}

func orQ(country string) string {
	if strings.TrimSpace(country) == "" {
		return "??"
	}
	return country
}

// topN returns the n highest-count labels, ties broken by label for determinism.
func topN(m map[string]int, n int) []DemoBucket {
	out := make([]DemoBucket, 0, len(m))
	for label, count := range m {
		out = append(out, DemoBucket{Label: label, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
