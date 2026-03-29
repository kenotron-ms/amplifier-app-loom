//go:build cgo

package tray

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/systray"
	"github.com/kardianos/service"

	"github.com/ms/amplifier-app-loom/internal/api"
	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/onboarding"
	"github.com/ms/amplifier-app-loom/internal/platform"
	internalsvc "github.com/ms/amplifier-app-loom/internal/service"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/updater"
)

type healthIssue struct {
	msg     string // shown in menu
	fixKind string // "apikey" | "fda" | "service" | "cli" | "amplifier"
}

// pendingReExec is set by the update apply flow before calling systray.Quit().
// The onQuit callback reads it and re-execs into the new binary.
var pendingReExec string

// Run launches the system tray app. Blocks until the user quits.
// Must be called from the main goroutine.
func Run(port int) error {
	systray.Run(
		func() { onReady(port) },
		func() {
			// If an update was applied, re-exec as tray once systray is fully shut down.
			if pendingReExec != "" {
				updater.ReExec(pendingReExec, "tray")
			}
		},
	)
	return nil
}

func isDaemonRunning(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/status", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func isCLIInPath() bool {
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		"/usr/local/bin/loom",
		filepath.Join(home, ".local", "bin", "loom"),
	} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func isAmplifierConnected() bool {
	amplifierPath, err := exec.LookPath("amplifier")
	if err != nil {
		return true // amplifier not installed — nothing to connect
	}
	out, err := exec.Command(amplifierPath, "bundle", "list").Output()
	if err != nil {
		return true // can't check — don't surface a spurious warning
	}
	return strings.Contains(string(out), "loom")
}

func checkHealth(port int) []healthIssue {
	var issues []healthIssue

	s, err := store.Open(platform.DBPath())
	if err == nil {
		if cfg, err := s.GetConfig(context.Background()); err == nil {
			if cfg.AnthropicKey == "" {
				issues = append(issues, healthIssue{"Anthropic API key missing", "apikey"})
			}
		}
		s.Close()
	}

	if !onboarding.CheckFDA() {
		issues = append(issues, healthIssue{"Full Disk Access missing", "fda"})
	}

	if !isServiceInstalled() && !isDaemonRunning(port) {
		issues = append(issues, healthIssue{"Background service not installed", "service"})
	}

	// Service running check (HTTP 200); treat connection refused as dead service.
	url := fmt.Sprintf("http://localhost:%d/api/status", port)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		issues = append(issues, healthIssue{"Service not responding", "service"})
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			issues = append(issues, healthIssue{"Service not responding", "service"})
		}
	}

	if !isCLIInPath() {
		issues = append(issues, healthIssue{"CLI not in terminal PATH", "cli"})
	}

	if !isAmplifierConnected() {
		issues = append(issues, healthIssue{"Amplifier not connected", "amplifier"})
	}

	return issues
}

