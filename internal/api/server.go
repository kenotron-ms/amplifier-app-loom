package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/nl"
	"github.com/ms/agent-daemon/internal/queue"
	"github.com/ms/agent-daemon/internal/scheduler"
	"github.com/ms/agent-daemon/internal/store"
)

// Server is the HTTP server for the web UI and REST API.
type Server struct {
	cfg       *config.Config
	store     store.Store
	scheduler *scheduler.Scheduler
	queue     *queue.BoundedQueue
	startedAt time.Time
	nlClient  nl.NLClient
	nlMu      sync.RWMutex
	httpSrv   *http.Server
}

func NewServer(cfg *config.Config, s store.Store, sched *scheduler.Scheduler, q *queue.BoundedQueue, startedAt time.Time) *Server {
	srv := &Server{
		cfg:       cfg,
		store:     s,
		scheduler: sched,
		queue:     q,
		startedAt: startedAt,
	}
	srv.nlClient = nl.NewClientFromConfig(cfg, s)
	return srv
}

func (s *Server) reinitNLClient() {
	client := nl.NewClientFromConfig(s.cfg, s.store)
	s.nlMu.Lock()
	s.nlClient = client
	s.nlMu.Unlock()
}

func (s *Server) getNLClient() nl.NLClient {
	s.nlMu.RLock()
	defer s.nlMu.RUnlock()
	return s.nlClient
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(recoverMiddleware(mux)),
	}
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Stop() {
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(ctx)
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Static web UI
	mux.Handle("/", staticHandler())

	// Jobs
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("POST /api/jobs", s.createJob)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	mux.HandleFunc("PUT /api/jobs/{id}", s.updateJob)
	mux.HandleFunc("DELETE /api/jobs/{id}", s.deleteJob)
	mux.HandleFunc("POST /api/jobs/{id}/trigger", s.triggerJob)
	mux.HandleFunc("POST /api/jobs/{id}/enable", s.enableJob)
	mux.HandleFunc("POST /api/jobs/{id}/disable", s.disableJob)
	mux.HandleFunc("POST /api/jobs/prune", s.pruneJobs)

	// Runs
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/jobs/{id}/runs", s.listJobRuns)

	// Daemon control
	mux.HandleFunc("GET /api/status", s.getStatus)
	mux.HandleFunc("POST /api/daemon/pause", s.pauseDaemon)
	mux.HandleFunc("POST /api/daemon/resume", s.resumeDaemon)
	mux.HandleFunc("POST /api/daemon/flush", s.flushQueue)

	// Settings
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.updateSettings)
	mux.HandleFunc("POST /api/settings/test", s.testSettings)

	// Natural language chat
	mux.HandleFunc("POST /api/chat", s.chat)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
