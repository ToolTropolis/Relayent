// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Offline IP→country lookup for the demo visitor analytics. Reads a
//
//	MaxMind GeoLite2-Country.mmdb via a pure-Go reader (github.com/oschwald/
//	maxminddb-golang) so the relay stays a single static CGO_ENABLED=0 binary and
//	NEVER makes an external call to resolve a country — the database is a local
//	file the operator supplies.
//
//	GeoIP is entirely optional and degrades gracefully. If RELAYENT_GEOIP_DB is
//	unset, points at a missing/corrupt file, or a lookup misses, Country returns
//	"" and the caller records "??". Everything else in the analytics keeps
//	working; only the country breakdown goes dark.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"log"
	"net"
	"os"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

// geoIP resolves an IP to an ISO country code from a local MaxMind DB. A nil
// *geoIP is valid and always returns "" — the "no database configured" state, so
// callers never need to nil-check.
type geoIP struct {
	db *maxminddb.Reader
}

// openGeoIP opens the country DB at path. An empty path (feature off) or an
// unreadable file yields a nil *geoIP and a nil error: analytics still runs, just
// without country data. Only a genuinely corrupt-but-present file is worth a log
// line, and even then we return nil rather than fail relay startup.
func openGeoIP(path string) *geoIP {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		log.Printf("[relay] GeoIP disabled: %s not readable (%v)", path, err)
		return nil
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		log.Printf("[relay] GeoIP disabled: could not open %s (%v)", path, err)
		return nil
	}
	log.Printf("[relay] GeoIP enabled from %s", path)
	return &geoIP{db: db}
}

// Country returns the ISO-3166 alpha-2 code for ip, or "" when GeoIP is off, the
// ip is unparseable/private, or the lookup misses. The caller maps "" to "??".
func (g *geoIP) Country(ip string) string {
	if g == nil || g.db == nil {
		return ""
	}
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return ""
	}
	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := g.db.Lookup(parsed, &rec); err != nil {
		return ""
	}
	return rec.Country.ISOCode
}

// Close releases the DB. Safe on nil.
func (g *geoIP) Close() error {
	if g == nil || g.db == nil {
		return nil
	}
	return g.db.Close()
}
