// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Unit tests for the relay job queue — round-trip, key scoping,
//
//	bridge-presence, and long-poll blocking.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"testing"
	"time"

	"github.com/ToolTropolis/Relayent/internal/api"
)

func newTestQueue() *Queue {
	return NewQueue(time.Minute, 40*time.Second)
}

func TestEnqueueClaimResultFetch(t *testing.T) {
	q := newTestQueue()
	q.Enqueue("k1", "job1", api.Job{ID: "job1", Backend: "claude", Prompt: "hi"})

	job, ok := q.ClaimNext("k1", time.Second)
	if !ok || job.ID != "job1" {
		t.Fatalf("expected to claim job1, got ok=%v id=%q", ok, job.ID)
	}

	if !q.SetResult("k1", "job1", api.ResultRequest{OK: true, Text: "hello"}) {
		t.Fatal("SetResult failed for valid job")
	}

	res, ok := q.Fetch("k1", "job1", 0)
	if !ok || res.Status != api.StatusDone || res.Text != "hello" {
		t.Fatalf("unexpected fetch result: %+v ok=%v", res, ok)
	}
}

func TestKeyScoping(t *testing.T) {
	q := newTestQueue()
	q.Enqueue("k1", "job1", api.Job{ID: "job1", Backend: "claude", Prompt: "x"})

	// A different key must not claim k1's job.
	if _, ok := q.ClaimNext("k2", 100*time.Millisecond); ok {
		t.Fatal("k2 should not claim k1's job")
	}
	// A different key must not post results to k1's job.
	if q.SetResult("k2", "job1", api.ResultRequest{OK: true}) {
		t.Fatal("k2 should not set result on k1's job")
	}
	// A different key must not fetch k1's job.
	if _, ok := q.Fetch("k2", "job1", 0); ok {
		t.Fatal("k2 should not fetch k1's job")
	}
}

func TestBridgeOnline(t *testing.T) {
	q := newTestQueue()
	if q.BridgeOnline("k1") {
		t.Fatal("no bridge has polled; should be offline")
	}
	// A poll (even one that times out with no job) marks the bridge online.
	q.ClaimNext("k1", 10*time.Millisecond)
	if !q.BridgeOnline("k1") {
		t.Fatal("after polling, bridge should be online")
	}
}

func TestClaimNextBlocksUntilEnqueue(t *testing.T) {
	q := newTestQueue()
	done := make(chan api.Job, 1)
	go func() {
		job, _ := q.ClaimNext("k1", 2*time.Second)
		done <- job
	}()
	// Give the claimer time to register as a waiter, then enqueue.
	time.Sleep(50 * time.Millisecond)
	q.Enqueue("k1", "job1", api.Job{ID: "job1", Backend: "codex", Prompt: "y"})

	select {
	case job := <-done:
		if job.ID != "job1" {
			t.Fatalf("expected job1, got %q", job.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("ClaimNext did not wake on enqueue")
	}
}

// Cancelling a still-queued job must actually prevent the work — that is the
// only case where cancellation saves anything.
func TestCancelPendingRemovesItFromTheQueue(t *testing.T) {
	q := NewQueue(time.Minute, time.Minute)
	q.Enqueue("k", "j1", api.Job{ID: "j1", Backend: "cursor", Prompt: "x"})

	state, ok := q.Cancel("k", "j1")
	if !ok || state != "pending" {
		t.Fatalf("Cancel = (%q, %v), want (\"pending\", true)", state, ok)
	}
	// A bridge must never receive it.
	if _, claimed := q.ClaimNext("k", 50*time.Millisecond); claimed {
		t.Error("a cancelled job must not be claimable by a bridge")
	}
	res, _ := q.Fetch("k", "j1", 0)
	if res.Status != api.StatusError {
		t.Errorf("status = %q, want error", res.Status)
	}
}

// A claimed job can be marked cancelled, but honestly: the work is already gone.
func TestCancelClaimedJobReportsRunning(t *testing.T) {
	q := NewQueue(time.Minute, time.Minute)
	q.Enqueue("k", "j1", api.Job{ID: "j1", Backend: "cursor", Prompt: "x"})
	if _, ok := q.ClaimNext("k", time.Second); !ok {
		t.Fatal("setup: bridge should have claimed the job")
	}
	state, ok := q.Cancel("k", "j1")
	if !ok || state != "running" {
		t.Fatalf("Cancel = (%q, %v), want (\"running\", true)", state, ok)
	}
}

// Cancel must unblock a caller waiting in Fetch, or the whole point is lost.
func TestCancelUnblocksAWaitingFetch(t *testing.T) {
	q := NewQueue(time.Minute, time.Minute)
	q.Enqueue("k", "j1", api.Job{ID: "j1", Backend: "cursor", Prompt: "x"})

	done := make(chan api.JobResult, 1)
	go func() { r, _ := q.Fetch("k", "j1", 5*time.Second); done <- r }()
	time.Sleep(50 * time.Millisecond) // let Fetch block
	q.Cancel("k", "j1")

	select {
	case r := <-done:
		if r.Status != api.StatusError {
			t.Errorf("status = %q, want error", r.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel did not unblock the waiting Fetch")
	}
}

// Cross-tenant safety: one key must never cancel another's job.
func TestCancelIsScopedToThePairingKey(t *testing.T) {
	q := NewQueue(time.Minute, time.Minute)
	q.Enqueue("owner", "j1", api.Job{ID: "j1", Backend: "cursor", Prompt: "x"})
	if _, ok := q.Cancel("attacker", "j1"); ok {
		t.Fatal("another pairing key must NOT be able to cancel this job")
	}
	res, _ := q.Fetch("owner", "j1", 0)
	if res.Status != api.StatusPending {
		t.Errorf("job should be untouched, status = %q", res.Status)
	}
	if _, ok := q.Cancel("owner", "nonexistent"); ok {
		t.Error("cancelling an unknown job should report not-ok")
	}
}

// A finished job cannot be cancelled — the caller must not be told otherwise.
func TestCancelFinishedJobIsANoop(t *testing.T) {
	q := NewQueue(time.Minute, time.Minute)
	q.Enqueue("k", "j1", api.Job{ID: "j1", Backend: "cursor", Prompt: "x"})
	q.ClaimNext("k", time.Second)
	q.SetResult("k", "j1", api.ResultRequest{OK: true, Text: "finished"})

	state, ok := q.Cancel("k", "j1")
	if !ok || state != api.StatusDone {
		t.Fatalf("Cancel = (%q, %v), want (\"done\", true)", state, ok)
	}
	res, _ := q.Fetch("k", "j1", 0)
	if res.Status != api.StatusDone || res.Text != "finished" {
		t.Errorf("a completed job's result must survive cancel: %+v", res)
	}
}
