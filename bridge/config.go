// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Bridge configuration, loaded from environment variables (a TOML
//
//	config file at ~/.relayent/config.toml can be layered on later).
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config controls the bridge daemon.
type Config struct {
	RelayURL   string        // base URL of the relay, e.g. https://relay.example.com
	PairingKey string        // bearer key identifying this user's job namespace
	PollWait   time.Duration // long-poll wait per /v1/jobs/next request
	JobTimeout time.Duration // max wall-clock per CLI invocation
}

// LoadConfig reads configuration from ~/.relayent/config.env (written by
// `relayent-bridge setup`) and then the environment, which takes precedence so a
// service manager or a one-off run can override the saved pairing.
func LoadConfig() (Config, error) {
	file, err := LoadFileConfig()
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}
	pick := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return file[key]
	}

	c := Config{
		RelayURL:   strings.TrimRight(pick("RELAYENT_RELAY_URL"), "/"),
		PairingKey: pick("RELAYENT_PAIRING_KEY"),
		PollWait:   durEnv("RELAYENT_POLL_WAIT", 25*time.Second),
		JobTimeout: durEnv("RELAYENT_JOB_TIMEOUT", 180*time.Second),
	}
	if c.RelayURL == "" {
		return c, fmt.Errorf("no relay configured (RELAYENT_RELAY_URL)")
	}
	if c.PairingKey == "" {
		return c, fmt.Errorf("no pairing key configured (RELAYENT_PAIRING_KEY)")
	}
	// Refuse to ship the key over plaintext to a remote host. This is a config
	// error, not a warning: by the time a job runs, the key is already exposed.
	if err := validateRelayURL(c.RelayURL); err != nil {
		return c, err
	}
	return c, nil
}

// durEnv reads an integer-seconds env var into a Duration, or returns def.
func durEnv(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return def
}
