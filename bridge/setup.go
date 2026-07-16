// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: `relayent-bridge setup` — an interactive pairing wizard, and
//
//	`relayent-bridge status` / `doctor` for diagnosis. Config is written to
//	~/.relayent/config.env with 0600 permissions; the pairing key is a
//	credential and is never printed back in full.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// configDirName is the per-user config directory under $HOME.
const configDirName = ".relayent"

// ConfigPath returns the path to the bridge's config file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, configDirName, "config.env"), nil
}

// fingerprint mirrors the relay's key fingerprint so a user can match what the
// bridge holds against what the relay reports, without either side showing the key.
func fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:8]
}

// maskKey renders a key for display without disclosing it. Terminals scroll back
// and get screenshotted, so the wizard never echoes a key in full.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "********"
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// LoadFileConfig reads ~/.relayent/config.env into a map. Missing file is not an
// error — the env vars alone are a valid way to run.
func LoadFileConfig() (map[string]string, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// Values may be quoted by hand or by the wizard; accept both.
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out, sc.Err()
}

// saveConfig writes the config file atomically with 0600 permissions. The file
// holds the pairing key, so it must never be group/world readable — and it is
// written via a temp file so a crash cannot leave a truncated (or worse,
// permissive) file behind.
func saveConfig(relayURL, key string) (string, error) {
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	content := fmt.Sprintf(`# Relayent bridge configuration
# Written by 'relayent-bridge setup' on %s
#
# This file contains your pairing key — a credential. Anyone who reads it can
# route AI jobs to this machine and spend your CLI subscription. It is chmod
# 0600 (owner-only) on purpose; do not commit it or share it.

RELAYENT_RELAY_URL=%s
RELAYENT_PAIRING_KEY=%s
`, time.Now().Format(time.RFC3339), relayURL, key)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("install config: %w", err)
	}
	return path, nil
}

// validateRelayURL rejects URLs that would send the pairing key somewhere unsafe.
// Plain http:// is allowed only for loopback: over the network it would put the
// key and every prompt on the wire in cleartext.
func validateRelayURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("not a valid URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return fmt.Errorf("refusing http:// for a remote relay (%s): your pairing key and every\n"+
			"  prompt would cross the network unencrypted. Use https:// instead", host)
	case "":
		return fmt.Errorf("missing scheme — use https://relay.example.com")
	default:
		return fmt.Errorf("unsupported scheme %q — use https://", u.Scheme)
	}
}

