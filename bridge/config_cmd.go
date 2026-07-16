// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: `relayent-bridge config` — inspect and change configuration the
//
//	way `aws configure` does: `list` to see what is set and where it came from,
//	`get`/`set`/`unset` for individual values. Settings are stored in
//	~/.relayent/config.env (0600); environment variables always win, and `list`
//	shows which source is in effect so a surprising value is explainable.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// setting describes one configurable value: how it is named on the CLI, the env
// var it maps to, and how it should be displayed and validated.
type setting struct {
	name   string // CLI name, e.g. "workspace"
	env    string // env var / config file key
	help   string
	secret bool               // mask when displayed; never printed by `get` without --reveal
	def    func() string      // default shown when unset
	valid  func(string) error // rejects bad values at set time, not at first job
}

// settings is the full configurable surface. Keeping it as data means `list`,
// `get`, `set` and the help text can never drift apart.
var settings = []setting{
	{
		name:  "relay-url",
		env:   "RELAYENT_RELAY_URL",
		help:  "Base URL of the relay to pull jobs from",
		valid: validateRelayURL,
	},
	{
		name:   "pairing-key",
		env:    "RELAYENT_PAIRING_KEY",
		help:   "Credential identifying your job namespace — keep it secret",
		secret: true,
		valid: func(v string) error {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("pairing key cannot be empty")
			}
			return nil
		},
	},
	{
		name: "workspace",
		env:  "RELAYENT_WORKSPACE",
		help: "Directory jobs run in (default: an empty sandbox, no access to your files)",
		def:  func() string { ws, _ := DefaultWorkspace(); return ws },
		valid: func(v string) error {
			_, err := resolveWorkspace(v)
			return err
		},
	},
	{
		name:  "poll-wait",
		env:   "RELAYENT_POLL_WAIT",
		help:  "Seconds to hold each long-poll open",
		def:   func() string { return "25" },
		valid: positiveSeconds,
	},
	{
		name:  "job-timeout",
		env:   "RELAYENT_JOB_TIMEOUT",
		help:  "Max seconds a single CLI invocation may run",
		def:   func() string { return "180" },
		valid: positiveSeconds,
	},
}

func positiveSeconds(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fmt.Errorf("must be a whole number of seconds")
	}
	if n <= 0 {
		return fmt.Errorf("must be greater than zero")
	}
	return nil
}

// tailOf returns the last n characters of s, for masked display.
func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// truncate shortens a value for the fixed-width table, marking the elision so a
// clipped path is never mistaken for the real one.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func findSetting(name string) (setting, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range settings {
		// Accept the env var name too — people copy those out of docs and scripts.
		if s.name == name || strings.EqualFold(s.env, name) {
			return s, true
		}
	}
	return setting{}, false
}

const configUsage = `relayent-bridge config — inspect and change configuration

USAGE
  relayent-bridge config list              Show every setting, its value and source
  relayent-bridge config get <name>        Print one value
  relayent-bridge config set <name> <val>  Set one value
  relayent-bridge config unset <name>      Remove a value (revert to default)
  relayent-bridge config path              Print the config file location

SETTINGS
  relay-url      Base URL of the relay to pull jobs from
  pairing-key    Your credential — masked by default (see --reveal)
  workspace      Directory jobs run in (default: an empty sandbox)
  poll-wait      Seconds to hold each long-poll open (default 25)
  job-timeout    Max seconds per CLI invocation (default 180)

NOTES
  Values live in ~/.relayent/config.env (owner-only). Environment variables
  override the file — 'config list' shows which source is winning.
  'config set' validates before writing, so a bad value fails now, not at the
  first job. Restart the bridge for changes to take effect:
    relayent-bridge uninstall && relayent-bridge install
`

// RunConfig dispatches `relayent-bridge config …`.
func RunConfig(args []string) error {
	if len(args) == 0 {
		fmt.Print(configUsage)
		return nil
	}
	switch args[0] {
	case "list", "ls":
		return configList()
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: relayent-bridge config get <name>")
		}
		reveal := len(args) > 2 && args[2] == "--reveal"
		return configGet(args[1], reveal)
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: relayent-bridge config set <name> <value>")
		}
		return configSet(args[1], strings.Join(args[2:], " "))
	case "unset":
		if len(args) < 2 {
			return fmt.Errorf("usage: relayent-bridge config unset <name>")
		}
		return configUnset(args[1])
	case "path":
		p, err := ConfigPath()
		if err != nil {
			return err
		}
		fmt.Println(p)
		return nil
	case "-h", "--help", "help":
		fmt.Print(configUsage)
		return nil
	default:
		return fmt.Errorf("unknown config command %q\n\n%s", args[0], configUsage)
	}
}

// resolveValue reports a setting's effective value and where it came from, so a
// user can tell why a value is what it is rather than guessing.
func resolveValue(s setting, file map[string]string) (value, source string) {
	if v := os.Getenv(s.env); v != "" {
		return v, "env"
	}
	if v, ok := file[s.env]; ok && v != "" {
		return v, "config file"
	}
	if s.def != nil {
		return s.def(), "default"
	}
	return "", "not set"
}

