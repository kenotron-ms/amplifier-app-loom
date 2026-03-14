//go:build cgo

package tray

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
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
	icon := makeIcon()
	systray.SetTemplateIcon(icon, icon)
	systray.SetTooltip("agent-daemon")

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
	for {
		select {
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
	svc, _, err := internalsvc.NewService(level)
	if err != nil {
		return
	}
	_ = service.Control(svc, "install")
	_ = service.Control(svc, "start")
}

func uninstallService() {
	// Try both levels
	for _, level := range []internalsvc.InstallLevel{internalsvc.LevelUser, internalsvc.LevelSystem} {
		svc, _, err := internalsvc.NewService(level)
		if err != nil {
			continue
		}
		_ = service.Control(svc, "stop")
		_ = service.Control(svc, "uninstall")
	}
}

func runServiceControl(level internalsvc.InstallLevel, action string) {
	svc, _, err := internalsvc.NewService(level)
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
