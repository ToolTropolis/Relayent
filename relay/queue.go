// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: In-memory, per-user job broker for the Relayent relay. Every map
//
//	  is keyed by a stable userID (supplied by the auth layer's Principal), so a
//	  job routes only to the bridge bound to that same user. In legacy single-key
//	  mode the userID is the constant "legacy", preserving the original
//	  single-namespace behaviour exactly.
//
//		Supports enqueue, long-poll claim by a bridge, result posting, result
//		fetching, and bridge-presence tracking (drives fail-fast). TTL-expires
//		orphaned jobs. Redis is the intended drop-in for multi-instance later.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"sync"
	"time"

	"github.com/ToolTropolis/Relayent/internal/api"
)

// jobEntry is the relay's internal record for one job.
type jobEntry struct {
	job       api.Job
	status    string
	result    api.ResultRequest
	createdAt time.Time
	done      chan struct{} // closed when a result arrives; lets fetchers block
}

// Queue is a concurrency-safe, per-userID job broker.
type Queue struct {
	mu sync.Mutex

	// pending[userID] is a FIFO of job IDs awaiting that user's bridge.
	pending map[string][]string
	// jobs is the global id -> entry map (ids are globally unique).
	jobs map[string]*jobEntry
	// jobUser maps a job id back to its user (for scoped result posting).
	jobUser map[string]string
	// waiters[userID] are bridges blocked in ClaimNext for that userID; signalled on enqueue.
	waiters map[string][]chan struct{}
	// lastPoll[userID] is the last time any bridge polled ClaimNext for that userID.
	lastPoll map[string]time.Time
	// caps[userID] is the most recent capabilities a bridge reported for that userID.
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
		jobUser:      make(map[string]string),
		waiters:      make(map[string][]chan struct{}),
		lastPoll:     make(map[string]time.Time),
		caps:         make(map[string]capsEntry),
		ttl:          ttl,
		onlineWindow: onlineWindow,
	}
	go q.janitor()
	return q
}

// Enqueue records a new job for a user and wakes one of that user's waiting bridges.
func (q *Queue) Enqueue(userID, id string, job api.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.jobs[id] = &jobEntry{
		job:       job,
		status:    api.StatusPending,
		createdAt: time.Now(),
		done:      make(chan struct{}),
	}
	q.jobUser[id] = userID
	q.pending[userID] = append(q.pending[userID], id)

	// Wake exactly one blocked bridge, if any.
	if ws := q.waiters[userID]; len(ws) > 0 {
		w := ws[0]
		q.waiters[userID] = ws[1:]
		close(w)
	}
}

// popLocked removes and returns the next pending job id for userID (caller holds lock).
func (q *Queue) popLocked(userID string) (string, bool) {
	ids := q.pending[userID]
	for len(ids) > 0 {
		id := ids[0]
		ids = ids[1:]
		q.pending[userID] = ids
		// Skip ids that were expired/removed while queued.
		if _, ok := q.jobs[id]; ok {
			return id, true
		}
	}
	return "", false
}

// ClaimNext returns the next job for userID, blocking up to wait for one to arrive.
// It also records the poll time so BridgeOnline can report presence.
func (q *Queue) ClaimNext(userID string, wait time.Duration) (api.Job, bool) {
	q.mu.Lock()
	q.lastPoll[userID] = time.Now()
	if id, ok := q.popLocked(userID); ok {
		job := q.jobs[id].job
		q.mu.Unlock()
		return job, true
	}

	// Nothing queued: register a waiter and block.
	ch := make(chan struct{})
	q.waiters[userID] = append(q.waiters[userID], ch)
	q.mu.Unlock()

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ch:
		// Signalled by Enqueue; refresh poll time and grab a job.
		q.mu.Lock()
		q.lastPoll[userID] = time.Now()
		if id, ok := q.popLocked(userID); ok {
			job := q.jobs[id].job
			q.mu.Unlock()
			return job, true
		}
		q.mu.Unlock()
		return api.Job{}, false
	case <-timer.C:
		// Timed out; remove our waiter so it isn't signalled later.
		q.removeWaiter(userID, ch)
		return api.Job{}, false
	}
}

