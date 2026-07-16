// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: In-memory, per-pairing-key job broker for the Relayent relay.
//
//	Supports enqueue, long-poll claim by a bridge, result posting, result
//	fetching, and bridge-presence tracking (drives fail-fast). TTL-expires
//	orphaned jobs. Redis is the intended drop-in for multi-instance later.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"sync"
	"time"

	"github.com/navjyotnishant/relayent/internal/api"
)

// jobEntry is the relay's internal record for one job.
type jobEntry struct {
	job       api.Job
	status    string
	result    api.ResultRequest
	createdAt time.Time
	done      chan struct{} // closed when a result arrives; lets fetchers block
}

// Queue is a concurrency-safe, per-key job broker.
type Queue struct {
	mu sync.Mutex

	// pending[key] is a FIFO of job IDs awaiting a bridge for that pairing key.
	pending map[string][]string
	// jobs is the global id -> entry map (ids are globally unique).
	jobs map[string]*jobEntry
	// jobKey maps a job id back to its pairing key (for scoped result posting).
	jobKey map[string]string
	// waiters[key] are bridges blocked in ClaimNext for that key; signalled on enqueue.
	waiters map[string][]chan struct{}
	// lastPoll[key] is the last time any bridge polled ClaimNext for that key.
	lastPoll map[string]time.Time
	// caps[key] is the most recent capabilities a bridge reported for that key.
	caps map[string]capsEntry

	ttl          time.Duration // how long a finished/orphaned job is retained
	onlineWindow time.Duration // a bridge counts as "online" if it polled within this window
}

// capsEntry is a stored capabilities report with its timestamp.
type capsEntry struct {
	caps       api.BridgeCapabilities
	reportedAt time.Time
}

// NewQueue builds a Queue and starts its janitor goroutine.
func NewQueue(ttl, onlineWindow time.Duration) *Queue {
	q := &Queue{
		pending:      make(map[string][]string),
		jobs:         make(map[string]*jobEntry),
		jobKey:       make(map[string]string),
		waiters:      make(map[string][]chan struct{}),
		lastPoll:     make(map[string]time.Time),
		caps:         make(map[string]capsEntry),
		ttl:          ttl,
		onlineWindow: onlineWindow,
	}
	go q.janitor()
	return q
}

// Enqueue records a new job for a pairing key and wakes one waiting bridge.
func (q *Queue) Enqueue(key, id string, job api.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.jobs[id] = &jobEntry{
		job:       job,
		status:    api.StatusPending,
		createdAt: time.Now(),
		done:      make(chan struct{}),
	}
	q.jobKey[id] = key
	q.pending[key] = append(q.pending[key], id)

	// Wake exactly one blocked bridge, if any.
	if ws := q.waiters[key]; len(ws) > 0 {
		w := ws[0]
		q.waiters[key] = ws[1:]
		close(w)
	}
}

// popLocked removes and returns the next pending job id for key (caller holds lock).
func (q *Queue) popLocked(key string) (string, bool) {
	ids := q.pending[key]
	for len(ids) > 0 {
		id := ids[0]
		ids = ids[1:]
		q.pending[key] = ids
		// Skip ids that were expired/removed while queued.
		if _, ok := q.jobs[id]; ok {
			return id, true
		}
	}
	return "", false
}

// ClaimNext returns the next job for key, blocking up to wait for one to arrive.
// It also records the poll time so BridgeOnline can report presence.
func (q *Queue) ClaimNext(key string, wait time.Duration) (api.Job, bool) {
	q.mu.Lock()
	q.lastPoll[key] = time.Now()
	if id, ok := q.popLocked(key); ok {
		job := q.jobs[id].job
		q.mu.Unlock()
		return job, true
	}

	// Nothing queued: register a waiter and block.
	ch := make(chan struct{})
	q.waiters[key] = append(q.waiters[key], ch)
	q.mu.Unlock()

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ch:
		// Signalled by Enqueue; refresh poll time and grab a job.
		q.mu.Lock()
		q.lastPoll[key] = time.Now()
		if id, ok := q.popLocked(key); ok {
			job := q.jobs[id].job
			q.mu.Unlock()
			return job, true
		}
		q.mu.Unlock()
		return api.Job{}, false
	case <-timer.C:
		// Timed out; remove our waiter so it isn't signalled later.
		q.removeWaiter(key, ch)
		return api.Job{}, false
	}
}

