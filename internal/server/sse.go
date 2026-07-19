package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"hardware-usage/internal/model"
)

// SnapshotProvider supplies the latest Snapshot to broadcast.
type SnapshotProvider interface {
	Snapshot() model.Snapshot
}

// EmptyProvider is a stub SnapshotProvider that returns an empty Snapshot.
// It keeps the server runnable until the real assembler is wired.
type EmptyProvider struct{}

// Snapshot returns an empty Snapshot.
func (EmptyProvider) Snapshot() model.Snapshot { return model.Snapshot{} }

// SSEHub broadcasts JSON snapshots to connected clients over Server-Sent Events.
// The hub only calls its provider when at least one client is connected.
type SSEHub struct {
	provider SnapshotProvider
	interval time.Duration

	mu      sync.RWMutex
	clients map[chan string]struct{}
	last    string // most recent broadcast frame, replayed to each new client
}

// NewSSEHub creates a hub that will broadcast at interval.
func NewSSEHub(provider SnapshotProvider, interval time.Duration) *SSEHub {
	return &SSEHub{
		provider: provider,
		interval: interval,
		clients:  make(map[chan string]struct{}),
	}
}

// Run starts the broadcast loop. It blocks until the process exits.
func (h *SSEHub) Run() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for range ticker.C {
		h.Tick()
	}
}

// Tick broadcasts one snapshot to all current subscribers.
// It is safe to call from tests or manually.
func (h *SSEHub) Tick() {
	if !h.hasClients() {
		return
	}
	snap := h.provider.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	msg := string(data)
	h.mu.Lock()
	h.last = msg
	h.mu.Unlock()
	h.broadcast(msg)
}

func (h *SSEHub) hasClients() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients) > 0
}

func (h *SSEHub) register(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
}

func (h *SSEHub) unregister(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, ch)
}

func (h *SSEHub) broadcast(msg string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// Client is slow; drop the frame and keep serving the rest.
		}
	}
}

// ServeHTTP implements http.Handler for the /events endpoint.
func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	ch := make(chan string, 1)
	h.register(ch)
	defer h.unregister(ch)

	// Send an initial comment so the client knows the connection is live.
	_, _ = fmt.Fprint(w, ":ok\n\n")
	flusher.Flush()

	// Replay the most recent frame so the page paints immediately instead of
	// waiting up to one broadcast interval for the next tick. This reuses the
	// cached snapshot, so it never re-hits the collectors.
	h.mu.RLock()
	last := h.last
	h.mu.RUnlock()
	if last != "" {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", last)
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