func onReady(port int) {
	// ── Onboarding check (macOS .app launch only) ─────────────────────────
	// Shows the first-run wizard if API key, UserContext, or FDA are missing.
	if ost, err := store.Open(platform.DBPath()); err != nil {
		slog.Warn("tray: onboarding check: cannot open store", "err", err)
	} else {
		if cfg, err := ost.GetConfig(context.Background()); err != nil {
			slog.Warn("tray: onboarding check: cannot read config", "err", err)
		} else if onboarding.NeedsOnboarding(cfg) {
			onboarding.Show(ost, func() {
				slog.Info("tray: onboarding complete")
			})
		}
		ost.Close()
	}

	// Step 1: tray icon appears immediately.
	icon := makeIcon()
	systray.SetTemplateIcon(icon, icon)
	systray.SetTooltip("loom")

	// Step 3: show setup prompts for anything not yet configured.
	// Service prompt: only if plist missing AND daemon not already running.
	var mSetup *systray.MenuItem
	if !isServiceInstalled() && !isDaemonRunning(port) {
		mSetup = systray.AddMenuItem("⚙  Set up background service…", "Install loom so it starts automatically")
		systray.AddSeparator()
	}

	// ── Status section ──────────────────────────────────────────────────────────────────────────────────────────────────
	mStatus := systray.AddMenuItem("⬤  Checking…", "Daemon status")
	mStatus.Disable()
	mDetails := systray.AddMenuItem("", "Jobs / queue details")
	mDetails.Disable()
	mDetails.Hide()

	// ── Health indicator (hidden until first check fires) ─────────────────
	mActionRequired := systray.AddMenuItem("⚠  Action Required", "One or more setup issues require attention")
	mActionRequired.Hide()
	mFixAPIKey := mActionRequired.AddSubMenuItem("Anthropic API key missing   Fix →", "Open settings to add API key")
	mFixFDA := mActionRequired.AddSubMenuItem("Full Disk Access missing     Fix →", "Open System Settings")
	mFixService := mActionRequired.AddSubMenuItem("Service not installed         Fix →", "Install background service")
	mFixCLI := mActionRequired.AddSubMenuItem("CLI not in terminal PATH     Fix →", "Add loom to PATH")
	mFixAmplifier := mActionRequired.AddSubMenuItem("Amplifier not connected      Fix →", "Register Amplifier bundle")
	mFixAPIKey.Hide()
	mFixFDA.Hide()
	mFixService.Hide()
	mFixCLI.Hide()
	mFixAmplifier.Hide()

	systray.AddSeparator()

	// ── Quick actions ───────────────────────────────────────────────────────────────────────────────────────────────────
	mOpenUI := systray.AddMenuItem("Open Web UI", fmt.Sprintf("http://localhost:%d", port))
	systray.AddSeparator()
	mStart := systray.AddMenuItem("Start Daemon", "")
	mStop := systray.AddMenuItem("Stop Daemon", "")
	mPause := systray.AddMenuItem("Pause Scheduling", "")
	mResume := systray.AddMenuItem("Resume Scheduling", "")
	mResume.Hide()

	systray.AddSeparator()

	// ── Installation section ────────────────────────────────────────────────────────────────────────────────────────────
	mInstall := systray.AddMenuItem("Install", "")
	mInstallUser := mInstall.AddSubMenuItem("User (login items)", "Starts on login, no sudo needed")
	mInstallSystem := mInstall.AddSubMenuItem("System (boot daemon)", "Starts at boot, requires sudo")
	mUninstall := systray.AddMenuItem("Uninstall", "Remove installed service")

	systray.AddSeparator()
	mCheckUpdate := systray.AddMenuItem("Check for Updates", "Check GitHub for a newer release")
	mUpdateAvail := systray.AddMenuItem("", "")
	mUpdateAvail.Hide()
	mQuit := systray.AddMenuItem("Quit Tray", "Close the tray app (daemon keeps running)")

	// ── Background tasks ────────────────────────────────────────────────────────────────────────────────────────────────
	// CLI install runs in background — may show an admin dialog, must not block.
	go installCLIIfNeeded()

	go func() {
		for {
			updateStatus(port, mStatus, mDetails, mStart, mStop, mPause, mResume)
			time.Sleep(2 * time.Second)
		}
	}()

	// ── Auto-updater ──────────────────────────────────────────────────────────
	// onChange runs from the updater's goroutine; systray menu ops are safe
	// to call from any goroutine.
	u := updater.New(api.Version, func(s updater.State, ver string) {
		switch s {
		case updater.StateDownloading:
			mUpdateAvail.SetTitle(fmt.Sprintf("⬇  Downloading v%s…", ver))
			mUpdateAvail.SetTooltip("Please wait — downloading update")
			mUpdateAvail.Disable()
			mUpdateAvail.Show()
		case updater.StateReady:
			mUpdateAvail.SetTitle(fmt.Sprintf("↻  Restart to apply v%s", ver))
			mUpdateAvail.SetTooltip("Click to stop the service, swap binary, reinstall, and re-launch")
			mUpdateAvail.Enable()
			mUpdateAvail.Show()
		case updater.StateApplying:
			mUpdateAvail.SetTitle("⏳  Applying update…")
			mUpdateAvail.Disable()
		case updater.StateFailed:
			mUpdateAvail.SetTitle("⚠  Update failed — click to retry")
			mUpdateAvail.SetTooltip("Click to try again")
			mUpdateAvail.Enable()
			mUpdateAvail.Show()
		default:
			mUpdateAvail.Hide()
		}
	})

	// Check on startup (after a short delay so the tray init completes first),
	// then every 4 hours.
	go func() {
		time.Sleep(5 * time.Second)
		_ = u.CheckAndStage(context.Background())

		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			_ = u.CheckAndStage(context.Background())
		}
	}()

	// Health check: starts after onboarding is done (poll OnboardingComplete).
	// Runs every 30 seconds once active. Does NOT run concurrently with the wizard.
	go func() {
		// Wait until onboarding is marked complete.
		for {
			time.Sleep(5 * time.Second)
			hs, err := store.Open(platform.DBPath())
			if err != nil {
				continue
			}
			cfg, err := hs.GetConfig(context.Background())
			hs.Close()
			if err == nil && cfg.OnboardingComplete {
				break
			}
		}
		// 30-second health check loop.
		for {
			issues := checkHealth(port)
			if len(issues) == 0 {
				systray.SetTooltip("loom")
				mActionRequired.Hide()
			} else {
				systray.SetTooltip("loom ⚠ action required")
				mFixAPIKey.Hide()
				mFixFDA.Hide()
				mFixService.Hide()
				mFixCLI.Hide()
				mFixAmplifier.Hide()
				for _, iss := range issues {
					slog.Debug("tray: health issue detected", "msg", iss.msg, "kind", iss.fixKind)
					switch iss.fixKind {
					case "apikey":
						mFixAPIKey.Show()
					case "fda":
						mFixFDA.Show()
					case "service":
						mFixService.Show()
					case "cli":
						mFixCLI.Show()
					case "amplifier":
						mFixAmplifier.Show()
					}
				}
				mActionRequired.Show()
			}
			time.Sleep(30 * time.Second)
		}
	}()

	// ── Event loop ──────────────────────────────────────────────────────────────────────────────────────────────────────
	setupCh := make(chan struct{})
	if mSetup != nil {
		setupCh = mSetup.ClickedCh
	}

	for {
		select {
		case <-setupCh:
			if runServiceInstallDialog() && mSetup != nil {
				mSetup.Hide()
				setupCh = make(chan struct{}) // disarm
			}

		case <-mOpenUI.ClickedCh:
			openBrowser(fmt.Sprintf("http://localhost:%d", port))

		case <-mStart.ClickedCh:
			runServiceControl(internalsvc.LevelUser, "start")

		case <-mStop.ClickedCh:
			runServiceControl(internalsvc.LevelUser, "stop")

		case <-mPause.ClickedCh:
			daemonPost(port, "/api/daemon/pause")

		case <-mResume.ClickedCh:
			daemonPost(port, "/api/daemon/resume")

		case <-mInstallUser.ClickedCh:
			installService(internalsvc.LevelUser)

		case <-mInstallSystem.ClickedCh:
			installService(internalsvc.LevelSystem)

		case <-mUninstall.ClickedCh:
			uninstallService()

		case <-mCheckUpdate.ClickedCh:
			mCheckUpdate.SetTitle("Checking…")
			mCheckUpdate.Disable()
			go func() {
				_ = u.CheckAndStage(context.Background())
				mCheckUpdate.SetTitle("Check for Updates")
				mCheckUpdate.Enable()
			}()

		case <-mUpdateAvail.ClickedCh:
			switch u.State() {
			case updater.StateReady:
				// Apply: stop service, swap binary, reinstall service.
				// Then quit systray cleanly — the onQuit callback does the re-exec.
				go func() {
					newExe, err := u.Apply()
					if err != nil {
						slog.Error("tray: auto-update apply failed", "err", err)
						return
					}
					pendingReExec = newExe
					systray.Quit()
				}()
			case updater.StateFailed:
				// Retry the check + download.
				go func() {
					_ = u.CheckAndStage(context.Background())
				}()
			}

		case <-mFixAPIKey.ClickedCh:
			openBrowser(fmt.Sprintf("http://localhost:%d/#/settings", port))

		case <-mFixFDA.ClickedCh:
			if ost, err := store.Open(platform.DBPath()); err != nil {
				slog.Warn("tray: mFixFDA: cannot open store", "err", err)
			} else {
				onboarding.Show(ost, func() {
					slog.Info("tray: FDA fix via health indicator complete")
				})
				ost.Close()
			}

		case <-mFixService.ClickedCh:
			captureAndSaveUserContext()
			installService(internalsvc.LevelUser)

		case <-mFixCLI.ClickedCh:
			go installCLIIfNeeded()

		case <-mFixAmplifier.ClickedCh:
			go connectAmplifier()

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

// isServiceInstalled returns true if a LaunchAgent or LaunchDaemon plist exists.
func isServiceInstalled() bool {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", "loom.plist")); err == nil {
		return true
	}
	if _, err := os.Stat("/Library/LaunchDaemons/loom.plist"); err == nil {
		return true
	}
	return false
}

// runServiceInstallDialog shows the install dialog and performs the install.
// Returns true if install succeeded.
func runServiceInstallDialog() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}

	level, ok := showInstallDialog()
	if !ok {
		return false // user cancelled
	}

	slog.Info("setup: installing daemon service", "level", level)

	if level == internalsvc.LevelSystem {
		script := fmt.Sprintf(
			`do shell script "%s install --system" with administrator privileges`,
			exePath,
		)
		if err := exec.Command("osascript", "-e", script).Run(); err != nil {
			slog.Error("setup: system install failed", "err", err)
			return false
		}
		captureAndSaveUserContext()
		return true
	}

	svc, err := internalsvc.NewServiceForControl(internalsvc.LevelUser)
	if err != nil {
		return false
	}
	if err := service.Control(svc, "install"); err != nil {
		slog.Error("setup: install failed", "err", err)
		return false
	}
	_ = service.Control(svc, "start")
	captureAndSaveUserContext()
	return true
}

