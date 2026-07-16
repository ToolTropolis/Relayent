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
	"path/filepath"
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

	// Workspace is the ONLY directory jobs run in. It defaults to a dedicated
	// empty sandbox (~/.relayent/workspace), never the user's home: a CLI
	// launched from $HOME inherits it as its working directory, which makes
	// macOS attribute any file access to the bridge and prompt for Desktop /
	// Documents / Downloads. Jobs have no need for the user's files, so the
	// default grants none — set RELAYENT_WORKSPACE deliberately to widen it.
	Workspace string
}

// DefaultWorkspace returns the dedicated sandbox directory jobs run in.
func DefaultWorkspace() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, configDirName, "workspace"), nil
}

// resolveWorkspace picks the job working directory and creates it. An explicit
// RELAYENT_WORKSPACE wins; otherwise the empty default sandbox is used.
func resolveWorkspace(configured string) (string, error) {
	ws := strings.TrimSpace(configured)
	if ws == "" {
		def, err := DefaultWorkspace()
		if err != nil {
			return "", err
		}
		ws = def
	}
	abs, err := filepath.Abs(ws)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("create workspace %s: %w", abs, err)
	}
	return abs, nil
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
	ws, err := resolveWorkspace(pick("RELAYENT_WORKSPACE"))
	if err != nil {
		return c, err
	}
	c.Workspace = ws
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
