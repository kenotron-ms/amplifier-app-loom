package service

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
)

const (
	ServiceName        = "agent-daemon"
	ServiceDisplayName = "Agent Daemon"
	ServiceDescription = "Scheduled job execution daemon with web UI and natural language interface"
)

// InstallLevel controls whether the service is installed for the current user
// (login item / LaunchAgent / systemd --user) or system-wide (root / LaunchDaemon / systemd).
type InstallLevel int

const (
	LevelUser   InstallLevel = iota // default: user-level, no sudo needed
	LevelSystem                     // system-level: starts at boot, needs admin/sudo
)

// Program implements kardianos/service.Interface.
type Program struct {
	daemon *Daemon
}

func (p *Program) Start(s service.Service) error {
	go func() {
		if err := p.daemon.Run(); err != nil {
			slog.Error("daemon exited with error", "err", err)
			os.Exit(1)
		}
	}()
	return nil
}

func (p *Program) Stop(s service.Service) error {
	p.daemon.Shutdown()
	return nil
}

// BuildServiceConfig returns the kardianos service config for the given install level.
func BuildServiceConfig(level InstallLevel) *service.Config {
	exePath, _ := os.Executable()
	// Resolve symlinks so the LaunchAgent plist always points to the real binary
	// inside the .app bundle — this ensures TCC grants to com.ms.agent-daemon apply
	// to the daemon process (same TCC identity as the tray).
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	cfg := &service.Config{
		Name:        ServiceName,
		DisplayName: ServiceDisplayName,
		Description: ServiceDescription,
		Executable:  exePath,
		Arguments:   []string{"_serve"},
	}

	switch runtime.GOOS {
	case "darwin":
		if level == LevelUser {
			// LaunchAgent: starts on login, lives in ~/Library/LaunchAgents
			cfg.Option = service.KeyValue{
				"UserService": true,
				"KeepAlive":   true,
				"RunAtLoad":   false,
			}
		} else {
			// LaunchDaemon: starts at boot, lives in /Library/LaunchDaemons (needs sudo)
			cfg.Option = service.KeyValue{
				"KeepAlive": true,
				"RunAtLoad": false,
			}
		}
	case "linux":
		if level == LevelUser {
			cfg.Option = service.KeyValue{
				"UserService": true,
			}
		}
		// System level: no special option needed; requires running as root
	case "windows":
		// Windows Service always runs system-wide; user-level would need Task Scheduler
		// We default to system-level on Windows regardless.
		_ = level
	}

	return cfg
}

// NewService creates a kardianos service wrapping our daemon.
// Opens the store — only use when the daemon will actually run.
func NewService(level InstallLevel) (service.Service, *Program, error) {
	daemon, err := NewDaemon()
	if err != nil {
		return nil, nil, err
	}
	p := &Program{daemon: daemon}
	svc, err := service.New(p, BuildServiceConfig(level))
	if err != nil {
		return nil, nil, err
	}
	return svc, p, nil
}

// NewServiceForControl creates a kardianos service for install/uninstall/start/stop
// operations only. It does NOT open the database, so it works even while the
// daemon is already running.
func NewServiceForControl(level InstallLevel) (service.Service, error) {
	p := &Program{daemon: nil}
	svc, err := service.New(p, BuildServiceConfig(level))
	if err != nil {
		return nil, err
	}
	return svc, nil
}

// RunDaemon starts the daemon directly (used by _serve sub-command).
func RunDaemon() error {
	daemon, err := NewDaemon()
	if err != nil {
		return err
	}
	return daemon.Run()
}

// DetectInstallLevel checks which level the service is currently installed at.
// Returns LevelUser, LevelSystem, or an error if not installed.
func DetectInstallLevel() (InstallLevel, error) {
	for _, level := range []InstallLevel{LevelUser, LevelSystem} {
		svc, err := NewServiceForControl(level)
		if err != nil {
			continue
		}
		status, err := svc.Status()
		if err == nil && status != service.StatusUnknown {
			return level, nil
		}
	}
	return LevelUser, nil // default
}
