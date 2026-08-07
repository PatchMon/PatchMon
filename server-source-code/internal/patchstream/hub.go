// Package patchstream provides an in-process pub/sub hub that fan-outs
// patch-run progress events (started / chunk / done) to any number of
// frontend WebSocket subscribers for a given patch run.
//
// It is intentionally kept small and in-memory: patch-run progress is
// ephemeral, and durable state is already persisted to the database by
// the agent-facing /patching/runs/{id}/output endpoint. The hub is only
// responsible for delivering those updates to interested browsers in
// near real-time.
package patchstream

import (
	"sync"
)

// EventType enumerates the kinds of events the hub forwards.
type EventType string

const (
	EventStarted EventType = "started"
	EventChunk   EventType = "chunk"
	EventDone    EventType = "done"
)

// Event is a single message published to subscribers of a patch run.
type Event struct {
	Type         EventType `json:"type"`
	PatchRunID   string    `json:"patch_run_id"`
	Stage        string    `json:"stage,omitempty"`
	Chunk        string    `json:"chunk,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// subscriber wraps a single frontend WebSocket listener. Each subscriber
// gets its own buffered channel; a slow consumer is dropped rather than
// blocking publishers (the DB still has the authoritative output).
type subscriber struct {
	ch chan Event
}

// Hub fans out events to per-run subscriber sets.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*subscriber]struct{} // patch_run_id -> subscribers
}

// NewHub returns a new in-process hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[*subscriber]struct{})}
}

// Subscribe registers a subscriber for the given patch run and returns a
// read-only channel plus an unsubscribe function. The channel is buffered;
// if a subscriber falls behind, the hub drops further events for that
// subscriber rather than blocking publishers.
func (h *Hub) Subscribe(patchRunID string) (<-chan Event, func()) {
	s := &subscriber{ch: make(chan Event, 256)}
	h.mu.Lock()
	if _, ok := h.subs[patchRunID]; !ok {
		h.subs[patchRunID] = make(map[*subscriber]struct{})
	}
	h.subs[patchRunID][s] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if set, ok := h.subs[patchRunID]; ok {
			delete(set, s)
			if len(set) == 0 {
				delete(h.subs, patchRunID)
			}
		}
		h.mu.Unlock()
		close(s.ch)
	}
	return s.ch, unsubscribe
}

// Publish sends the event to every current subscriber of the patch run.
// Non-blocking: if a subscriber's buffer is full we drop the event for
// that subscriber rather than stall the publisher.
func (h *Hub) Publish(ev Event) {
	h.mu.RLock()
	set, ok := h.subs[ev.PatchRunID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	// Snapshot subscribers under the read lock so we don't hold it while sending.
	subs := make([]*subscriber, 0, len(set))
	for s := range set {
		subs = append(subs, s)
	}
	h.mu.RUnlock()

	for _, s := range subs {
		select {
		case s.ch <- ev:
		default:
		}
	}
}

// HasSubscribers reports whether any frontend is currently watching the run.
// Useful to avoid building payloads no one will read.
func (h *Hub) HasSubscribers(patchRunID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set, ok := h.subs[patchRunID]
	return ok && len(set) > 0
}
