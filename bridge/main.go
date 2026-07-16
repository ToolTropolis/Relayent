// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Relayent bridge — a daemon the user runs on their own machine. It
//   dials OUT to the relay (no inbound ports), long-polls for jobs, runs the
//   matching local CLI adapter headless (reusing the CLI's subscription auth),
//   and posts the result back. Works behind NAT/firewalls.
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/navjyotnishant/relayent/bridge/adapters"
	"github.com/navjyotnishant/relayent/internal/api"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("[relayent-bridge] config error: %v", err)
	}
	reg := NewRegistry()

	log.Printf("[relayent-bridge] relay=%s available backends=%v",
		cfg.RelayURL, reg.Available())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b := &bridge{cfg: cfg, reg: reg, http: &http.Client{Timeout: cfg.PollWait + 10*time.Second}}
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
	for {
		if ctx.Err() != nil {
			return
		}
		job, ok, err := b.claimNext(ctx)
		if err != nil {
			log.Printf("[relayent-bridge] poll error: %v (retrying in %s)", err, backoff)
			if !sleepCtx(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
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
		res = api.ResultRequest{OK: false, Error: fmt.Sprintf("backend %q CLI not installed on this machine", job.Backend)}
	} else {
		jobCtx, cancel := context.WithTimeout(ctx, b.cfg.JobTimeout)
		out, runErr := adapter.Run(jobCtx, adapters.Request{
			Model:      job.Model,
			Prompt:     job.Prompt,
			System:     job.System,
			JSONSchema: job.JSONSchema,
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
