package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/mirror"
	"github.com/ms/amplifier-app-loom/internal/nl"
	"github.com/ms/amplifier-app-loom/internal/queue"
	"github.com/ms/amplifier-app-loom/internal/scheduler"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/workspaces"
)

// Server is the HTTP server for the web UI and REST API.
type Server struct {
	cfg         *config.Config
	store       store.Store
	scheduler   *scheduler.Scheduler
	runner      *scheduler.Runner
	broadcaster *scheduler.Broadcaster
	focusClients *focusRegistry
	queue       *queue.BoundedQueue
	startedAt   time.Time
	nlClient    nl.NLClient
	nlMu        sync.RWMutex
	httpSrv     *http.Server
	mirrorStore    *mirror.MirrorStore
	syncEngine     *mirror.SyncEngine
	workspaceStore  *workspaces.Service
	muxOnce        sync.Once
	mux            *http.ServeMux
}

func NewServer(cfg *config.Config, s store.Store, sched *scheduler.Scheduler, q *queue.BoundedQueue, startedAt time.Time, b *scheduler.Broadcaster) *Server {
	srv := &Server{
		cfg:          cfg,
		store:        s,
		scheduler:    sched,
		broadcaster:  b,
		focusClients: newFocusRegistry(),
		queue:        q,
		startedAt:    startedAt,
	}
	srv.nlClient = nl.NewClientFromConfig(cfg, s, sched)
	return srv
}

// SetRunner wires the Runner into the server so run cancellation is possible.
// Must be called before Start.
func (s *Server) SetRunner(r *scheduler.Runner) {
	s.runner = r
}

// SetMirror wires the mirror subsystem into the server. Called by the daemon
// after constructing the MirrorStore and SyncEngine. Safe to call before Start.
func (s *Server) SetMirror(ms *mirror.MirrorStore, se *mirror.SyncEngine) {
	s.mirrorStore = ms
	s.syncEngine = se
}

// SetWorkspaces wires the workspace subsystem (projects) into the server.
func (s *Server) SetWorkspaces(ws *workspaces.Service) {
	s.workspaceStore = ws
}

// ServeHTTP implements http.Handler for use in tests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.muxOnce.Do(func() {
		s.mux = http.NewServeMux()
		s.registerRoutes(s.mux)
	})
	s.mux.ServeHTTP(w, r)
}

func (s *Server) reinitNLClient() {
	client := nl.NewClientFromConfig(s.cfg, s.store, s.scheduler)
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

// WorkspaceStore exposes the workspace service for testing.
func (s *Server) WorkspaceStore() *workspaces.Service {
	return s.workspaceStore
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
	mux.HandleFunc("DELETE /api/runs", s.clearRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/runs/{id}/stream", s.streamRun)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.cancelRun)
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
	mux.HandleFunc("GET /api/chat/history", s.getChatHistory)
	mux.HandleFunc("DELETE /api/chat/history", s.clearChatHistory)

	// Mirror — connectors
	mux.HandleFunc("GET /api/mirror/connectors", s.listConnectors)
	mux.HandleFunc("POST /api/mirror/connectors", s.createConnector)
	mux.HandleFunc("GET /api/mirror/connectors/{id}", s.getConnector)
	mux.HandleFunc("PUT /api/mirror/connectors/{id}", s.updateConnector)
	mux.HandleFunc("DELETE /api/mirror/connectors/{id}", s.deleteConnector)

	// Mirror — entities
	mux.HandleFunc("GET /api/mirror/entities", s.listEntities)
	mux.HandleFunc("GET /api/mirror/entities/{address...}", s.getEntity)

	// Mirror — changes
	mux.HandleFunc("GET /api/mirror/changes", s.listChanges)
	mux.HandleFunc("POST /api/mirror/changes/prune", s.pruneChanges)

	// Projects
	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("GET /api/projects/{id}", s.getProject)
	mux.HandleFunc("PATCH /api/projects/{id}", s.updateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)
	mux.HandleFunc("GET /api/projects/{id}/settings", s.getProjectSettings)
	mux.HandleFunc("PUT /api/projects/{id}/settings", s.updateProjectSettings)

	// Files (project-scoped, no longer session-scoped)
	mux.HandleFunc("GET /api/projects/{id}/files", s.listFiles)
	mux.HandleFunc("GET /api/projects/{id}/files/{path...}", s.readFile)

	// Amplifier sessions + terminal launch
	mux.HandleFunc("GET /api/projects/{id}/amplifier-sessions", s.handleListAmplifierSessions)
	mux.HandleFunc("POST /api/projects/{id}/open-terminal", s.handleOpenTerminal)

	// Server-side directory browser (works for remote servers too)
	mux.HandleFunc("GET /api/filesystem/browse", s.browseDirs)
	mux.HandleFunc("GET /api/filesystem/find-dir", s.findDir)

	// Registry + app bundle management
	mux.HandleFunc("GET /api/registry", s.getRegistry)
	mux.HandleFunc("GET /api/bundles", s.listBundles)
	mux.HandleFunc("POST /api/bundles", s.addBundle)
	mux.HandleFunc("DELETE /api/bundles/{id}", s.removeBundle)
	mux.HandleFunc("POST /api/bundles/{id}/toggle", s.toggleBundle)

	// Feedback → files a GitHub issue via gh CLI
	mux.HandleFunc("POST /api/feedback", s.createFeedback)

	// Window focus — lets the tray bring an existing tab forward instead of opening a new one
	mux.HandleFunc("GET /api/ui/focus", s.focusStream)
	mux.HandleFunc("POST /api/ui/focus", s.focusTrigger)
}

// ── helpers ───────────────────────────────────────────────────────────────────────────────────────────────────────────────────

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
				slog.Error("handler panic", "recover", rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
