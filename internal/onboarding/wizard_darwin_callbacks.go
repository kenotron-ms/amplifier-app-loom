//go:build darwin && cgo

package onboarding

/*
#include <stdlib.h>
// Only extern declarations — definitions live in wizard_darwin_impl.go.
// CGo generates _cgo_export.h which provides these at link time to that file.
extern void wizard_eval_js(const char *js);
extern void wizard_close(void);
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kardianos/service"
	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/platform"
	internalsvc "github.com/ms/amplifier-app-loom/internal/service"
	"github.com/ms/amplifier-app-loom/internal/store"
)

// wizardGoMessage is called from ObjC when JS posts to window.webkit.messageHandlers.agent.
// Messages: setAnthropicKey, setOpenAIKey, openSettings, installService, done.
//
//export wizardGoMessage
func wizardGoMessage(cAction *C.char, cPayload *C.char) {
	action := C.GoString(cAction)
	payload := C.GoString(cPayload)
	s := gState.Load()
	if s == nil {
		return
	}
	switch action {
	case "setAnthropicKey":
		s.mu.Lock()
		s.anthropicKey = payload
		s.mu.Unlock()
	case "setOpenAIKey":
		s.mu.Lock()
		s.openAIKey = payload
		s.mu.Unlock()
	case "openSettings":
		openSystemSettings()
		go pollFDA(s)
	case "installService":
		// payload is "user" or "system"
		go doInstallService(payload, s)
	case "done":
		go handleDone(s)
	}
}

// wizardGoActivation is called from NSNotificationCenter when the app becomes active.
// Primary FDA detection signal: user returned from System Settings.
//
//export wizardGoActivation
func wizardGoActivation() {
	s := gState.Load()
	if s == nil || s.fdaGranted.Load() {
		return
	}
	if CheckFDA() {
		s.fdaGranted.Store(true)
		pushJS(`window.dispatchEvent(new CustomEvent('fdaGranted'))`)
	}
}

// handleDone is the final action when the user clicks Done on the last step.
// It saves any API keys that were entered, captures UserContext, marks
// OnboardingComplete, and closes the wizard. Service installation is handled
// separately by doInstallService (triggered from the Install step).
func handleDone(s *state) {
	s.mu.Lock()
	anthropicKey := s.anthropicKey
	openAIKey := s.openAIKey
	s.mu.Unlock()

	// Try API path first — avoids DB lock contention when daemon is already running.
	// Falls back to direct DB access for fresh installs where no daemon is running yet.
	if trySaveKeysViaAPI(anthropicKey, openAIKey) {
		slog.Info("onboarding: saved keys via daemon API")
		closeWizard(s)
		return
	}

	// Daemon not running — safe to open DB directly.
	st, err := store.Open(platform.DBPath())
	if err != nil {
		pushInstallError("Failed to open database: " + err.Error())
		return
	}
	defer st.Close()

	cfg, err := st.GetConfig(context.Background())
	if err != nil {
		pushInstallError("Failed to read config: " + err.Error())
		return
	}

	// Persist the API keys snapshotted above.
	cfg.AnthropicKey = anthropicKey
	cfg.OpenAIKey = openAIKey

	// Capture user context (HomeDir, Shell, UID) — always refresh on completion.
	if uc := config.CaptureUserContext(); uc != nil {
		cfg.UserContext = uc
		slog.Info("onboarding: captured user context", "home", uc.HomeDir, "shell", uc.Shell)
	}

	cfg.OnboardingComplete = true

	if err := st.SaveConfig(context.Background(), cfg); err != nil {
		pushInstallError("Failed to save config: " + err.Error())
		return
	}

	closeWizard(s)
}

// trySaveKeysViaAPI attempts to persist API keys through the running daemon's
// HTTP endpoint. Returns true if the daemon is reachable and responds 2xx.
// Using the HTTP path avoids BoltDB exclusive-lock contention: the daemon owns
// the write lock while it is running, so opening the DB file directly from the
// tray/wizard process will time out.
func trySaveKeysViaAPI(anthropicKey, openAIKey string) bool {
	body, err := json.Marshal(map[string]string{
		"anthropicKey": anthropicKey,
		"openAIKey":    openAIKey,
	})
	if err != nil {
		return false
	}
	url := fmt.Sprintf("http://localhost:%d/api/settings", config.DefaultPort)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Daemon not reachable — this is expected on fresh installs.
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// doInstallService handles the Install Service wizard step.
// level is "user" (LaunchAgent, no admin) or "system" (LaunchDaemon, needs admin).
// Posts serviceInstalled or serviceError events back to the JS layer.
func doInstallService(level string, s *state) {
	if level == "system" {
		exePath, err := os.Executable()
		if err != nil {
			pushServiceError("Cannot determine binary path: " + err.Error())
			return
		}
		script := fmt.Sprintf(
			`do shell script "%s install --system" with administrator privileges`,
			exePath,
		)
		if err := exec.Command("osascript", "-e", script).Run(); err != nil {
			pushServiceError("System install failed (admin password required): " + err.Error())
			return
		}
	} else {
		svc, err := internalsvc.NewServiceForControl(internalsvc.LevelUser)
		if err != nil {
			pushServiceError("Cannot create service config: " + err.Error())
			return
		}
		if err := service.Control(svc, "install"); err != nil {
			pushServiceError("Install failed: " + err.Error())
			return
		}
	}

	// Capture user context now — we're still in the user's interactive session.
	st, err := store.Open(platform.DBPath())
	if err == nil {
		if cfg, err := st.GetConfig(context.Background()); err == nil {
			if uc := config.CaptureUserContext(); uc != nil {
				cfg.UserContext = uc
				_ = st.SaveConfig(context.Background(), cfg)
			}
		}
		st.Close()
	}

	// Start service (best-effort).
	if svc, err := internalsvc.NewServiceForControl(internalsvc.LevelUser); err == nil {
		_ = service.Control(svc, "start")
	}

	slog.Info("onboarding: service installed", "level", level)
	pushJS(`window.dispatchEvent(new CustomEvent('serviceInstalled'))`)
}

// closeWizard marks the session closed, clears gState, and closes the NSPanel.
func closeWizard(s *state) {
	s.closed.Store(true)
	gState.Store(nil)
	C.wizard_close()
	if s.onDone != nil {
		s.onDone()
	}
}

// pushServiceError sends a serviceError event to the wizard JS layer.
func pushServiceError(msg string) {
	msgJSON, _ := json.Marshal(msg)
	pushJS(fmt.Sprintf(
		`window.dispatchEvent(new CustomEvent('serviceError', {detail: {msg: %s}}))`,
		string(msgJSON),
	))
}

// openSystemSettings deep-links to Privacy & Security → Full Disk Access.
func openSystemSettings() {
	if err := exec.Command("open",
		"x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles",
	).Start(); err != nil {
		slog.Warn("onboarding: failed to open System Settings", "err", err)
	}
}

// isServiceInstalled checks whether the LaunchAgent or LaunchDaemon plist exists.
func isServiceInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("onboarding: cannot determine home dir for plist check", "err", err)
	}
	if home != "" {
		plistPath := filepath.Join(home, "Library", "LaunchAgents", internalsvc.LaunchAgentPlistName)
		if _, err := os.Stat(plistPath); err == nil {
			return true
		}
	}
	systemPath := filepath.Join("/Library", "LaunchDaemons", internalsvc.LaunchAgentPlistName)
	if _, err := os.Stat(systemPath); err == nil {
		return true
	}
	return false
}

// pushInstallError sends an installError event to the wizard JS layer.
func pushInstallError(msg string) {
	msgJSON, _ := json.Marshal(msg) // json.Marshal handles all JS-unsafe chars: \n, \r, \0, ", \
	pushJS(fmt.Sprintf(
		`window.dispatchEvent(new CustomEvent('installError', {detail: {msg: %s}}))`,
		string(msgJSON),
	))
}
