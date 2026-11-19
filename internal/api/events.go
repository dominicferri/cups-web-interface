package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEEvent represents a real-time event sent over Server-Sent Events.
type SSEEvent struct {
	Type   string      `json:"type"`             // "job_update", "printer_update", "ping"
	Action string      `json:"action,omitempty"` // "cancel", "hold", "release", "restart", "pause", "resume", "purge", "default"
	Target string      `json:"target,omitempty"` // printer name or job ID
	Data   interface{} `json:"data,omitempty"`
}

// EventBroadcaster manages active SSE subscriptions and broadcasts updates.
type EventBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[chan SSEEvent]struct{}
}

// NewEventBroadcaster creates an initialized EventBroadcaster.
func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		subscribers: make(map[chan SSEEvent]struct{}),
	}
}

// Subscribe registers a new subscriber and returns a channel and an unsubscribe closure.
func (b *EventBroadcaster) Subscribe() (chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 16)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
	}

	return ch, unsubscribe
}

// Broadcast dispatches an event to all active subscribers. Non-blocking if a channel is full.
func (b *EventBroadcaster) Broadcast(event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// If buffer is full, drop to keep server responsive
		}
	}
}

// handleEvents streams real-time updates via Server-Sent Events (SSE).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported by server")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")

	// Send initial connection comment
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ch, unsubscribe := s.events.Subscribe()
	defer unsubscribe()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			pingEvent, _ := json.Marshal(SSEEvent{Type: "ping"})
			_, err := fmt.Fprintf(w, "data: %s\n\n", pingEvent)
			if err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, err = fmt.Fprintf(w, "data: %s\n\n", data)
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
