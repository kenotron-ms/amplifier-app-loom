package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ms/amplifier-app-loom/internal/scheduler"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/types"
)

// newTestStreamServer builds a minimal Server with a real bolt store and broadcaster.
func newTestStreamServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Server{
		store:       db,
		broadcaster: scheduler.NewBroadcaster(),
	}
}

// seedRun inserts a completed run into the store and returns it.
func seedRun(t *testing.T, st store.Store, status types.RunStatus, output string) *types.JobRun {
	t.Helper()
	now := time.Now()
	endedAt := now.Add(time.Second)
	run := &types.JobRun{
		ID:        fmt.Sprintf("run-%d", time.Now().UnixNano()),
		JobID:     "job-1",
		JobName:   "Test Job",
		StartedAt: now,
		EndedAt:   &endedAt,
		Status:    status,
		Output:    output,
	}
	if err := st.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return run
}

// TestStreamRun_CompletedRun — completed run emits one data chunk with stored output,
// then event:done with status; Content-Type is text/event-stream.
func TestStreamRun_CompletedRun(t *testing.T) {
	srv := newTestStreamServer(t)
	run := seedRun(t, srv.store, types.RunStatusSuccess, "hello output")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runs/{id}/stream", srv.streamRun)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID+"/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %q", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"chunk":"hello output"`) {
		t.Fatalf("expected chunk with output in body, got:\n%s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected 'event: done' in body, got:\n%s", body)
	}
	if !strings.Contains(body, `"status":"success"`) {
		t.Fatalf("expected status=success in done event, got:\n%s", body)
	}
}

// TestStreamRun_NotFound — unknown run ID returns 404.
func TestStreamRun_NotFound(t *testing.T) {
	srv := newTestStreamServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runs/{id}/stream", srv.streamRun)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/nonexistent-id/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestStreamRun_LiveThenDone — running run streams buffered chunks, receives live chunks,
// emits event:done when broadcaster.Complete is called.
func TestStreamRun_LiveThenDone(t *testing.T) {
	srv := newTestStreamServer(t)

	runID := fmt.Sprintf("live-run-%d", time.Now().UnixNano())
	now := time.Now()
	run := &types.JobRun{
		ID:        runID,
		JobID:     "job-live",
		JobName:   "Live Job",
		StartedAt: now,
		Status:    types.RunStatusRunning,
	}
	if err := srv.store.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}

	// Register and pre-buffer 2 chunks before handler subscribes.
	srv.broadcaster.Register(runID)
	srv.broadcaster.Write(runID, "buffered-chunk-1")
	srv.broadcaster.Write(runID, "buffered-chunk-2")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runs/{id}/stream", srv.streamRun)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+runID+"/stream", nil)
	w := httptest.NewRecorder()

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		mux.ServeHTTP(w, req)
	}()

	// Give the handler goroutine time to subscribe.
	time.Sleep(20 * time.Millisecond)

	// Inject a live chunk after subscribe.
	srv.broadcaster.Write(runID, "live-chunk-1")

	// Update run as completed in store before completing broadcaster.
	endedAt := time.Now()
	run.Status = types.RunStatusSuccess
	run.EndedAt = &endedAt
	if err := srv.store.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("update run in store: %v", err)
	}

	// Complete the broadcaster — closes all subscriber channels.
	srv.broadcaster.Complete(runID)

	// Wait for handler to exit.
	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit in time")
	}

	body := w.Body.String()

	if !strings.Contains(body, "buffered-chunk-1") {
		t.Fatalf("expected buffered-chunk-1 in body, got:\n%s", body)
	}
	if !strings.Contains(body, "buffered-chunk-2") {
		t.Fatalf("expected buffered-chunk-2 in body, got:\n%s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected 'event: done' in body, got:\n%s", body)
	}
	if !strings.Contains(body, `"status":"success"`) {
		t.Fatalf("expected status=success in done event, got:\n%s", body)
	}
}