// showInstallDialog presents a native macOS dialog to choose install level.
// Returns the chosen level and true, or false if cancelled.
func showInstallDialog() (internalsvc.InstallLevel, bool) {
	script := `tell application "System Events"
	set choice to display dialog "Choose how Loom should run in the background:" ¬
		buttons {"Cancel", "System (all users, starts at boot)", "Just for me (login item)"} ¬
		default button "Just for me (login item)" ¬
		with title "Loom Setup" ¬
		with icon caution
	return button returned of choice
end tell`

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return internalsvc.LevelUser, false
	}
	if strings.HasPrefix(strings.TrimSpace(string(out)), "System") {
		return internalsvc.LevelSystem, true
	}
	return internalsvc.LevelUser, true
}

// installCLIIfNeeded symlinks the binary to /usr/local/bin when running from
// a .app bundle for the first time so `loom` works in the terminal.
func installCLIIfNeeded() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	if !strings.Contains(exePath, ".app/Contents/MacOS/") {
		return
	}

	home, _ := os.UserHomeDir()
	target := "/usr/local/bin/loom"

	if _, err := os.Lstat(target); err == nil {
		return // already there
	}

	// Try without privileges first (writable on Homebrew setups).
	if os.Symlink(exePath, target) == nil {
		slog.Info("installed CLI to /usr/local/bin/loom")
		return
	}

	// Ask for admin via macOS password dialog.
	script := fmt.Sprintf(
		`do shell script "ln -sf %q /usr/local/bin/loom" with administrator privileges`,
		exePath,
	)
	if exec.Command("osascript", "-e", script).Run() == nil {
		slog.Info("installed CLI to /usr/local/bin/loom (via admin dialog)")
		return
	}

	// Fall back to ~/.local/bin silently.
	localBin := filepath.Join(home, ".local", "bin")
	_ = os.MkdirAll(localBin, 0755)
	if os.Symlink(exePath, filepath.Join(localBin, "loom")) == nil {
		slog.Info("installed CLI to ~/.local/bin/loom")
		addToShellProfile(home, localBin)
	}
}