// probeRelay checks the relay is reachable and the key is accepted, so setup
// fails at pairing time rather than silently at the first job.
func probeRelay(relayURL, key string) error {
	req, err := http.NewRequest("GET", strings.TrimRight(relayURL, "/")+"/v1/status", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach the relay: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var st struct {
			Version     string `json:"version"`
			TLS         bool   `json:"tls"`
			KeyRetiring bool   `json:"key_retiring"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&st); err == nil {
			fmt.Printf("  ✓ Paired with relay (version %s)\n", st.Version)
			if !st.TLS && !strings.HasPrefix(relayURL, "http://localhost") &&
				!strings.HasPrefix(relayURL, "http://127.0.0.1") {
				fmt.Println("  ! WARNING: this relay is not using TLS — traffic is unencrypted.")
			}
			if st.KeyRetiring {
				fmt.Println("  ! NOTE: this key is being rotated out and will stop working soon.")
				fmt.Println("    Ask the relay operator for the new key.")
			}
		}
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("the relay rejected this pairing key (401). Check it and try again")
	case http.StatusTooManyRequests:
		return fmt.Errorf("the relay is rate-limiting this machine (429) — wait a minute and retry")
	default:
		return fmt.Errorf("unexpected relay response: %s", resp.Status)
	}
}

// RunSetup is the interactive pairing wizard. It states the trust model up front:
// pairing hands the relay the ability to spend this machine's CLI subscription,
// and a user should understand that before, not after.
func RunSetup() error {
	in := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  Relayent bridge setup")
	fmt.Println("  ─────────────────────")
	fmt.Println()
	fmt.Println("  This machine will run AI jobs on YOUR logged-in CLI subscription")
	fmt.Println("  (Claude Code / Codex / Cursor) on behalf of apps that send jobs to")
	fmt.Println("  a relay you trust.")
	fmt.Println()
	fmt.Println("  What this does NOT do:")
	fmt.Println("    • open any port on this machine (the bridge only dials out)")
	fmt.Println("    • store or transmit your CLI credentials")
	fmt.Println("    • let jobs edit files or run shell commands")
	fmt.Println()
	fmt.Println("  What to keep in mind:")
	fmt.Println("    • anyone holding the pairing key can send jobs here and use your quota")
	fmt.Println("    • only pair with a relay you control or trust")
	fmt.Println()

	existing, _ := LoadFileConfig()

	relayURL := strings.TrimSpace(existing["RELAYENT_RELAY_URL"])
	for {
		prompt := "  Relay URL (e.g. https://relay.example.com): "
		if relayURL != "" {
			prompt = fmt.Sprintf("  Relay URL [%s]: ", relayURL)
		}
		fmt.Print(prompt)
		line, err := in.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		if v := strings.TrimSpace(line); v != "" {
			relayURL = v
		}
		if relayURL == "" {
			fmt.Println("  A relay URL is required.")
			continue
		}
		if err := validateRelayURL(relayURL); err != nil {
			fmt.Printf("  ✗ %v\n", err)
			relayURL = ""
			continue
		}
		break
	}

	key := strings.TrimSpace(existing["RELAYENT_PAIRING_KEY"])
	for {
		prompt := "  Pairing key (from the relay operator): "
		if key != "" {
			prompt = fmt.Sprintf("  Pairing key [keep existing %s]: ", maskKey(key))
		}
		fmt.Print(prompt)
		line, err := in.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		if v := strings.TrimSpace(line); v != "" {
			key = v
		}
		if key == "" {
			fmt.Println("  A pairing key is required.")
			continue
		}
		break
	}

	fmt.Println()
	fmt.Println("  Checking the relay…")
	if err := probeRelay(relayURL, key); err != nil {
		fmt.Printf("  ✗ %v\n", err)
		fmt.Println()
		fmt.Println("  Nothing was saved. Fix the problem above and run setup again.")
		return err
	}

	path, err := saveConfig(relayURL, key)
	if err != nil {
		return err
	}
	fmt.Printf("  ✓ Saved %s (owner-only, 0600)\n", path)
	fmt.Printf("  ✓ Key fingerprint: %s  (matches the relay's status page)\n", fingerprint(key))
	fmt.Println()

	reg := NewRegistry()
	fmt.Println("  Backends on this machine:")
	for _, b := range reg.Describe() {
		switch {
		case b.Ready:
			fmt.Printf("    ✓ %-8s ready\n", b.Name)
		case !b.Installed:
			fmt.Printf("    · %-8s CLI not installed\n", b.Name)
		case !b.Supported:
			fmt.Printf("    · %-8s CLI found, adapter not implemented yet\n", b.Name)
		}
	}
	fmt.Println()
	fmt.Println("  Next:")
	fmt.Println("    relayent-bridge            run it now (foreground)")
	fmt.Println("    relayent-bridge install    run it always, in the background, at login")
	fmt.Println()
	return nil
}

// RunDoctor diagnoses a bridge install: config, relay reachability, backends.
// It exists so "it doesn't work" has a single, obvious first step.
func RunDoctor() error {
	fmt.Println()
	fmt.Println("  Relayent bridge doctor")
	fmt.Println("  ──────────────────────")
	fmt.Println()

	path, _ := ConfigPath()
	fileCfg, err := LoadFileConfig()
	if err != nil {
		fmt.Printf("  ✗ config: %v\n", err)
	} else if len(fileCfg) == 0 {
		fmt.Printf("  · config: none at %s (using environment variables)\n", path)
	} else {
		fmt.Printf("  ✓ config: %s\n", path)
		if fi, err := os.Stat(path); err == nil {
			if perm := fi.Mode().Perm(); perm != 0o600 {
				fmt.Printf("  ! permissions are %04o — should be 0600 (run: chmod 600 %s)\n", perm, path)
			}
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Printf("  ✗ %v\n", err)
		fmt.Println()
		fmt.Println("  Run 'relayent-bridge setup' to configure this machine.")
		return err
	}
	fmt.Printf("  ✓ relay URL: %s\n", cfg.RelayURL)
	fmt.Printf("  ✓ key fingerprint: %s\n", fingerprint(cfg.PairingKey))

	if err := validateRelayURL(cfg.RelayURL); err != nil {
		fmt.Printf("  ! %v\n", err)
	}

	fmt.Println()
	fmt.Println("  Relay:")
	if err := probeRelay(cfg.RelayURL, cfg.PairingKey); err != nil {
		fmt.Printf("  ✗ %v\n", err)
	}

	fmt.Println()
	fmt.Println("  Backends:")
	for _, b := range NewRegistry().Describe() {
		switch {
		case b.Ready:
			fmt.Printf("    ✓ %-8s ready\n", b.Name)
		case !b.Installed:
			fmt.Printf("    · %-8s CLI not installed\n", b.Name)
		case !b.Supported:
			fmt.Printf("    · %-8s CLI found, adapter not implemented yet\n", b.Name)
		}
	}
	fmt.Println()
	return nil
}
