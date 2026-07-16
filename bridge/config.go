// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Bridge configuration, loaded from environment variables (a TOML
//   config file at ~/.relayent/config.toml can be layered on later).
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config controls the bridge daemon.
type Config struct {
	RelayURL   string        // base URL of the relay, e.g. https://relay.example.com
	PairingKey string        // bearer key identifying this user's job namespace
	PollWait   time.Duration // long-poll wait per /v1/jobs/next request
	JobTimeout time.Duration // max wall-clock per CLI invocation
}

// LoadConfig reads configuration from the environment and validates it.
func LoadConfig() (Config, error) {
	c := Config{
		RelayURL:   os.Getenv("RELAYENT_RELAY_URL"),
		PairingKey: os.Getenv("RELAYENT_PAIRING_KEY"),
		PollWait:   durEnv("RELAYENT_POLL_WAIT", 25*time.Second),
		JobTimeout: durEnv("RELAYENT_JOB_TIMEOUT", 180*time.Second),
	}
	if c.RelayURL == "" {
		return c, fmt.Errorf("RELAYENT_RELAY_URL is required")
	}
	if c.PairingKey == "" {
		return c, fmt.Errorf("RELAYENT_PAIRING_KEY is required")
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
