package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/platform"
	internalsvc "github.com/ms/agent-daemon/internal/service"
	"github.com/ms/agent-daemon/internal/store"
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
		svc, err := internalsvc.NewServiceForControl(level)
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

		// Absorb API keys and capture installing user's context into the DB.
		fmt.Println("\nConfiguring AI assistant keys...")
		s, err := store.Open(platform.DBPath())
		if err == nil {
			// Capture user identity now — we're running in the user's shell
			// session so $HOME, $SHELL, and $USER are correct and complete.
			if uc := captureUserContext(); uc != nil {
				if cfg, cerr := s.GetConfig(context.Background()); cerr == nil {
					cfg.UserContext = uc
					_ = s.SaveConfig(context.Background(), cfg)
					fmt.Printf("  \u2713 Captured user context (home: %s, shell: %s)\n", uc.HomeDir, uc.Shell)
				}
			}
			absorbed, _ := absorbEnvKeys(s)
			s.Close()
			if absorbed == 0 {
				hasEnv := os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != ""
				if level == internalsvc.LevelSystem && !hasEnv {
					fmt.Println("\n  ⚠  No API keys found in environment.")
					fmt.Println("  The AI assistant will not work until a key is configured.")
					fmt.Println("  Options:")
					fmt.Println("    1. Re-run with sudo -E to preserve env vars:")
					fmt.Println("         sudo -E agent-daemon install --system")
					fmt.Println("    2. After starting, open http://localhost:7700 → Settings")
					fmt.Println("    3. Run:  sudo -E agent-daemon config absorb-env")
				} else if absorbed == 0 {
					fmt.Println("  No API keys in environment — configure via http://localhost:7700 → Settings")
				}
			}
		}

		fmt.Println("\n  Run 'agent-daemon start' to start it.")
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
		svc, err := internalsvc.NewServiceForControl(level)
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
		svc, err := internalsvc.NewServiceForControl(level)
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
		svc, err := internalsvc.NewServiceForControl(level)
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

// captureUserContext records the identity of the installing user so the daemon
// can recreate a proper shell environment when spawning jobs.
//
// For user-level installs this is simply the current process user.
// For system-level installs (sudo), $SUDO_USER identifies the real person who
// ran sudo — we always want that user, not root, because amplifier/claude auth
// tokens and configs live under their home directory.
func captureUserContext() *config.UserContext {
	var u *user.User
	var err error

	// Prefer the real invoking user when running under sudo.
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err = user.Lookup(sudoUser)
	}
	if u == nil {
		u, err = user.Current()
	}
	if err != nil || u == nil {
		return nil
	}

	return &config.UserContext{
		HomeDir:  u.HomeDir,
		Username: u.Username,
		Shell:    lookupUserShell(u.Username),
		UID:      u.Uid,
	}
}

// lookupUserShell returns the login shell for username.
// On macOS it queries Directory Services; on other platforms it parses
// /etc/passwd. Falls back to /bin/zsh (macOS) or /bin/bash (Linux).
func lookupUserShell(username string) string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("dscl", ".", "-read",
			"/Users/"+username, "UserShell").Output()
		if err == nil {
			// Output: "UserShell: /bin/zsh\n"
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "UserShell:") {
					if parts := strings.Fields(line); len(parts) >= 2 {
						return parts[1]
					}
				}
			}
		}
	}

	// Linux / fallback: parse /etc/passwd
	// Format: username:password:uid:gid:gecos:home:shell
	if data, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) >= 7 && fields[0] == username {
				return fields[6]
			}
		}
	}

	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/bash"
}

func init() {
	for _, cmd := range []*cobra.Command{installCmd, uninstallCmd, startCmd, stopCmd} {
		cmd.Flags().Bool("system", false, "Use system-level service (requires admin/sudo)")
	}
}
