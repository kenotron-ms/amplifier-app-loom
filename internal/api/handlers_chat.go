package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ms/agent-daemon/internal/types"
)

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Text    string   `json:"text"`
	Actions []string `json:"actions,omitempty"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	client := s.getNLClient()
	if client == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error":            "no_api_key",
			"message":          "AI assistant not configured. Add your API key in Settings.",
			"settingsRequired": true,
		})
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	history, _ := s.store.ListChatHistory(r.Context())

	text, actions, err := client.Chat(r.Context(), req.Message, history)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Persist both turns.
	now := time.Now()
	_ = s.store.AppendChatMessage(r.Context(), types.ChatMessage{
		ID: fmt.Sprintf("%d-u", now.UnixNano()), Role: "user", Content: req.Message, CreatedAt: now,
	})
	_ = s.store.AppendChatMessage(r.Context(), types.ChatMessage{
		ID: fmt.Sprintf("%d-a", now.UnixNano()), Role: "assistant", Content: text, CreatedAt: now.Add(time.Millisecond),
	})

	// If jobs were mutated, reload the scheduler.
	if len(actions) > 0 {
		_ = s.scheduler.Reload()
	}

	writeJSON(w, http.StatusOK, chatResponse{Text: text, Actions: actions})
}

func (s *Server) getChatHistory(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.store.ListChatHistory(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msgs == nil {
		msgs = []types.ChatMessage{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) clearChatHistory(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ClearChatHistory(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clearRuns(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteAllRuns(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
