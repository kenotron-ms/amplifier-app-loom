package api

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
)

type feedbackRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type feedbackResponse struct {
	URL string `json:"url"`
}

// POST /api/feedback
// Files a GitHub issue on kenotron-ms/amplifier-app-loom via the gh CLI.
func (s *Server) createFeedback(w http.ResponseWriter, r *http.Request) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	args := []string{
		"issue", "create",
		"--repo", "kenotron-ms/amplifier-app-loom",
		"--title", req.Title,
		"--label", "user-feedback",
	}
	if req.Body != "" {
		args = append(args, "--body", req.Body)
	} else {
		args = append(args, "--body", "*(no description provided)*")
	}

	out, err := exec.CommandContext(r.Context(), "gh", args...).Output()
	if err != nil {
		// Surface the stderr if available for better diagnostics
		msg := "failed to create issue: gh CLI error"
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			msg = "gh: " + strings.TrimSpace(string(exitErr.Stderr))
		}
		writeError(w, http.StatusInternalServerError, msg)
		return
	}

	url := strings.TrimSpace(string(out))
	writeJSON(w, http.StatusCreated, feedbackResponse{URL: url})
}
