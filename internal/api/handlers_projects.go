package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"

	"github.com/ms/amplifier-app-loom/internal/files"
)

// ── Projects ──────────────────────────────────────────────────────────────────

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.workspaceStore.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, "name and path are required")
		return
	}
	p, err := s.workspaceStore.CreateProject(r.Context(), req.Name, req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.workspaceStore.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	p, err := s.workspaceStore.UpdateProject(r.Context(), id, req.Name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sessions, _ := s.workspaceStore.ListSessions(r.Context(), id)
	for _, sess := range sessions {
		if sess.ProcessID != nil {
			s.ptyMgr.Kill(*sess.ProcessID) //nolint:errcheck
		}
	}
	if err := s.workspaceStore.DeleteProject(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Sessions ──────────────────────────────────────────────────────────────────

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sessions, err := s.workspaceStore.ListSessions(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var req struct {
		Name         string `json:"name"`
		WorktreePath string `json:"worktreePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.WorktreePath == "" {
		writeError(w, http.StatusBadRequest, "worktreePath is required")
		return
	}
	// create git worktree if directory doesn't exist
	if _, err := os.Stat(req.WorktreePath); os.IsNotExist(err) {
		p, err := s.workspaceStore.GetProject(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		out, err := exec.CommandContext(r.Context(), "git", "-C", p.Path, "worktree", "add", req.WorktreePath, req.Name).CombinedOutput()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "git worktree add: "+string(out))
			return
		}
	}
	sess, err := s.workspaceStore.CreateSession(r.Context(), projectID, req.Name, req.WorktreePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	sess, err := s.workspaceStore.GetSession(r.Context(), sid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if sess.ProcessID != nil {
		s.ptyMgr.Kill(*sess.ProcessID) //nolint:errcheck
	}
	if err := s.workspaceStore.DeleteSession(r.Context(), sid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Terminal ──────────────────────────────────────────────────────────────────

func (s *Server) spawnTerminal(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	sess, err := s.workspaceStore.GetSession(r.Context(), sid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	key := sess.ProjectID + "::" + sess.WorktreePath
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	processID, err := s.ptyMgr.Spawn(key, sess.WorktreePath, []string{shell})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.workspaceStore.UpdateSessionStatus(r.Context(), sid, "active", &processID) //nolint:errcheck
	writeJSON(w, http.StatusOK, map[string]string{"processId": processID})
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	processID := r.PathValue("processId")
	s.ptyMgr.ServeWS(w, r, processID)
}

// ── Files ─────────────────────────────────────────────────────────────────────

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	sess, err := s.workspaceStore.GetSession(r.Context(), sid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	rel := r.URL.Query().Get("path")
	entries, err := files.New(sess.WorktreePath).List(rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	path := r.PathValue("path")
	sess, err := s.workspaceStore.GetSession(r.Context(), sid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	data, err := files.New(sess.WorktreePath).Read(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data) //nolint:errcheck
}

// ── Stats ─────────────────────────────────────────────────────────────────────

func (s *Server) getSessionStats(w http.ResponseWriter, r *http.Request) {
	// Placeholder: full implementation reads ~/.amplifier/.../events.jsonl
	writeJSON(w, http.StatusOK, map[string]any{
		"tokens": 0,
		"tools":  0,
	})
}
