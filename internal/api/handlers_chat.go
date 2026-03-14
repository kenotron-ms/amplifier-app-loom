package api

import (
	"encoding/json"
	"net/http"
)

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Text    string   `json:"text"`
	Actions []string `json:"actions,omitempty"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	if s.nlClient == nil {
		writeError(w, http.StatusServiceUnavailable, "NL interface unavailable: set ANTHROPIC_API_KEY or configure it in the daemon config")
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

	text, actions, err := s.nlClient.Chat(r.Context(), req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If jobs were mutated, reload the scheduler
	if len(actions) > 0 {
		_ = s.scheduler.Reload()
	}

	writeJSON(w, http.StatusOK, chatResponse{Text: text, Actions: actions})
}
