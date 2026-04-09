package api

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// focusRegistry tracks all SSE clients that are waiting for window-focus signals.
// The tray calls POST /api/ui/focus; each connected frontend tab receives the event
// and calls window.focus(), bringing the existing tab forward without opening a new one.
type focusRegistry struct {
	mu      sync.Mutex
	nextID  atomic.Int64
	clients map[int64]chan struct{}
}

func newFocusRegistry() *focusRegistry {
	return &focusRegistry{clients: make(map[int64]chan struct{})}
}

func (r *focusRegistry) add() (int64, chan struct{}) {
	ch := make(chan struct{}, 1)
	id := r.nextID.Add(1)
	r.mu.Lock()
	r.clients[id] = ch
	r.mu.Unlock()
	return id, ch
}

func (r *focusRegistry) remove(id int64) {
	r.mu.Lock()
	delete(r.clients, id)
	r.mu.Unlock()
}

// broadcast sends a signal to every connected tab and returns how many were reached.
func (r *focusRegistry) broadcast() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ch := range r.clients {
		select {
		case ch <- struct{}{}:
		default: // already has a pending signal, skip
		}
	}
	return len(r.clients)
}

// focusStream is the long-lived SSE endpoint the frontend subscribes to on mount.
// GET /api/ui/focus
func (s *Server) focusStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	id, ch := s.focusClients.add()
	defer s.focusClients.remove(id)

	// Establish the stream with a comment so the browser EventSource considers it open.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-ch:
			fmt.Fprintf(w, "event: focus\ndata: {}\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// focusTrigger is called by the tray to bring an existing dashboard tab forward.
// Returns {"clients": N} so the tray knows whether to also open a new browser window.
// POST /api/ui/focus
func (s *Server) focusTrigger(w http.ResponseWriter, r *http.Request) {
	n := s.focusClients.broadcast()
	writeJSON(w, http.StatusOK, map[string]int{"clients": n})
}
