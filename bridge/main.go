// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Relayent bridge — a daemon the user runs on their own machine. It
//
//	dials OUT to the relay (no inbound ports), long-polls for jobs, runs the
//	matching local CLI adapter headless (reusing the CLI's subscription auth),
//	and posts the result back. Works behind NAT/firewalls.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ToolTropolis/Relayent/bridge/adapters"
	"github.com/ToolTropolis/Relayent/internal/api"
)

// Version is the bridge build version, overridable at link time.
var Version = "1.0.0"

// bridgeUsage is the bridge's CLI help.
const bridgeUsage = `relayent-bridge — runs AI jobs on this machine's CLI subscription

USAGE
  relayent-bridge             Run the bridge (foreground)
  relayent-bridge setup       Pair this machine with a relay (interactive)
  relayent-bridge config      Show or change settings ('config list' to start)
  relayent-bridge monitor     Live status and logs in the terminal
  relayent-bridge install     Run automatically in the background at login
  relayent-bridge uninstall   Remove the login service
  relayent-bridge status      Show service status
  relayent-bridge doctor      Diagnose configuration and connectivity
  relayent-bridge help        Show this help

HOW IT WORKS
  The bridge dials OUT to a relay and pulls jobs. Nothing listens on this
  machine — no ports are opened and no inbound connection is ever accepted.
  Jobs run through your already-authenticated CLI, so no credentials are
  stored or transmitted by Relayent.

CONFIG
  Written by 'setup' to ~/.relayent/config.env (owner-only). Environment
  variables override the file:
    RELAYENT_RELAY_URL     Relay base URL (https:// required off-localhost)
    RELAYENT_PAIRING_KEY   Your pairing key — a credential; keep it secret
    RELAYENT_POLL_WAIT     Long-poll seconds per request (default 25)
    RELAYENT_JOB_TIMEOUT   Max seconds per CLI invocation (default 180)
    RELAYENT_{CLAUDE,CODEX,CURSOR,GEMINI}_BIN   Override a CLI path
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			if err := RunSetup(); err != nil {
				os.Exit(1)
			}
			return
		case "config":
			if err := RunConfig(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "\n  ✗ %v\n\n", err)
				os.Exit(1)
			}
			return
		case "monitor", "watch":
			if err := RunMonitor(); err != nil {
				fmt.Fprintf(os.Stderr, "\n  ✗ %v\n\n", err)
				os.Exit(1)
			}
			return
		case "install":
			if err := InstallService(); err != nil {
				fmt.Fprintf(os.Stderr, "\n  ✗ %v\n\n", err)
				os.Exit(1)
			}
			return
		case "uninstall":
			if err := UninstallService(); err != nil {
				fmt.Fprintf(os.Stderr, "\n  ✗ %v\n\n", err)
				os.Exit(1)
			}
			return
		case "status":
			if err := ServiceStatus(); err != nil {
				os.Exit(1)
			}
			return
		case "doctor":
			if err := RunDoctor(); err != nil {
				os.Exit(1)
			}
			return
		case "-h", "--help", "help":
			fmt.Print(bridgeUsage)
			return
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("[relayent-bridge] config error: %v\n\n  Run 'relayent-bridge setup' to pair this machine.", err)
	}
	reg := NewRegistry()

	log.Printf("[relayent-bridge] relay=%s available backends=%v",
		cfg.RelayURL, reg.Available())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Keep our own log files bounded: neither launchd nor systemd rotates a
	// service's StandardOutPath/StandardErrorPath, so without this the bridge
	// would append to one file for as long as it is installed.
	go logRotationLoop(ctx.Done())

	b := &bridge{cfg: cfg, reg: reg, http: &http.Client{Timeout: cfg.PollWait + 10*time.Second}}
	// Publish what this machine can do so the relay's status API can surface it,
	// then keep it fresh in the background.
	b.reportCapabilities(ctx)
	go b.capabilitiesLoop(ctx)

	b.run(ctx)
	log.Printf("[relayent-bridge] shutting down")
}

type bridge struct {
	cfg  Config
	reg  *Registry
	http *http.Client
}

// run is the poll loop: claim a job, process it, repeat, until ctx is cancelled.
func (b *bridge) run(ctx context.Context) {
	backoff := time.Second
	pollFailed := false                // true after a poll error, so recovery can re-report caps
	var lastCaps time.Time             // when caps were last reported from this loop
	const capsEvery = 15 * time.Second // keep the relay's readiness fresh to ~this
	for {
		if ctx.Err() != nil {
			return
		}
		job, ok, err := b.claimNext(ctx)
		if err != nil {
			log.Printf("[relayent-bridge] poll error: %v (retrying in %s)", err, backoff)
			pollFailed = true
			if !sleepCtx(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		// Keep readiness current: re-scan and report on recovery from a poll failure
		// (a relay restart loses in-memory caps), and otherwise at most every
		// capsEvery so a newly installed/removed CLI surfaces within a poll cycle
		// rather than waiting on the slow fallback ticker. Describe() re-detects the
		// CLIs each call, so this is always a fresh scan, not a cached one.
		if pollFailed || time.Since(lastCaps) >= capsEvery {
			pollFailed = false
			b.reportCapabilities(ctx)
			lastCaps = time.Now()
		}
		if !ok {
			continue // 204: no job within the wait window; poll again
		}
		b.process(ctx, job)
	}
}

// claimNext long-polls GET /v1/jobs/next. ok=false means "no job right now".
func (b *bridge) claimNext(ctx context.Context) (api.Job, bool, error) {
	url := fmt.Sprintf("%s/v1/jobs/next?wait=%d", strings.TrimRight(b.cfg.RelayURL, "/"),
		int(b.cfg.PollWait.Seconds()))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	b.authorize(req)

	resp, err := b.http.Do(req)
	if err != nil {
		return api.Job{}, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return api.Job{}, false, nil
	case http.StatusOK:
		var job api.Job
		if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
			return api.Job{}, false, fmt.Errorf("decode job: %w", err)
		}
		return job, true, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return api.Job{}, false, fmt.Errorf("relay returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// process runs a job's adapter and posts the result (success or error) back.
func (b *bridge) process(ctx context.Context, job api.Job) {
	log.Printf("[relayent-bridge] job %s backend=%s model=%s", job.ID, job.Backend, job.Model)

	res := api.ResultRequest{}
	adapter, err := b.reg.Get(job.Backend)
	if err != nil {
		res = api.ResultRequest{OK: false, Error: err.Error()}
	} else if !adapter.Available() {
		res = api.ResultRequest{OK: false, Error: b.unavailableReason(job.Backend, adapter)}
	} else {
		jobCtx, cancel := context.WithTimeout(ctx, b.cfg.JobTimeout)
		out, runErr := adapter.Run(jobCtx, adapters.Request{
			Model:      job.Model,
			Prompt:     job.Prompt,
			System:     job.System,
			JSONSchema: job.JSONSchema,
			WorkDir:    b.cfg.Workspace,
		})
		cancel()
		if runErr != nil {
			res = api.ResultRequest{OK: false, Error: runErr.Error()}
		} else if out.IsJSON {
			res = api.ResultRequest{OK: true, JSON: out.JSON}
		} else {
			res = api.ResultRequest{OK: true, Text: out.Text}
		}
	}

	if err := b.postResult(ctx, job.ID, res); err != nil {
		log.Printf("[relayent-bridge] failed to post result for job %s: %v", job.ID, err)
		return
	}
	if res.OK {
		log.Printf("[relayent-bridge] job %s done", job.ID)
	} else {
		log.Printf("[relayent-bridge] job %s error: %s", job.ID, res.Error)
	}
}

// postResult sends the outcome to POST /v1/jobs/{id}/result.
func (b *bridge) postResult(ctx context.Context, id string, res api.ResultRequest) error {
	body, err := json.Marshal(res)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v1/jobs/%s/result", strings.TrimRight(b.cfg.RelayURL, "/"), id)
	// Use a fresh short timeout independent of the long-poll client.
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	b.authorize(req)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("relay returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// unavailableReason explains why a backend can't run, distinguishing "the adapter
// isn't implemented yet" from "the CLI isn't installed here".
func (b *bridge) unavailableReason(name string, a adapters.Adapter) string {
	if bp, ok := a.(interface{ BinPresent() bool }); ok {
		if bp.BinPresent() {
			return fmt.Sprintf("backend %q is not supported yet by this bridge (adapter is a stub)", name)
		}
		return fmt.Sprintf("backend %q is not supported yet and its CLI is not installed on this machine", name)
	}
	return fmt.Sprintf("backend %q CLI not installed on this machine", name)
}

// reportCapabilities tells the relay which backends exist on this machine, so
// GET /v1/bridge/capabilities and the status page can show them.
func (b *bridge) reportCapabilities(ctx context.Context) {
	host, _ := os.Hostname()
	caps := api.BridgeCapabilities{
		Version:  Version,
		Hostname: host,
		Backends: b.reg.Describe(ctx),
	}
	body, err := json.Marshal(caps)
	if err != nil {
		log.Printf("[relayent-bridge] marshal capabilities: %v", err)
		return
	}
	url := strings.TrimRight(b.cfg.RelayURL, "/") + "/v1/bridge/capabilities"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	b.authorize(req)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[relayent-bridge] report capabilities: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[relayent-bridge] report capabilities: relay returned %d", resp.StatusCode)
	}
}

// capabilitiesLoop refreshes the report periodically so a restarted relay (which
// holds this state in memory) re-learns what this bridge supports.
func (b *bridge) capabilitiesLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.reportCapabilities(ctx)
		}
	}
}

func (b *bridge) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+b.cfg.PairingKey)
}

// sleepCtx sleeps for d unless ctx is cancelled first. Returns false if cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
