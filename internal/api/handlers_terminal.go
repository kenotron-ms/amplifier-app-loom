package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/ms/amplifier-app-loom/internal/amplifier"
)

// handleListAmplifierSessions returns Amplifier sessions for a project.
func (s *Server) handleListAmplifierSessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.workspaceStore.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	sessions, err := amplifier.ListProjectSessions(p.Path)
	if err != nil {
		// Session store may be missing — return empty list, not an error.
		writeJSON(w, http.StatusOK, []amplifier.AmplifierSession{})
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// handleOpenTerminal launches or focuses a native terminal for a project.
func (s *Server) handleOpenTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.workspaceStore.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		Mode      string `json:"mode"`      // "new" | "resume"
		SessionID string `json:"sessionId"` // required when mode=resume
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	terminal := s.cfg.PreferredTerminal
	if terminal == "" {
		terminal = "Ghostty"
	}
	terminal = strings.TrimSuffix(terminal, ".app")

	switch req.Mode {
	case "new":
		ampBin := resolveAmplifier()
		var cmd *exec.Cmd
		switch terminal {
		case "Ghostty":
			// On macOS, Ghostty must be launched via `open -na Ghostty.app` to get its
			// full app-bundle identity. Directly exec-ing the binary (e.g. via exec.LookPath)
			// causes macOS TCC to show a permission prompt on every launch because the
			// calling process has no stable app identity to anchor the permission to.
			cmd = exec.Command("open", "-na", "Ghostty.app",
				"--args",
				"--working-directory="+p.Path,
				"-e", ampBin, "run")
		default:
			// Terminal.app, iTerm2, Warp — AppleScript do script
			script := fmt.Sprintf(
				`tell application "%s"
	activate
	do script "cd '%s' && '%s' run"
end tell`, terminal, p.Path, ampBin)
			cmd = exec.Command("osascript", "-e", script)
		}
		if err := cmd.Run(); err != nil {
			writeError(w, http.StatusInternalServerError,
				fmt.Sprintf("failed to open %s: %s", terminal, err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "opened"})

	case "resume":
		if req.SessionID == "" {
			writeError(w, http.StatusBadRequest, "sessionId required for resume mode")
			return
		}
		// Check if a process with this session ID is already running.
		check := exec.Command("bash", "-c",
			fmt.Sprintf("ps aux | grep '%s' | grep -v grep", req.SessionID))
		if check.Run() == nil {
			// Process found — focus the terminal window via AppleScript.
			script := fmt.Sprintf(`tell application "%s" to activate`, terminal)
			exec.Command("osascript", "-e", script).Run() //nolint:errcheck
			writeJSON(w, http.StatusOK, map[string]string{"status": "focused"})
			return
		}
		// Not running — open a new terminal with amplifier --resume.
		ampBin := resolveAmplifier()
		script := fmt.Sprintf(
			`tell application "%s"
	activate
	do script "cd '%s' && '%s' run --resume '%s'"
end tell`, terminal, p.Path, ampBin, req.SessionID)
		if err := exec.Command("osascript", "-e", script).Run(); err != nil {
			writeError(w, http.StatusInternalServerError,
				fmt.Sprintf("failed to resume session: %s", err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})

	default:
		writeError(w, http.StatusBadRequest, `mode must be "new" or "resume"`)
	}
}
