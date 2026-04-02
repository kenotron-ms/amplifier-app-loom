package amplifier

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed prepare_session.py
var prepareSessionPy []byte

// PrepareSession pre-creates an Amplifier session directory for projectPath
// and returns the new session UUID.
//
// It writes prepare_session.py to a temp file, runs it with the system Python
// interpreter, and reads the session ID from stdout (one clean line, no banner
// parsing required).
//
// The created directory is:
//
//	~/.amplifier/projects/<slug>/sessions/<uuid>/
//
// Calling code can then always start amplifier with:
//
//	amplifier run --mode chat --resume <uuid>
func PrepareSession(projectPath string) (string, error) {
	// Write the embedded script to a temp file
	tmp, err := os.CreateTemp("", "loom-prepare-session-*.py")
	if err != nil {
		return "", fmt.Errorf("PrepareSession: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(prepareSessionPy); err != nil {
		tmp.Close()
		return "", fmt.Errorf("PrepareSession: write script: %w", err)
	}
	tmp.Close()

	python, err := resolvePython()
	if err != nil {
		return "", fmt.Errorf("PrepareSession: %w", err)
	}

	out, execErr := exec.Command(python, tmpPath, projectPath).Output()
	if execErr != nil {
		msg := execErr.Error()
		if exitErr, ok := execErr.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			msg = strings.TrimSpace(string(exitErr.Stderr))
		}
		return "", fmt.Errorf("PrepareSession: script failed: %s", msg)
	}

	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("PrepareSession: script returned empty session ID")
	}
	return id, nil
}

// resolvePython finds a usable Python 3 interpreter.
// macOS GUI apps and launchd services run with a stripped PATH
// that often doesn't include ~/.local/bin or /opt/homebrew/bin.
func resolvePython() (string, error) {
	// Standard PATH first — works in shell-launched contexts
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	// Probe common install locations
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "python3"),
		"/opt/homebrew/bin/python3",
		"/usr/local/bin/python3",
		"/usr/bin/python3",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("python3 not found — install Python 3 to enable session pre-creation")
}