// connectAmplifier registers the loom bundle as an Amplifier app bundle.
func connectAmplifier() {
	amplifierPath, err := exec.LookPath("amplifier")
	if err != nil {
		slog.Warn("tray: amplifier not found in PATH")
		return
	}
	out, err := exec.Command(amplifierPath, "bundle", "add",
		"git+https://github.com/kenotron-ms/amplifier-app-loom@main", "--app").CombinedOutput()
	if err != nil {
		slog.Warn("tray: amplifier bundle add failed", "err", err, "out", string(out))
		return
	}
	slog.Info("tray: amplifier bundle registered")
}

// addToShellProfile appends a PATH export to shell rc files if not already present.
func addToShellProfile(home, dir string) {
	line := fmt.Sprintf("\nexport PATH=\"%s:$PATH\" # added by Loom\n", dir)
	for _, rc := range []string{".zshrc", ".bashrc", ".bash_profile"} {
		p := filepath.Join(home, rc)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		contents, _ := os.ReadFile(p)
		if strings.Contains(string(contents), dir) {
			continue
		}
		if f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			_, _ = f.WriteString(line)
			f.Close()
		}
	}
}

// updateStatus polls the daemon and updates menu items accordingly.
func updateStatus(port int, mStatus, mDetails, mStart, mStop, mPause, mResume *systray.MenuItem) {
	url := fmt.Sprintf("http://localhost:%d/api/status", port)
	resp, err := http.Get(url)
	if err != nil {
		mStatus.SetTitle("⬤  Offline")
		mDetails.Hide()
		mStart.Show()
		mStop.Hide()
		mPause.Hide()
		mResume.Hide()
		return
	}
	defer resp.Body.Close()

	var s struct {
		State      string `json:"state"`
		ActiveRuns int    `json:"activeRuns"`
		QueueDepth int    `json:"queueDepth"`
		JobCount   int    `json:"jobCount"`
		Version    string `json:"version"`
	}
	if err := jsonDecode(resp.Body, &s); err != nil {
		return
	}

	switch s.State {
	case "running":
		mStatus.SetTitle("⬤  Running  v" + s.Version)
	case "paused":
		mStatus.SetTitle("⏸  Paused   v" + s.Version)
	default:
		mStatus.SetTitle("⬤  " + s.State)
	}

	if s.JobCount > 0 || s.ActiveRuns > 0 || s.QueueDepth > 0 {
		mDetails.SetTitle(fmt.Sprintf("   %d jobs · %d running · %d queued", s.JobCount, s.ActiveRuns, s.QueueDepth))
		mDetails.Show()
	} else {
		mDetails.SetTitle(fmt.Sprintf("   %d jobs", s.JobCount))
		mDetails.Show()
	}

	mStart.Hide()
	mStop.Show()

	if s.State == "paused" {
		mPause.Hide()
		mResume.Show()
	} else {
		mPause.Show()
		mResume.Hide()
	}
}

