package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/scheduler"
)

// TestNewServer_BroadcasterIsLastParam verifies that NewServer accepts
// *scheduler.Broadcaster as its final parameter and stores it on the Server.
func TestNewServer_BroadcasterIsLastParam(t *testing.T) {
	cfg := &config.Config{}
	b := scheduler.NewBroadcaster()

	// Signature must be: NewServer(cfg, s, sched, q, startedAt, b)
	// broadcaster is the 6th (final) parameter.
	srv := NewServer(cfg, nil, nil, nil, time.Time{}, b)

	if srv.broadcaster != b {
		t.Fatal("expected broadcaster to be set on server, but it was not")
	}
}

// TestRegisterRoutes_StreamRoute verifies that the SSE stream route is registered.
func TestRegisterRoutes_StreamRoute(t *testing.T) {
	cfg := &config.Config{}
	b := scheduler.NewBroadcaster()
	srv := NewServer(cfg, nil, nil, nil, time.Time{}, b)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	// Wrap with recoverMiddleware so handler panics (from nil store) are caught
	// and turned into 500s rather than crashing the test.
	handler := recoverMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/some-id/stream", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// A 404 with the exact mux default body means the route is NOT registered.
	// Any other status (including 500 from a recovered panic) confirms the route exists.
	if w.Code == http.StatusNotFound && w.Body.String() == "404 page not found\n" {
		t.Fatal("stream route GET /api/runs/{id}/stream is not registered in registerRoutes")
	}
}
