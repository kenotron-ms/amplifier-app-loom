package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ms/agent-daemon/internal/types"
)

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*types.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var job types.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if job.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if job.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	job.ID = uuid.New().String()
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	if !job.Enabled {
		job.Enabled = true // default to enabled
	}

	if err := s.store.SaveJob(r.Context(), &job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.scheduler.AddJob(&job)
	writeJSON(w, http.StatusCreated, &job)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) updateJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	var updates types.Job
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Merge updates into existing
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if updates.Command != "" {
		existing.Command = updates.Command
	}
	if updates.CWD != "" {
		existing.CWD = updates.CWD
	}
	if updates.Trigger.Type != "" {
		existing.Trigger = updates.Trigger
	}
	if updates.Timeout != "" {
		existing.Timeout = updates.Timeout
	}
	existing.MaxRetries = updates.MaxRetries
	existing.Enabled = updates.Enabled
	existing.UpdatedAt = time.Now()

	if err := s.store.SaveJob(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Re-register with scheduler
	s.scheduler.RemoveJob(id)
	s.scheduler.AddJob(existing)

	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetJob(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	s.scheduler.RemoveJob(id)
	if err := s.store.DeleteJob(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) triggerJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.scheduler.TriggerNow(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "triggered"})
}

func (s *Server) enableJob(w http.ResponseWriter, r *http.Request) {
	s.setJobEnabled(w, r, true)
}

func (s *Server) disableJob(w http.ResponseWriter, r *http.Request) {
	s.setJobEnabled(w, r, false)
}

func (s *Server) setJobEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	id := r.PathValue("id")
	job, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	job.Enabled = enabled
	job.UpdatedAt = time.Now()
	if err := s.store.SaveJob(r.Context(), job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.scheduler.RemoveJob(id)
	if enabled {
		s.scheduler.AddJob(job)
	}
	writeJSON(w, http.StatusOK, job)
}
