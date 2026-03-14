package service

import (
	"log/slog"
	"os"
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
	// Try user first
	for _, level := range []InstallLevel{LevelUser, LevelSystem} {
		svc, _, err := NewService(level)
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
