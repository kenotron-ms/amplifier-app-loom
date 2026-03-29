package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ms/amplifier-app-loom/internal/mirror"
)

func (s *Server) createConnector(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}

	var conn mirror.Connector
	if err := json.NewDecoder(r.Body).Decode(&conn); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if conn.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if conn.EntityAddress == "" {
		writeError(w, http.StatusBadRequest, "entityAddress is required")
		return
	}
	if conn.FetchMethod == "" {
		writeError(w, http.StatusBadRequest, "fetchMethod is required (command, http, or browser)")
		return
	}
	if conn.Interval == "" {
		conn.Interval = "5m" // default
	}

	conn.ID = uuid.New().String()
	conn.CreatedAt = time.Now()
	conn.UpdatedAt = time.Now()
	conn.Health = mirror.HealthHealthy
	if !conn.Enabled {
		conn.Enabled = true // default
	}

	if err := s.mirrorStore.SaveConnector(r.Context(), &conn); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Start the sync loop for this connector
	if s.syncEngine != nil && conn.Enabled {
		s.syncEngine.StartConnector(&conn)
	}

	writeJSON(w, http.StatusCreated, &conn)
}

func (s *Server) updateConnector(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}

	id := r.PathValue("id")
	existing, err := s.mirrorStore.GetConnector(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector not found")
		return
	}

	var updates mirror.Connector
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Merge updates
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if updates.Prompt != "" {
		existing.Prompt = updates.Prompt
	}
	if updates.URL != "" {
		existing.URL = updates.URL
	}
	if updates.FetchMethod != "" {
		existing.FetchMethod = updates.FetchMethod
	}
	if updates.Command != "" {
		existing.Command = updates.Command
	}
	if updates.Interval != "" {
		existing.Interval = updates.Interval
	}
	if updates.EntityAddress != "" {
		existing.EntityAddress = updates.EntityAddress
	}
	existing.Enabled = updates.Enabled
	existing.UpdatedAt = time.Now()

	if err := s.mirrorStore.SaveConnector(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Restart the sync loop with updated config
	if s.syncEngine != nil {
		s.syncEngine.StopConnector(id)
		if existing.Enabled {
			s.syncEngine.StartConnector(existing)
		}
	}

	writeJSON(w, http.StatusOK, existing)
}