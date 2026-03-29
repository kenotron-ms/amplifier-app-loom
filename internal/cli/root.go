package cli

import (
	"fmt"
	"os"

	"github.com/ms/amplifier-app-loom/internal/api"
	"github.com/ms/amplifier-app-loom/internal/updater"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "loom",
	Version: api.Version,
	Short:   "Scheduled job execution daemon with web UI",
	Long: `loom — a cross-platform scheduled job runner.

Runs as a system service (launchd / systemd / Windows Service) with:
  - Cron, interval, and immediate job triggers
  - Web UI at http://localhost:7700
  - Natural language job management via Claude AI
  - Job deduplication and bounded concurrency queue`,
}

func Execute() {
	// Remove any <exe>.old leftover from a previous auto-update.
	updater.CleanupOldBinary()

	// When launched as a macOS .app bundle, macOS sets __CFBundleIdentifier
	// in the environment. In that case, default to the tray command.
	if os.Getenv("__CFBundleIdentifier") != "" && len(os.Args) == 1 {
		os.Args = append(os.Args, "tray")
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(
		installCmd,
		uninstallCmd,
		startCmd,
		stopCmd,
		statusCmd,
		pauseCmd,
		resumeCmd,
		flushCmd,
		listCmd,
		addCmd,
		removeCmd,
		pruneCmd,
		configCmd,
		serveCmd,      // internal: invoked by service manager
		mirrorCmd,     // mirror subcommands: entities, get, changes, connectors
		connectorCmd,  // connector subcommands: add, list, remove
	)
}
