// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: Tests for the demo's stdlib User-Agent / referrer parsing. The
//
//	parser only needs coarse families (not a fingerprint); these lock in the
//	specific-before-generic ordering (Edge vs Chrome, tablet vs mobile) and the
//	bot filter, plus that the referrer is reduced to a host and never a URL.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import "testing"

func TestParseUA(t *testing.T) {
	cases := []struct {
		name                   string
		ua                     string
		wDevice, wBrowser, wOS string
	}{
		{"chrome-win", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36", "desktop", "chrome", "windows"},
		{"edge-win", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36 Edg/120.0", "desktop", "edge", "windows"},
		{"safari-mac", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15", "desktop", "safari", "macos"},
		{"safari-iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1", "mobile", "safari", "ios"},
		{"chrome-android", "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 Chrome/120.0 Mobile Safari/537.36", "mobile", "chrome", "android"},
		{"ipad", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Safari/604.1", "tablet", "safari", "ios"},
		{"firefox-linux", "Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0", "desktop", "firefox", "linux"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, b, o := parseUA(c.ua)
			if d != c.wDevice || b != c.wBrowser || o != c.wOS {
				t.Errorf("parseUA(%s) = (%q,%q,%q), want (%q,%q,%q)", c.name, d, b, o, c.wDevice, c.wBrowser, c.wOS)
			}
		})
	}
}

func TestParseUABots(t *testing.T) {
	bots := []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"curl/8.4.0",
		"python-requests/2.31.0",
		"Go-http-client/1.1",
	}
	for _, ua := range bots {
		if d, _, _ := parseUA(ua); d != "bot" {
			t.Errorf("parseUA(%q) device = %q, want bot", ua, d)
		}
	}
}

func TestReferrerHost(t *testing.T) {
	cases := map[string]string{
		"https://news.ycombinator.com/item?id=123": "news.ycombinator.com",
		"http://Example.COM/some/path":             "example.com",
		"":                                         "",
		"not a url":                                "",
		"android-app://com.google.android.gm":      "com.google.android.gm",
	}
	for in, want := range cases {
		if got := referrerHost(in); got != want {
			t.Errorf("referrerHost(%q) = %q, want %q", in, got, want)
		}
	}
}
