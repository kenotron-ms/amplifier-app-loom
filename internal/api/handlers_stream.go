package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ms/agent-daemon/internal/types"
)

// sseRunDone is the payload for the SSE "done" event.
type sseRunDone struct {
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}

// streamRun serves buffered + live run output over Server-Sent Events.
// GET /api/runs/{id}/stream
func (s *Server) streamRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// Completed run: emit stored output as single chunk, then done.
	if run.Status != types.RunStatusRunning {
		emitChunk(w, run.Output)
		emitDone(w, run)
		flusher.Flush()
		return
	}

	// Running run: subscribe to live output.
	buffered, ch, done := s.broadcaster.Subscribe(id)
	if done {
		// Race condition: run just completed between GetRun and Subscribe.
		run, err = s.store.GetRun(r.Context(), id)
		if err == nil {
			emitChunk(w, run.Output)
			emitDone(w, run)
		}
		flusher.Flush()
		return
	}

	// Replay buffered chunks, then flush before entering live loop.
	for _, chunk := range buffered {
		emitChunk(w, chunk)
	}
	flusher.Flush()

	// Stream live chunks until channel is closed or client disconnects.
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				// Channel closed: run completed. Reload for final status.
				run, _ = s.store.GetRun(r.Context(), id)
				emitDone(w, run)
				flusher.Flush()
				return
			}
			emitChunk(w, chunk)
			flusher.Flush()
		case <-r.Context().Done():
			s.broadcaster.Unsubscribe(id, ch)
			return
		}
	}
}

// emitChunk writes a SSE data event with the chunk payload.
func emitChunk(w http.ResponseWriter, chunk string) {
	data, _ := json.Marshal(map[string]string{"chunk": chunk})
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// emitDone writes a SSE "done" event with run status and timestamps.
func emitDone(w http.ResponseWriter, run *types.JobRun) {
	if run == nil {
		fmt.Fprintf(w, "event: done\ndata: {\"status\":\"unknown\"}\n\n")
		return
	}
	evt := sseRunDone{
		Status: string(run.Status),
	}
	if !run.StartedAt.IsZero() {
		evt.StartedAt = run.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if run.EndedAt != nil {
		evt.EndedAt = run.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	data, _ := json.Marshal(evt)
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
}
