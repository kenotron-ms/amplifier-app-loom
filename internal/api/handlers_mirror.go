package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ms/amplifier-app-loom/internal/mirror"
)

// requireMirror is a guard — returns true if mirrorStore is available, else writes an error.
func (s *Server) requireMirror(w http.ResponseWriter) bool {
	if s.mirrorStore == nil {
		writeError(w, http.StatusServiceUnavailable, "mirror system not initialized")
		return false
	}
	return true
}

// ── Connectors ───────────────────────────────────────────────────────────────

func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}
	conns, err := s.mirrorStore.ListConnectors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conns == nil {
		conns = []*mirror.Connector{}
	}
	writeJSON(w, http.StatusOK, conns)
}

func (s *Server) getConnector(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}
	id := r.PathValue("id")
	conn, err := s.mirrorStore.GetConnector(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector not found")
		return
	}
	writeJSON(w, http.StatusOK, conn)
}

func (s *Server) deleteConnector(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.mirrorStore.DeleteConnector(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Stop the sync loop for this connector
	if s.syncEngine != nil {
		s.syncEngine.StopConnector(id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ── Entities ─────────────────────────────────────────────────────────────────

func (s *Server) listEntities(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}
	kind := r.URL.Query().Get("kind")
	entities, err := s.mirrorStore.ListEntities(r.Context(), kind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entities == nil {
		entities = []*mirror.Entity{}
	}
	writeJSON(w, http.StatusOK, entities)
}

func (s *Server) getEntity(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}
	// The address is everything after /api/mirror/entities/
	address := strings.TrimPrefix(r.URL.Path, "/api/mirror/entities/")
	if address == "" {
		writeError(w, http.StatusBadRequest, "entity address required")
		return
	}
	entity, err := s.mirrorStore.GetEntity(r.Context(), address)
	if err != nil {
		writeError(w, http.StatusNotFound, "entity not found")
		return
	}

	// Also fetch meta if available
	meta, _ := s.mirrorStore.GetEntityMeta(r.Context(), address)

	type entityWithMeta struct {
		Address string              `json:"address"`
		Data    json.RawMessage     `json:"data"`
		Meta    *mirror.EntityMeta  `json:"meta,omitempty"`
	}
	writeJSON(w, http.StatusOK, entityWithMeta{
		Address: entity.Address,
		Data:    entity.Data,
		Meta:    meta,
	})
}

// ── Changes ──────────────────────────────────────────────────────────────────

func (s *Server) listChanges(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}
	address := r.URL.Query().Get("address")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	changes, err := s.mirrorStore.ListChanges(r.Context(), address, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if changes == nil {
		changes = []*mirror.ChangeRecord{}
	}
	writeJSON(w, http.StatusOK, changes)
}

func (s *Server) pruneChanges(w http.ResponseWriter, r *http.Request) {
	if !s.requireMirror(w) {
		return
	}
	age := 7 * 24 * time.Hour
	pruned, err := mirror.PruneOldChanges(r.Context(), s.mirrorStore, age)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"pruned": pruned})
}