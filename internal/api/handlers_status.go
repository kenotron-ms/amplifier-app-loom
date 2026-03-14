package api

import (
	"net/http"
	"os"

	"github.com/ms/agent-daemon/internal/types"
)

const Version = "0.1.0"

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	jobs, _ := s.store.ListJobs(r.Context())

	state := "running"
	if s.scheduler.IsPaused() {
		state = "paused"
	}

	status := types.DaemonStatus{
		State:      state,
		PID:        os.Getpid(),
		StartedAt:  s.startedAt,
		ActiveRuns: s.queue.ActiveCount(),
		QueueDepth: s.queue.PendingCount(),
		JobCount:   len(jobs),
		Version:    Version,
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) pauseDaemon(w http.ResponseWriter, r *http.Request) {
	s.scheduler.Pause()

	// Persist pause state
	cfg, err := s.store.GetConfig(r.Context())
	if err == nil {
		cfg.Paused = true
		_ = s.store.SaveConfig(r.Context(), cfg)
	}

	writeJSON(w, http.StatusOK, map[string]string{"state": "paused"})
}

func (s *Server) resumeDaemon(w http.ResponseWriter, r *http.Request) {
	s.scheduler.Resume()

	cfg, err := s.store.GetConfig(r.Context())
	if err == nil {
		cfg.Paused = false
		_ = s.store.SaveConfig(r.Context(), cfg)
	}

	writeJSON(w, http.StatusOK, map[string]string{"state": "running"})
}

func (s *Server) flushQueue(w http.ResponseWriter, r *http.Request) {
	s.queue.Flush()
	writeJSON(w, http.StatusOK, map[string]string{"status": "flushed"})
}
