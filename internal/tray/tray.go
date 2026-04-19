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

	bolt "go.etcd.io/bbolt"

	"fyne.io/systray"
	"github.com/kardianos/service"

	"github.com/ms/amplifier-app-loom/internal/api"
	"github.com/ms/amplifier-app-loom/internal/meeting"
	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/onboarding"
	"github.com/ms/amplifier-app-loom/internal/platform"
	internalsvc "github.com/ms/amplifier-app-loom/internal/service"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/updater"
)

// Run launches the system tray app. Blocks until the user quits.
// Must be called from the main goroutine.
func Run(port int) error {
	systray.Run(
		func() { onReady(port) },
		func() {},
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

func onReady(port int) {
	// ── Bundle self-repair ──────────────────────────────────────────────────
	// Ensures Loom.icns is present in Contents/Resources/ even on installs
	// that predate the icon or were updated via the binary-only updater.
	repairBundle()

	// ── Onboarding check (macOS .app launch only) ───────────────────────────
	// Shows the first-run wizard if API key or background service are missing.
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

	icon := makeIcon()
	systray.SetTemplateIcon(icon, icon)
	systray.SetTooltip("loom")

	// ── Status ──────────────────────────────────────────────────────────────
	mStatus := systray.AddMenuItem("⬤  Checking…", "Daemon status")
	mStatus.Disable()
	mDetails := systray.AddMenuItem("", "Jobs / queue details")
	mDetails.Disable()
	mDetails.Hide()

	systray.AddSeparator()

	// ── Primary actions ─────────────────────────────────────────────────────
	mOpenUI := systray.AddMenuItem("Open Dashboard", fmt.Sprintf("http://localhost:%d", port))
	systray.AddSeparator()
	mStop := systray.AddMenuItem("Stop Service", "Stop the background daemon")
	mStart := systray.AddMenuItem("Start Service", "Start the background daemon")
	mPause := systray.AddMenuItem("Pause Jobs", "Suspend job scheduling (daemon keeps running)")
	mResume := systray.AddMenuItem("Resume Jobs", "Resume job scheduling")
	mResume.Hide()

	systray.AddSeparator()

	// ── Updates ─────────────────────────────────────────────────────────────
	mCheckUpdate := systray.AddMenuItem("Check for Updates", "Check GitHub for a newer release")
	mUpdateAvail := systray.AddMenuItem("", "")
	mUpdateAvail.Hide()

	systray.AddSeparator()

	// ── Bottom section ───────────────────────────────────────────────────────
	// API key warning: surfaces only when no key is configured.
	// Clicking it opens the settings page to fix the issue.
	mAPIKeyWarning := systray.AddMenuItem("⚠  API key not set — Open Settings", "Add an Anthropic or OpenAI key to enable AI jobs")
	mAPIKeyWarning.Hide()

	// ── Meeting transcription toggle ──────────────────────────────────────────
	systray.AddSeparator()
	mMeeting := systray.AddMenuItemCheckbox(
		"Meeting Transcription",
		"Record and transcribe Teams/Zoom/Meet meetings",
		false, // initial state, updated in goroutine below
	)

	go func() {
		// Open a dedicated bbolt handle for meeting config
		// (the tray's existing store is scoped to the onboarding check above)
		meetingDBPath := filepath.Join(platform.DataDir(), "meeting.db")
			meetingDB, err := bolt.Open(meetingDBPath, 0o600, &bolt.Options{Timeout: 2 * time.Second})
		if err != nil {
			slog.Error("tray: meeting: open db", "err", err)
			return
		}
		defer meetingDB.Close()

		meetingStore := meeting.NewConfigStore(meetingDB)
		cfg, _ := meetingStore.Get(context.Background())
		if cfg.Enabled {
			mMeeting.Check()
		}

		// Start the meeting service
		notifier := meeting.NewNotifier()
		notifier.Setup()
		trans := meeting.NewTranscriber("") // reads OPENAI_API_KEY from env
		svc := meeting.NewService(meetingStore, notifier, trans)
		if err := svc.Start(context.Background()); err != nil {
			slog.Error("tray: meeting: start failed", "err", err)
		}

		// Handle toggle clicks
		for range mMeeting.ClickedCh {
			enabled := !mMeeting.Checked()
			if err := svc.SetEnabled(context.Background(), enabled); err != nil {
				slog.Error("tray: meeting: toggle", "err", err)
				continue
			}
			if enabled {
				mMeeting.Check()
				slog.Info("tray: meeting transcription enabled")
			} else {
				mMeeting.Uncheck()
				slog.Info("tray: meeting transcription disabled")
			}
		}
	}()

	mUninstall := systray.AddMenuItem("Uninstall Loom…", "Remove the background service and close the tray")
	mQuit := systray.AddMenuItem("Quit", "Close the tray app (service keeps running)")

	// ── Background tasks ─────────────────────────────────────────────────────
	// Symlinks the binary to /usr/local/bin once, silently, from .app context.
	go installCLIIfNeeded()

	// Status poller: updates the status line and Start/Stop/Pause/Resume
	// visibility every 2 seconds.
	go func() {
		for {
			updateStatus(port, mStatus, mDetails, mStart, mStop, mPause, mResume)
			time.Sleep(2 * time.Second)
		}
	}()

	// ── Auto-updater ─────────────────────────────────────────────────────────
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

	go func() {
		time.Sleep(5 * time.Second)
		_ = u.CheckAndStage(context.Background())

		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			_ = u.CheckAndStage(context.Background())
		}
	}()

	// ── API key health check ──────────────────────────────────────────────────
	// Only surfaces the one issue users can act on directly from the tray:
	// a missing API key.  Everything else (service install, FDA, CLI) is
	// handled by the onboarding wizard when the app first launches.
	go func() {
		for {
			time.Sleep(30 * time.Second)
			hs, err := store.Open(platform.DBPath())
			if err != nil {
				continue
			}
			cfg, err := hs.GetConfig(context.Background())
			hs.Close()
			if err != nil {
				continue
			}
			if cfg.AnthropicKey == "" && cfg.OpenAIKey == "" {
				mAPIKeyWarning.Show()
				systray.SetTooltip("loom ⚠ API key missing")
			} else {
				mAPIKeyWarning.Hide()
				systray.SetTooltip("loom")
			}
		}
	}()

	// ── Event loop ───────────────────────────────────────────────────────────
	for {
		select {
		case <-mOpenUI.ClickedCh:
			openBrowser(fmt.Sprintf("http://localhost:%d", port))

		case <-mStart.ClickedCh:
			// Handle the post-uninstall case: if the plist is gone, install
			// (user-level login item) then start, rather than just start.
			if !isServiceInstalled() {
				captureAndSaveUserContext()
				installService(internalsvc.LevelUser)
			} else {
				runServiceControl(internalsvc.LevelUser, "start")
			}

		case <-mStop.ClickedCh:
			runServiceControl(internalsvc.LevelUser, "stop")

		case <-mPause.ClickedCh:
			daemonPost(port, "/api/daemon/pause")

		case <-mResume.ClickedCh:
			daemonPost(port, "/api/daemon/resume")

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
				go func() {
					newExe, err := u.Apply()
					if err != nil {
						slog.Error("tray: auto-update apply failed", "err", err)
						return
					}
					cmd := exec.Command(newExe, "tray")
					if err := cmd.Start(); err != nil {
						slog.Error("tray: relaunch failed", "path", newExe, "err", err)
					} else {
						slog.Info("tray: relaunched new tray", "pid", cmd.Process.Pid)
						_ = cmd.Process.Release()
					}
					systray.Quit()
				}()
			case updater.StateFailed:
				go func() { _ = u.CheckAndStage(context.Background()) }()
			}

		case <-mAPIKeyWarning.ClickedCh:
			openBrowser(fmt.Sprintf("http://localhost:%d/#/settings", port))

		case <-mUninstall.ClickedCh:
			if confirmUninstall() {
				uninstallService()
				systray.Quit()
				return
			}

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

// confirmUninstall shows a native macOS dialog asking the user to confirm.
// Returns true only if the user clicked "Uninstall".
func confirmUninstall() bool {
	script := `tell application "System Events"
	set choice to display dialog "This will remove the Loom background service.\n\nThe tray will close. You can reinstall by launching the app again." ¬
		buttons {"Cancel", "Uninstall"} ¬
		default button "Cancel" ¬
		with title "Uninstall Loom?" ¬
		with icon caution
	return button returned of choice
end tell`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "Uninstall"
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

// updateStatus polls the daemon and updates the status line and
// Start/Stop/Pause/Resume visibility.
func updateStatus(port int, mStatus, mDetails, mStart, mStop, mPause, mResume *systray.MenuItem) {
	url := fmt.Sprintf("http://localhost:%d/api/status", port)
	resp, err := http.Get(url) //nolint:noctx
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
	} else {
		mDetails.SetTitle(fmt.Sprintf("   %d jobs", s.JobCount))
	}
	mDetails.Show()

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
	resp, err := http.Post(url, "application/json", nil) //nolint:noctx
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


