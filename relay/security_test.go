// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Tests for the relay's security policy — the startup key policy
//
//	(the control that stops a public relay from being an open proxy to someone's
//	CLI subscription), key generation, constant-time comparison, and limiters.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"testing"
	"time"
)

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8787": true,
		"localhost:8787": true,
		"[::1]:8787":     true,
		":8787":          false, // all interfaces — reachable
		"0.0.0.0:8787":   false,
		"[::]:8787":      false,
		"192.168.1.5:87": false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

// The core security property: a network-reachable relay must not start without a
// strong fixed key, because that config lets anyone spend the user's subscription.
func TestValidateKeyPolicy(t *testing.T) {
	strong, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	cases := []struct {
		name    string
		key     string
		addr    string
		allow   bool
		wantErr bool
	}{
		{"loopback without key is fine", "", "127.0.0.1:8787", false, false},
		{"loopback with weak key is fine", "dev", "localhost:8787", false, false},
		{"public without key is refused", "", ":8787", false, true},
		{"public with weak key is refused", "devkey", ":8787", false, true},
		{"public with strong key is fine", strong, ":8787", false, false},
		{"public without key but explicit opt-out", "", ":8787", true, false},
		{"public bind addr without key is refused", "", "0.0.0.0:8787", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateKeyPolicy(c.key, c.addr, c.allow)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateKeyPolicy(%q, %q, %v) error = %v, wantErr %v",
					c.key, c.addr, c.allow, err, c.wantErr)
			}
		})
	}
}

func TestGenerateKeyIsStrongAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		k, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey: %v", err)
		}
		if len(k) < minKeyLen {
			t.Fatalf("generated key too short: %d chars", len(k))
		}
		if seen[k] {
			t.Fatalf("GenerateKey returned a duplicate: %q", k)
		}
		seen[k] = true
	}
}

func TestCheckKey(t *testing.T) {
	if !checkKey("abc123", "abc123") {
		t.Error("identical keys should match")
	}
	if checkKey("abc123", "abc124") {
		t.Error("different keys must not match")
	}
	if checkKey("", "abc123") {
		t.Error("empty key must not match")
	}
	if checkKey("abc", "abc123") {
		t.Error("prefix must not match (length differs)")
	}
}

func TestKeyFingerprintHidesKey(t *testing.T) {
	key := "super-secret-pairing-key"
	fp := keyFingerprint(key)
	if fp == "" || len(fp) != 8 {
		t.Fatalf("fingerprint should be 8 chars, got %q", fp)
	}
	if fp == key[:8] {
		t.Error("fingerprint must not reveal the key's prefix")
	}
	if keyFingerprint(key) != fp {
		t.Error("fingerprint must be stable")
	}
	if keyFingerprint("another-key") == fp {
		t.Error("different keys should produce different fingerprints")
	}
}

func TestParseKeySet(t *testing.T) {
	ks := ParseKeySet("primary123,old456, old789 ")
	if ks.primary != "primary123" {
		t.Errorf("primary = %q, want primary123", ks.primary)
	}
	if len(ks.retiring) != 2 {
		t.Fatalf("retiring = %v, want 2 entries", ks.retiring)
	}
	// Whitespace around hand-edited env entries must not silently break auth.
	if ks.retiring[1] != "old789" {
		t.Errorf("retiring[1] = %q, want old789 (trimmed)", ks.retiring[1])
	}
	if ParseKeySet("").Empty() != true {
		t.Error("empty string should yield an empty KeySet")
	}
	if ParseKeySet("  ,  ").Empty() != true {
		t.Error("only-separators should yield an empty KeySet")
	}
}

// Rotation's whole point: both old and new keys work during the overlap window.
func TestKeySetAcceptsPrimaryAndRetiring(t *testing.T) {
	ks := ParseKeySet("newkey,oldkey")
	if !ks.Accepts("newkey") {
		t.Error("primary key must be accepted")
	}
	if !ks.Accepts("oldkey") {
		t.Error("retiring key must still be accepted during rotation")
	}
	if ks.Accepts("otherkey") {
		t.Error("unknown key must be rejected")
	}
	if ks.Accepts("") {
		t.Error("empty key must be rejected")
	}
}

func TestKeySetIsRetiring(t *testing.T) {
	ks := ParseKeySet("newkey,oldkey")
	if ks.IsRetiring("newkey") {
		t.Error("primary key is not retiring")
	}
	if !ks.IsRetiring("oldkey") {
		t.Error("old key should report as retiring so operators can track migration")
	}
	if ks.IsRetiring("unknown") {
		t.Error("unknown key is not retiring")
	}
	// A single-key set has nothing retiring.
	if ParseKeySet("only").IsRetiring("only") {
		t.Error("sole key must not report as retiring")
	}
}

// A rotation must not be a way to sneak a weak key onto a public relay.
func TestValidateKeySetPolicyChecksEveryKey(t *testing.T) {
	strong, _ := GenerateKey()
	strong2, _ := GenerateKey()

	if err := validateKeySetPolicy(ParseKeySet(strong+","+strong2), ":8787", false); err != nil {
		t.Errorf("two strong keys should be accepted on a public relay: %v", err)
	}
	if err := validateKeySetPolicy(ParseKeySet(strong+",weak"), ":8787", false); err == nil {
		t.Error("a weak RETIRING key must be refused on a public relay")
	}
	if err := validateKeySetPolicy(ParseKeySet("weak,"+strong), ":8787", false); err == nil {
		t.Error("a weak PRIMARY key must be refused on a public relay")
	}
	if err := validateKeySetPolicy(ParseKeySet("weak,also-weak"), "127.0.0.1:8787", false); err != nil {
		t.Errorf("loopback relay should tolerate weak keys: %v", err)
	}
}

func TestLimiterBlocksBurstThenRefills(t *testing.T) {
	// 10 tokens/sec, burst of 3: three immediate calls pass, the fourth fails.
	l := &limiter{buckets: map[string]*bucket{}, rate: 10, burst: 3}
	for i := 0; i < 3; i++ {
		if !l.allow("id") {
			t.Fatalf("call %d should be allowed within burst", i+1)
		}
	}
	if l.allow("id") {
		t.Fatal("call past the burst must be blocked")
	}
	// After ~100ms one token (10/sec) is back.
	time.Sleep(150 * time.Millisecond)
	if !l.allow("id") {
		t.Fatal("bucket should refill over time")
	}
}

func TestLimiterIsolatesIdentities(t *testing.T) {
	l := &limiter{buckets: map[string]*bucket{}, rate: 1, burst: 1}
	if !l.allow("a") {
		t.Fatal("first call for a should pass")
	}
	if l.allow("a") {
		t.Fatal("second call for a should be blocked")
	}
	// One identity exhausting its bucket must not affect another.
	if !l.allow("b") {
		t.Fatal("identity b must have its own bucket")
	}
}
