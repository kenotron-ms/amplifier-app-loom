package api

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/ms/amplifier-app-loom/internal/index"
	"github.com/ms/amplifier-app-loom/internal/types"
)

// ── Scan state ───────────────────────────────────────────────────────────────

var (
	indexScanMu      sync.Mutex
	indexScanRunning bool
)

// ── Index status ─────────────────────────────────────────────────────────────

type indexStatusResponse struct {
	LastScan   string `json:"lastScan"`
	RepoCount  int    `json:"repoCount"`
	Scanning   bool   `json:"scanning"`
	WatchJobID string `json:"watchJobId,omitempty"`
}

// GET /api/index/status
func (s *Server) getIndexStatus(w http.ResponseWriter, r *http.Request) {
	dir := index.DefaultDir()
	idx, _ := index.LoadIndex(dir)

	jobs, _ := s.store.ListJobs(r.Context())
	watchJobID := ""
	for _, j := range jobs {
		if j.Name == "amplifier-bundle-index" {
			watchJobID = j.ID
			break
		}
	}

	indexScanMu.Lock()
	scanning := indexScanRunning
	indexScanMu.Unlock()

	resp := indexStatusResponse{
		LastScan:   idx.LastScan,
		RepoCount:  len(idx.Repos),
		Scanning:   scanning,
		WatchJobID: watchJobID,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── Trigger scan ─────────────────────────────────────────────────────────────

// POST /api/index/scan
func (s *Server) triggerIndexScan(w http.ResponseWriter, r *http.Request) {
	indexScanMu.Lock()
	if indexScanRunning {
		indexScanMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{
			"scanning": true,
			"message":  "scan already running",
		})
		return
	}
	indexScanRunning = true
	indexScanMu.Unlock()

	go func() {
		defer func() {
			indexScanMu.Lock()
			indexScanRunning = false
			indexScanMu.Unlock()
		}()
		dir := index.DefaultDir()
		index.Scan(context.Background(), dir, index.ScanOptions{Quiet: true}) //nolint:errcheck
	}()

	writeJSON(w, http.StatusAccepted, map[string]bool{"scanning": true})
}

// ── Watch job ─────────────────────────────────────────────────────────────────

// POST /api/index/watch?every=2h
func (s *Server) addIndexWatch(w http.ResponseWriter, r *http.Request) {
	every := r.URL.Query().Get("every")
	if every == "" {
		every = "2h"
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = "loom"
	}

	job := &types.Job{
		ID:   uuid.New().String(),
		Name: "amplifier-bundle-index",
		Trigger: types.Trigger{
			Type:     types.TriggerLoop,
			Schedule: every,
		},
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: execPath + " index scan --quiet",
		},
		Enabled: true,
	}

	if err := s.store.SaveJob(r.Context(), job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// DELETE /api/index/watch
func (s *Server) removeIndexWatch(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var jobID string
	for _, j := range jobs {
		if j.Name == "amplifier-bundle-index" {
			jobID = j.ID
			break
		}
	}

	if jobID == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"removed": false})
		return
	}

	if err := s.store.DeleteJob(r.Context(), jobID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}