func configList() error {
	file, err := LoadFileConfig()
	if err != nil {
		return err
	}
	path, _ := ConfigPath()

	fmt.Println()
	fmt.Printf("  Config file: %s\n", path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("               (does not exist yet — run 'relayent-bridge setup')")
	}
	fmt.Println()
	fmt.Printf("  %-14s %-42s %s\n", "SETTING", "VALUE", "SOURCE")
	fmt.Printf("  %-14s %-42s %s\n", strings.Repeat("─", 14), strings.Repeat("─", 42), strings.Repeat("─", 11))

	for _, s := range settings {
		v, src := resolveValue(s, file)
		shown := v
		switch {
		case v == "":
			shown = "—"
		case s.secret:
			// Show only the tail plus the fingerprint — enough to recognise which
			// key this is (and match it against the relay's status page) without
			// putting the credential itself on screen. aws prints ****ABCD; the
			// fingerprint is the more useful identifier here.
			shown = "****" + tailOf(v, 4) + "  (fp " + fingerprint(v) + ")"
		default:
			shown = truncate(v, 42)
		}
		fmt.Printf("  %-14s %-42s %s\n", s.name, shown, src)
	}
	fmt.Println()
	fmt.Println("  Environment variables override the config file.")
	fmt.Println("  Change a value:  relayent-bridge config set <setting> <value>")
	fmt.Println()
	return nil
}

func configGet(name string, reveal bool) error {
	s, ok := findSetting(name)
	if !ok {
		return unknownSetting(name)
	}
	file, err := LoadFileConfig()
	if err != nil {
		return err
	}
	v, _ := resolveValue(s, file)
	if v == "" {
		return fmt.Errorf("%s is not set", s.name)
	}
	// The pairing key spends real money. Printing it on request is fine; printing
	// it by default puts it in scrollback, screenshots and pasted terminal dumps.
	if s.secret && !reveal {
		fmt.Println(maskKey(v))
		fmt.Fprintf(os.Stderr, "\n  (masked — use 'config get %s --reveal' to print it in full)\n", s.name)
		return nil
	}
	fmt.Println(v)
	return nil
}

func configSet(name, value string) error {
	s, ok := findSetting(name)
	if !ok {
		return unknownSetting(name)
	}
	value = strings.TrimSpace(value)
	if s.valid != nil {
		if err := s.valid(value); err != nil {
			return fmt.Errorf("invalid %s: %w", s.name, err)
		}
	}
	if err := writeSetting(s.env, value); err != nil {
		return err
	}

	shown := value
	if s.secret {
		shown = maskKey(value) + "  (" + fingerprint(value) + ")"
	}
	fmt.Printf("\n  ✓ %s = %s\n", s.name, shown)

	// A value set in the environment silently wins over the file. Saying so beats
	// letting someone wonder why their change had no effect.
	if env := os.Getenv(s.env); env != "" && env != value {
		fmt.Printf("  ! %s is also set in your environment, which overrides this file.\n", s.env)
		fmt.Printf("    Unset it (unset %s) for this value to take effect.\n", s.env)
	}
	if s.name == "workspace" {
		home, _ := os.UserHomeDir()
		if abs, err := resolveWorkspace(value); err == nil && abs == home {
			fmt.Println("  ! This is your home directory. Jobs will run against your personal files")
			fmt.Println("    and macOS will prompt for Desktop/Documents access. Prefer a project")
			fmt.Println("    folder, or 'config unset workspace' for the empty sandbox.")
		}
	}
	fmt.Println("\n  Restart the bridge to apply:")
	fmt.Println("    relayent-bridge uninstall && relayent-bridge install")
	fmt.Println()
	return nil
}

func configUnset(name string) error {
	s, ok := findSetting(name)
	if !ok {
		return unknownSetting(name)
	}
	if err := writeSetting(s.env, ""); err != nil {
		return err
	}
	fmt.Printf("\n  ✓ %s removed from the config file\n", s.name)
	if s.def != nil {
		fmt.Printf("    Now using the default: %s\n", s.def())
	}
	fmt.Println()
	return nil
}

func unknownSetting(name string) error {
	valid := make([]string, 0, len(settings))
	for _, s := range settings {
		valid = append(valid, s.name)
	}
	sort.Strings(valid)
	return fmt.Errorf("unknown setting %q — valid settings: %s", name, strings.Join(valid, ", "))
}

// writeSetting updates one key in the config file, preserving the rest of the
// file (comments included) and rewriting atomically at 0600. An empty value
// removes the key. Rewriting in place rather than regenerating means a user's own
// comments and any keys a newer version adds are never silently dropped.
func writeSetting(key, value string) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(pathDir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var lines []string
	replaced := false
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				lines = append(lines, line)
				continue
			}
			k, _, ok := strings.Cut(trimmed, "=")
			if ok && strings.TrimSpace(k) == key {
				if value != "" {
					lines = append(lines, key+"="+value)
				}
				replaced = true // dropping the line when value=="" is the unset case
				continue
			}
			lines = append(lines, line)
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return fmt.Errorf("read config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("open config: %w", err)
	} else {
		lines = append(lines,
			"# Relayent bridge configuration",
			"# Written by 'relayent-bridge config' on "+time.Now().Format(time.RFC3339),
			"#",
			"# Contains your pairing key — a credential. Anyone who reads it can route",
			"# AI jobs to this machine and spend your CLI subscription. Keep it 0600;",
			"# do not commit or share it.",
			"")
	}
	if !replaced && value != "" {
		lines = append(lines, key+"="+value)
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("install config: %w", err)
	}
	return nil
}

// pathDir returns the directory part of a path without importing path/filepath
// into this file's surface area for one call.
func pathDir(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "."
}
