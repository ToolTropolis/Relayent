// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Unit tests for the relay job queue — round-trip, key scoping,
//   bridge-presence, and long-poll blocking.
// AI usage: Built with assistance from AI tools for implementation acceleration,
//   review, and refactoring.
package main

import (
	"testing"
	"time"

	"github.com/navjyotnishant/relayent/internal/api"
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
