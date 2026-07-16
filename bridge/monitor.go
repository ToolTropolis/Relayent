// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: `relayent-bridge monitor` — a live console dashboard: relay
//
//	reachability, bridge presence, backend readiness and a tail of the service
//	log, refreshed in place. Colour is used to make state scannable, and is
//	disabled automatically when stdout is not a terminal or NO_COLOR is set.
//	The pairing key is never shown — only its fingerprint — because this screen
//	is exactly what people screenshot when asking for help.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

// ANSI helpers. Empty strings when colour is off, so every call site stays the
// same and no escape codes leak into piped output.
type palette struct {
	reset, dim, bold, red, green, yellow, blue, cyan string
}

func newPalette(enabled bool) palette {
	if !enabled {
		return palette{}
	}
	return palette{
		reset: "\033[0m", dim: "\033[2m", bold: "\033[1m",
		red: "\033[31m", green: "\033[32m", yellow: "\033[33m",
		blue: "\033[34m", cyan: "\033[36m",
	}
}

// colourEnabled reports whether to emit ANSI codes. Honours NO_COLOR (the de
// facto standard) and detects a pipe/redirect via the character-device bit, so
// `monitor > file` and `monitor | grep` produce clean text.
func colourEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// monitorState is one poll's worth of information about the system.
type monitorState struct {
	relayURL  string
	keyFP     string
	workspace string

	reachable bool
	relayErr  string
	version   string
	uptime    int64
	online    bool
	pending   int
	tls       bool
	retiring  bool

	backends []struct {
		Name      string `json:"name"`
		Installed bool   `json:"installed"`
		Supported bool   `json:"supported"`
		Ready     bool   `json:"ready"`
	}

	serviceInstalled bool
	logLines         []string
}

// RunMonitor renders the live dashboard until interrupted.
func RunMonitor() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("%w\n\n  Run 'relayent-bridge setup' to pair this machine", err)
	}

	p := newPalette(colourEnabled())
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}

	// Alternate screen buffer + hidden cursor, so quitting restores the user's
	// terminal exactly as it was rather than leaving a wall of redraws.
	if p.reset != "" {
		fmt.Print("\033[?1049h\033[?25l")
		defer fmt.Print("\033[?25h\033[?1049l")
	}

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	for {
		st := pollState(ctx, client, cfg)
		render(p, st)

		select {
		case <-ctx.Done():
			// Leave a summary line behind on the normal screen.
			fmt.Println("\n  Monitor stopped.")
			return nil
		case <-tick.C:
		}
	}
}

func pollState(ctx context.Context, client *http.Client, cfg Config) monitorState {
	st := monitorState{
		relayURL:  cfg.RelayURL,
		keyFP:     fingerprint(cfg.PairingKey),
		workspace: cfg.Workspace,
	}

	// Relay status.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.RelayURL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.PairingKey)
	resp, err := client.Do(req)
	if err != nil {
		st.relayErr = condenseErr(err)
	} else {
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			var s struct {
				Version       string `json:"version"`
				UptimeSeconds int64  `json:"uptime_seconds"`
				BridgeOnline  bool   `json:"bridge_online"`
				PendingJobs   int    `json:"pending_jobs"`
				TLS           bool   `json:"tls"`
				KeyRetiring   bool   `json:"key_retiring"`
			}
			if json.NewDecoder(resp.Body).Decode(&s) == nil {
				st.reachable = true
				st.version, st.uptime = s.Version, s.UptimeSeconds
				st.online, st.pending = s.BridgeOnline, s.PendingJobs
				st.tls, st.retiring = s.TLS, s.KeyRetiring
			}
		case http.StatusUnauthorized:
			st.relayErr = "pairing key rejected (401)"
		case http.StatusTooManyRequests:
			st.relayErr = "rate limited (429)"
		default:
			st.relayErr = "relay returned " + resp.Status
		}
	}

	// Capabilities.
	if st.reachable {
		cReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.RelayURL+"/v1/bridge/capabilities", nil)
		cReq.Header.Set("Authorization", "Bearer "+cfg.PairingKey)
		if cResp, err := client.Do(cReq); err == nil {
			defer cResp.Body.Close()
			var c struct {
				Capabilities struct {
					Backends []struct {
						Name      string `json:"name"`
						Installed bool   `json:"installed"`
						Supported bool   `json:"supported"`
						Ready     bool   `json:"ready"`
					} `json:"backends"`
				} `json:"capabilities"`
			}
			if json.NewDecoder(cResp.Body).Decode(&c) == nil {
				st.backends = c.Capabilities.Backends
			}
		}
	}

	// Local backends when the relay has nothing to report (bridge not running).
	if len(st.backends) == 0 {
		for _, b := range NewRegistry().Describe(ctx) {
			st.backends = append(st.backends, struct {
				Name      string `json:"name"`
				Installed bool   `json:"installed"`
				Supported bool   `json:"supported"`
				Ready     bool   `json:"ready"`
			}{b.Name, b.Installed, b.Supported, b.Ready})
		}
	}

	if path, err := servicePaths(); err == nil {
		if _, err := os.Stat(path); err == nil {
			st.serviceInstalled = true
		}
	}
	st.logLines = tailLog(12)
	return st
}