func installService(level internalsvc.InstallLevel) {
	svc, err := internalsvc.NewServiceForControl(level)
	if err != nil {
		return
	}
	if err := service.Control(svc, "install"); err != nil {
		slog.Warn("tray: service install failed", "level", level, "err", err)
		return
	}
	_ = service.Control(svc, "start")
	captureAndSaveUserContext()
}

// captureAndSaveUserContext saves the current user's HomeDir, Shell, and UID
// into the daemon's database. Must be called while still in the user's
// interactive session (before the daemon takes over under launchd).
func captureAndSaveUserContext() {
	uc := config.CaptureUserContext()
	if uc == nil {
		return
	}
	s, err := store.Open(platform.DBPath())
	if err != nil {
		slog.Warn("tray: could not open store for UserContext capture", "err", err)
		return
	}
	defer s.Close()
	cfg, err := s.GetConfig(context.Background())
	if err != nil {
		slog.Warn("tray: could not load config for UserContext capture", "err", err)
		return
	}
	cfg.UserContext = uc
	if err := s.SaveConfig(context.Background(), cfg); err != nil {
		slog.Warn("tray: failed to save user context", "err", err)
		return
	}
	slog.Info("tray: captured user context", "home", uc.HomeDir, "shell", uc.Shell)
}

func uninstallService() {
	for _, level := range []internalsvc.InstallLevel{internalsvc.LevelUser, internalsvc.LevelSystem} {
		svc, err := internalsvc.NewServiceForControl(level)
		if err != nil {
			continue
		}
		_ = service.Control(svc, "stop")
		_ = service.Control(svc, "uninstall")
	}
}

func runServiceControl(level internalsvc.InstallLevel, action string) {
	svc, err := internalsvc.NewServiceForControl(level)
	if err != nil {
		return
	}
	_ = service.Control(svc, action)
}

func daemonPost(port int, path string) {
	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	resp, err := http.Post(url, "application/json", nil)
	if err == nil {
		resp.Body.Close()
	}
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "start"
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, url).Start()
}