func (q *Queue) removeWaiter(userID string, ch chan struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	ws := q.waiters[userID]
	for i, w := range ws {
		if w == ch {
			q.waiters[userID] = append(ws[:i], ws[i+1:]...)
			return
		}
	}
}

// SetResult records a bridge's result for a job scoped to userID. Returns false if
// the job is unknown or does not belong to that user.
func (q *Queue) SetResult(userID, id string, res api.ResultRequest) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.jobUser[id] != userID {
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

// Cancel abandons a job scoped to userID. It returns the job's state at the moment
// of cancellation so the caller can tell what actually happened:
//   - "pending"   — it was still queued and has been removed; no bridge will run it
//   - "running"   — a bridge already claimed it; see the caveat below
//   - "done"/"error" — it had already finished; nothing to cancel
//
// ⚠️ Cancelling a claimed job does NOT stop the CLI already running on the
// bridge. The bridge owns that process, and the relay has no channel to reach it
// — the connection is outbound and one-way by design. What cancel does is stop
// the caller waiting and mark the job so a late result is discarded. The work
// (and the subscription quota) is already spent. This is a deliberate limit of
// the dial-out architecture, not an oversight.
func (q *Queue) Cancel(userID, id string) (state string, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.jobUser[id] != userID {
		return "", false // unknown job, or another tenant's — same answer either way
	}
	e, exists := q.jobs[id]
	if !exists {
		return "", false
	}
	if e.status == api.StatusDone || e.status == api.StatusError {
		return e.status, true // already finished; nothing to do
	}

	// Was it still queued? Removing it from pending is what actually prevents work.
	prev := "running"
	if q.removeFromPendingLocked(userID, id) {
		prev = "pending"
	}

	e.status = api.StatusError
	e.result = api.ResultRequest{OK: false, Error: "job cancelled by the caller"}
	select {
	case <-e.done:
	default:
		close(e.done) // unblock anyone in Fetch
	}
	return prev, true
}

// removeFromPendingLocked drops id from userID's queue, reporting whether it was
// there. Caller must hold q.mu.
func (q *Queue) removeFromPendingLocked(userID, id string) bool {
	ids := q.pending[userID]
	for i, v := range ids {
		if v == id {
			q.pending[userID] = append(ids[:i:i], ids[i+1:]...)
			return true
		}
	}
	return false
}

// Fetch returns the current result for a job scoped to userID. If the job is still
// pending and wait > 0, it blocks up to wait for a result to arrive.
func (q *Queue) Fetch(userID, id string, wait time.Duration) (api.JobResult, bool) {
	q.mu.Lock()
	if q.jobUser[id] != userID {
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

// BridgeOnline reports whether a bridge polled for userID within the online window.
func (q *Queue) BridgeOnline(userID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	last, ok := q.lastPoll[userID]
	return ok && time.Since(last) <= q.onlineWindow
}

// ReportCapabilities stores what a bridge says it can do for this pairing key.
// The relay cannot inspect the user's machine, so this is the only source of truth
// for which CLI backends are actually installed there.
func (q *Queue) ReportCapabilities(userID string, caps api.BridgeCapabilities) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.caps[userID] = capsEntry{caps: caps, reportedAt: time.Now()}
}

// Capabilities returns the last reported capabilities for userID, its report time,
// and whether a bridge is currently online.
func (q *Queue) Capabilities(userID string) (api.BridgeCapabilities, time.Time, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	e, ok := q.caps[userID]
	last, pollOK := q.lastPoll[userID]
	online := pollOK && time.Since(last) <= q.onlineWindow
	if !ok {
		return api.BridgeCapabilities{}, time.Time{}, online
	}
	return e.caps, e.reportedAt, online
}

// PendingCount returns how many jobs are queued (unclaimed) for userID.
func (q *Queue) PendingCount(userID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, id := range q.pending[userID] {
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
				delete(q.jobUser, id)
			}
		}
		q.mu.Unlock()
	}
}