// tailLog returns the last n lines of the service log, merged from both streams
// and ordered by timestamp. Files are size-capped by logRotationLoop.
//
// Both files matter: Go's log package writes to STDERR, so ordinary activity
// ("job … done") lands in the .err file, not the .out file. Reading only stdout
// left this panel permanently empty. Lines are prefixed "2006/01/02 15:04:05",
// which sorts lexicographically in chronological order — so a plain sort merges
// the two streams correctly without parsing timestamps.
func tailLog(n int) []string {
	out, errPath, err := logPaths()
	if err != nil {
		return nil
	}
	var lines []string
	for _, p := range []string{out, errPath} {
		lines = append(lines, readLines(p)...)
	}
	sort.SliceStable(lines, func(i, j int) bool {
		return logTimestamp(lines[i]) < logTimestamp(lines[j])
	})
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// readLines returns the non-empty lines of a file, or nil if it cannot be read.
func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			lines = append(lines, t)
		}
	}
	return lines
}

// logTimestamp extracts the leading "2006/01/02 15:04:05" prefix for ordering.
// Lines without one sort first, which keeps them visible rather than dropped.
func logTimestamp(l string) string {
	const stamp = len("2006/01/02 15:04:05")
	if len(l) >= stamp && l[4] == '/' && l[7] == '/' && l[13] == ':' {
		return l[:stamp]
	}
	return ""
}

// condenseErr turns a verbose net/http error into something that fits a line.
func condenseErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "connection refused"):
		return "connection refused — is the relay running?"
	case strings.Contains(s, "no such host"):
		return "DNS lookup failed — check the relay URL"
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded"):
		return "timed out"
	case strings.Contains(s, "certificate"):
		return "TLS certificate error"
	}
	if i := strings.LastIndex(s, ": "); i > 0 && len(s) > 70 {
		return s[i+2:]
	}
	return s
}

func render(p palette, st monitorState) {
	var b strings.Builder

	if p.reset != "" {
		b.WriteString("\033[H\033[2J") // home + clear
	}

	line := func(s string) { b.WriteString("  " + s + "\n") }

	ok := func(s string) string { return p.green + s + p.reset }
	bad := func(s string) string { return p.red + s + p.reset }
	warn := func(s string) string { return p.yellow + s + p.reset }
	dim := func(s string) string { return p.dim + s + p.reset }
	key := func(s string) string { return fmt.Sprintf("%-18s", s) }

	b.WriteString("\n")
	line(p.bold + "Relayent bridge" + p.reset + dim("  ·  monitor  ·  Ctrl-C to quit"))
	b.WriteString("\n")

	// --- connection ---
	line(p.bold + "CONNECTION" + p.reset)
	line(dim(key("Relay")) + st.relayURL)
	if st.reachable {
		status := ok("● reachable")
		if !st.tls && !strings.HasPrefix(st.relayURL, "http://localhost") &&
			!strings.HasPrefix(st.relayURL, "http://127.0.0.1") {
			status += warn("  (no TLS — traffic is in the clear)")
		}
		line(dim(key("Status")) + status)
		line(dim(key("Relay version")) + st.version + dim("  up "+fmtDur(st.uptime)))
	} else {
		line(dim(key("Status")) + bad("● unreachable") + dim("  "+st.relayErr))
	}
	line(dim(key("Key fingerprint")) + st.keyFP + func() string {
		if st.retiring {
			return warn("  (retiring — ask for the new key)")
		}
		return ""
	}())
	b.WriteString("\n")

	// --- bridge ---
	line(p.bold + "BRIDGE" + p.reset)
	switch {
	case st.online:
		line(dim(key("Polling")) + ok("● online"))
	case st.reachable:
		line(dim(key("Polling")) + bad("● offline") + dim("  (no bridge is pulling jobs for this key)"))
	default:
		line(dim(key("Polling")) + dim("unknown"))
	}
	if st.serviceInstalled {
		line(dim(key("Service")) + ok("installed") + dim("  (starts at login)"))
	} else {
		line(dim(key("Service")) + dim("not installed") + dim("  — relayent-bridge install"))
	}
	line(dim(key("Pending jobs")) + fmt.Sprint(st.pending))
	line(dim(key("Workspace")) + st.workspace)
	b.WriteString("\n")

	// --- backends ---
	line(p.bold + "BACKENDS" + p.reset)
	for _, bk := range st.backends {
		switch {
		case bk.Ready:
			line("  " + ok("✓") + " " + fmt.Sprintf("%-10s", bk.Name) + dim("ready"))
		case !bk.Installed:
			line("  " + dim("·") + " " + fmt.Sprintf("%-10s", bk.Name) + dim("CLI not installed"))
		case !bk.Supported:
			line("  " + warn("·") + " " + fmt.Sprintf("%-10s", bk.Name) + dim("CLI found, adapter is a stub"))
		default:
			line("  " + warn("·") + " " + fmt.Sprintf("%-10s", bk.Name) + dim("not ready"))
		}
	}
	b.WriteString("\n")

	// --- log ---
	line(p.bold + "RECENT ACTIVITY" + p.reset)
	if len(st.logLines) == 0 {
		if st.serviceInstalled {
			line(dim("  no activity logged yet — jobs will appear here as they run"))
		} else {
			line(dim("  no service log — 'relayent-bridge install', or run it in the foreground"))
		}
	} else {
		for _, l := range st.logLines {
			line("  " + colourLog(p, l))
		}
	}

	b.WriteString("\n")
	b.WriteString(dim("  refreshed " + time.Now().Format("15:04:05") + " · every 2s"))
	b.WriteString("\n")

	fmt.Print(b.String())
}

// colourLog tints a log line by severity so failures stand out in a scroll.
func colourLog(p palette, l string) string {
	low := strings.ToLower(l)
	switch {
	case strings.Contains(low, "error") || strings.Contains(low, "failed"):
		return p.red + l + p.reset
	case strings.Contains(low, " done"):
		return p.green + l + p.reset
	case strings.Contains(low, "job "):
		return p.cyan + l + p.reset
	}
	return p.dim + l + p.reset
}

func fmtDur(secs int64) string {
	d := time.Duration(secs) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}
