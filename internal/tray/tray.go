//go:build cgo

package tray

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/getlantern/systray"
	"github.com/kardianos/service"

	internalsvc "github.com/ms/agent-daemon/internal/service"
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

func onReady(port int) {
	// Step 1: copy CLI binary to PATH immediately — fast, no dialogs.
	installCLIIfNeeded()

	// Step 2: tray icon appears.
	icon := makeIcon()
	systray.SetTemplateIcon(icon, icon)
	systray.SetTooltip("agent-daemon")

	// Step 3: if not yet installed as a service, show a setup prompt at the
	// top of the menu. The user can click it whenever they're ready.
	var mSetup *systray.MenuItem
	if !isServiceInstalled() {
		mSetup = systray.AddMenuItem("⚙  Set up background service…", "Install agent-daemon so it starts automatically")
		systray.AddSeparator()
	}

	// ── Status section ────────────────────────────────────────────────────────
	mStatus := systray.AddMenuItem("⬤  Checking…", "Daemon status")
	mStatus.Disable()
	mDetails := systray.AddMenuItem("", "Jobs / queue details")
	mDetails.Disable()
	mDetails.Hide()

	systray.AddSeparator()

	// ── Quick actions ─────────────────────────────────────────────────────────
	mOpenUI := systray.AddMenuItem("Open Web UI", fmt.Sprintf("http://localhost:%d", port))
	systray.AddSeparator()
	mStart := systray.AddMenuItem("Start Daemon", "")
	mStop := systray.AddMenuItem("Stop Daemon", "")
	mPause := systray.AddMenuItem("Pause Scheduling", "")
	mResume := systray.AddMenuItem("Resume Scheduling", "")
	mResume.Hide()

	systray.AddSeparator()

	// ── Installation section ──────────────────────────────────────────────────
	mInstall := systray.AddMenuItem("Install", "")
	mInstallUser := mInstall.AddSubMenuItem("User (login items)", "Starts on login, no sudo needed")
	mInstallSystem := mInstall.AddSubMenuItem("System (boot daemon)", "Starts at boot, requires sudo")
	mUninstall := systray.AddMenuItem("Uninstall", "Remove installed service")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Tray", "Close the tray app (daemon keeps running)")

	// ── Background poller ─────────────────────────────────────────────────────
	go func() {
		for {
			updateStatus(port, mStatus, mDetails, mStart, mStop, mPause, mResume)
			time.Sleep(2 * time.Second)
		}
	}()

	// ── Event loop ────────────────────────────────────────────────────────────
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

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

// isServiceInstalled returns true if a LaunchAgent or LaunchDaemon plist exists.
func isServiceInstalled() bool {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", "agent-daemon.plist")); err == nil {
		return true
	}
	if _, err := os.Stat("/Library/LaunchDaemons/agent-daemon.plist"); err == nil {
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
	return true
}

// showInstallDialog presents a native macOS dialog to choose install level.
// Returns the chosen level and true, or false if cancelled.
func showInstallDialog() (internalsvc.InstallLevel, bool) {
	script := `tell application "System Events"
	set choice to display dialog "Choose how Agent Daemon should run in the background:" ¬
		buttons {"Cancel", "System (all users, starts at boot)", "Just for me (login item)"} ¬
		default button "Just for me (login item)" ¬
		with title "Agent Daemon Setup" ¬
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
// a .app bundle for the first time so `agent-daemon` works in the terminal.
func installCLIIfNeeded() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	if !strings.Contains(exePath, ".app/Contents/MacOS/") {
		return
	}

	home, _ := os.UserHomeDir()
	target := "/usr/local/bin/agent-daemon"

	if _, err := os.Lstat(target); err == nil {
		return // already there
	}

	// Try without privileges first (writable on Homebrew setups).
	if os.Symlink(exePath, target) == nil {
		slog.Info("installed CLI to /usr/local/bin/agent-daemon")
		return
	}

	// Ask for admin via macOS password dialog.
	script := fmt.Sprintf(
		`do shell script "ln -sf %q /usr/local/bin/agent-daemon" with administrator privileges`,
		exePath,
	)
	if exec.Command("osascript", "-e", script).Run() == nil {
		slog.Info("installed CLI to /usr/local/bin/agent-daemon (via admin dialog)")
		return
	}

	// Fall back to ~/.local/bin silently.
	localBin := filepath.Join(home, ".local", "bin")
	_ = os.MkdirAll(localBin, 0755)
	if os.Symlink(exePath, filepath.Join(localBin, "agent-daemon")) == nil {
		slog.Info("installed CLI to ~/.local/bin/agent-daemon")
		addToShellProfile(home, localBin)
	}
}

// addToShellProfile appends a PATH export to shell rc files if not already present.
func addToShellProfile(home, dir string) {
	line := fmt.Sprintf("\nexport PATH=\"%s:$PATH\" # added by AgentDaemon\n", dir)
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
	_ = service.Control(svc, "install")
	_ = service.Control(svc, "start")
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
