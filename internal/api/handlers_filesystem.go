package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// pickFolder opens a native directory picker by invoking the zenity binary directly.
// No Go wrapper, no AppleScript — just exec.Command("zenity", "--file-selection", "--directory").
//
// GET /api/filesystem/pick-folder         — open the dialog
// GET /api/filesystem/pick-folder?check=1 — probe: returns {supported: true/false}
func (s *Server) pickFolder(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("check") != "" {
		_, err := exec.LookPath("zenity")
		writeJSON(w, http.StatusOK, map[string]any{"supported": err == nil})
		return
	}

	zenityPath, err := exec.LookPath("zenity")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "zenity not found — install it first")
		return
	}

	cmd := exec.CommandContext(r.Context(), zenityPath,
		"--file-selection",
		"--directory",
		"--title=Select Project Folder",
	)
	out, err := cmd.Output()
	if err != nil {
		// exit code 1 = user cancelled
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	path := strings.TrimRight(string(out), "\r\n")
	writeJSON(w, http.StatusOK, map[string]any{"path": path})
}

// findDir resolves a folder name to candidate absolute paths via Spotlight/find.
// Used as a fallback when the native picker is unavailable.
//
// GET /api/filesystem/find-dir?name=loom
func (s *Server) findDir(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeJSON(w, http.StatusOK, map[string]any{"paths": []string{}})
		return
	}
	home, _ := os.UserHomeDir()
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		query := `kMDItemKind == "Folder" && kMDItemFSName == "` + name + `"`
		out, err := exec.CommandContext(r.Context(), "mdfind", "-onlyin", home, query).Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.Contains(line, "/Library/") ||
					strings.Contains(line, "/.Trash/") || strings.Contains(line, "/Cache") {
					continue
				}
				if looksLikeProject(line) {
					paths = append(paths, line)
				}
			}
		}
	case "linux":
		out, err := exec.CommandContext(r.Context(), "find", home,
			"-maxdepth", "6", "-type", "d", "-name", name, "-not", "-path", "*/.*").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line = strings.TrimSpace(line); line != "" && looksLikeProject(line) {
					paths = append(paths, line)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"paths": paths})
}

func looksLikeProject(dir string) bool {
	for _, m := range []string{".git", "go.mod", "package.json", "Cargo.toml",
		"pyproject.toml", "setup.py", "pom.xml", "Makefile"} {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}
