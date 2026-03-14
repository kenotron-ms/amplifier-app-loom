package cli

import (
	"fmt"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"

	internalsvc "github.com/ms/agent-daemon/internal/service"
)

func installLevel(cmd *cobra.Command) internalsvc.InstallLevel {
	sys, _ := cmd.Flags().GetBool("system")
	if sys {
		return internalsvc.LevelSystem
	}
	return internalsvc.LevelUser
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install agent-daemon as a service",
	Long: `Install agent-daemon as a system service.

By default installs as a user-level service (login item / LaunchAgent / systemd --user).
Use --system to install system-wide (starts at boot, requires admin/sudo).`,
	Example: `  agent-daemon install              # user-level (login items)
  sudo agent-daemon install --system  # system-wide (boot daemon)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		level := installLevel(cmd)
		svc, _, err := internalsvc.NewService(level)
		if err != nil {
			return err
		}
		if err := service.Control(svc, "install"); err != nil {
			return fmt.Errorf("install failed: %w\n\nTip: system-level install requires sudo", err)
		}

		levelStr := "user-level (login items)"
		if level == internalsvc.LevelSystem {
			levelStr = "system-level (boot daemon)"
		}
		fmt.Printf("✓ Installed agent-daemon as %s\n", levelStr)
		fmt.Println("  Run 'agent-daemon start' to start it.")
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall agent-daemon service",
	Example: `  agent-daemon uninstall              # remove user-level service
  sudo agent-daemon uninstall --system  # remove system-level service`,
	RunE: func(cmd *cobra.Command, args []string) error {
		level := installLevel(cmd)
		svc, _, err := internalsvc.NewService(level)
		if err != nil {
			return err
		}
		_ = service.Control(svc, "stop") // stop first, ignore error
		if err := service.Control(svc, "uninstall"); err != nil {
			return fmt.Errorf("uninstall failed: %w", err)
		}
		fmt.Println("✓ agent-daemon service removed.")
		return nil
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent-daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		level := installLevel(cmd)
		svc, _, err := internalsvc.NewService(level)
		if err != nil {
			return err
		}
		if err := service.Control(svc, "start"); err != nil {
			return fmt.Errorf("start failed: %w", err)
		}
		fmt.Println("✓ agent-daemon started.")
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the agent-daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		level := installLevel(cmd)
		svc, _, err := internalsvc.NewService(level)
		if err != nil {
			return err
		}
		if err := service.Control(svc, "stop"); err != nil {
			return fmt.Errorf("stop failed: %w", err)
		}
		fmt.Println("✓ agent-daemon stopped.")
		return nil
	},
}

// serveCmd is the internal command invoked by the OS service manager.
var serveCmd = &cobra.Command{
	Use:    "_serve",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return internalsvc.RunDaemon()
	},
}

func init() {
	for _, cmd := range []*cobra.Command{installCmd, uninstallCmd, startCmd, stopCmd} {
		cmd.Flags().Bool("system", false, "Use system-level service (requires admin/sudo)")
	}
}
