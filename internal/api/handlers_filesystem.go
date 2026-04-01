package api

import (
	"net/http"

	"github.com/ncruces/zenity"
)

// pickFolder opens the native OS directory picker dialog and returns the
// selected path. Works on macOS, Windows, and Linux.
//
// GET /api/filesystem/pick-folder          — open the dialog
// GET /api/filesystem/pick-folder?check=1  — probe support without opening dialog
//
// Uses github.com/ncruces/zenity (no CGO required):
//   macOS   — osascript (NSOpenPanel via AppleScript)
//   Windows — Win32 APIs via syscall
//   Linux   — zenity / matedialog / qarma CLI
func (s *Server) pickFolder(w http.ResponseWriter, r *http.Request) {
	// Capability probe — just report availability without opening anything.
	if r.URL.Query().Get("check") != "" {
		writeJSON(w, http.StatusOK, map[string]any{"supported": true})
		return
	}

	prompt := r.URL.Query().Get("prompt")
	if prompt == "" {
		prompt = "Select Project Folder"
	}

	path, err := zenity.SelectFile(
		zenity.Title(prompt),
		zenity.Directory(),
		zenity.Context(r.Context()),
	)
	if err == zenity.ErrCanceled {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "supported": true})
}