func (q *Queue) removeWaiter(key string, ch chan struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	ws := q.waiters[key]
	for i, w := range ws {
		if w == ch {
			q.waiters[key] = append(ws[:i], ws[i+1:]...)
			return
		}
	}
}

// SetResult records a bridge's result for a job scoped to key. Returns false if
// the job is unknown or does not belong to that pairing key.
func (q *Queue) SetResult(key, id string, res api.ResultRequest) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.jobKey[id] != key {
		return false
	}
	e, ok := q.jobs[id]
	if !ok {
		return false
	}
	e.result = res
	if res.OK {
		e.status = api.StatusDone
	} else {
		e.status = api.StatusError
	}
	select {
	case <-e.done: // already closed
	default:
		close(e.done)
	}
	return true
}

// Fetch returns the current result for a job scoped to key. If the job is still
// pending and wait > 0, it blocks up to wait for a result to arrive.
func (q *Queue) Fetch(key, id string, wait time.Duration) (api.JobResult, bool) {
	q.mu.Lock()
	if q.jobKey[id] != key {
		q.mu.Unlock()
		return api.JobResult{}, false
	}
	e, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()
		return api.JobResult{}, false
	}
	done := e.done
	if e.status != api.StatusPending || wait <= 0 {
		res := q.snapshotLocked(id, e)
		q.mu.Unlock()
		return res, true
	}
	q.mu.Unlock()

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	e, ok = q.jobs[id]
	if !ok {
		return api.JobResult{}, false
	}
	return q.snapshotLocked(id, e), true
}

func (q *Queue) snapshotLocked(id string, e *jobEntry) api.JobResult {
	return api.JobResult{
		ID:     id,
		Status: e.status,
		JSON:   e.result.JSON,
		Text:   e.result.Text,
		Error:  e.result.Error,
	}
}

// BridgeOnline reports whether a bridge polled for key within the online window.
func (q *Queue) BridgeOnline(key string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	last, ok := q.lastPoll[key]
	return ok && time.Since(last) <= q.onlineWindow
}

// ReportCapabilities stores what a bridge says it can do for this pairing key.
// The relay cannot inspect the user's machine, so this is the only source of truth
// for which CLI backends are actually installed there.
func (q *Queue) ReportCapabilities(key string, caps api.BridgeCapabilities) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.caps[key] = capsEntry{caps: caps, reportedAt: time.Now()}
}

// Capabilities returns the last reported capabilities for key, its report time,
// and whether a bridge is currently online.
func (q *Queue) Capabilities(key string) (api.BridgeCapabilities, time.Time, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	e, ok := q.caps[key]
	last, pollOK := q.lastPoll[key]
	online := pollOK && time.Since(last) <= q.onlineWindow
	if !ok {
		return api.BridgeCapabilities{}, time.Time{}, online
	}
	return e.caps, e.reportedAt, online
}

// PendingCount returns how many jobs are queued (unclaimed) for key.
func (q *Queue) PendingCount(key string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, id := range q.pending[key] {
		if _, ok := q.jobs[id]; ok {
			n++
		}
	}
	return n
}

// janitor periodically evicts jobs older than the TTL so memory stays bounded.
func (q *Queue) janitor() {
	ticker := time.NewTicker(q.ttl / 2)
	defer ticker.Stop()
	for range ticker.C {
		q.mu.Lock()
		for id, e := range q.jobs {
			if time.Since(e.createdAt) > q.ttl {
				delete(q.jobs, id)
				delete(q.jobKey, id)
			}
		}
		q.mu.Unlock()
	}
}
