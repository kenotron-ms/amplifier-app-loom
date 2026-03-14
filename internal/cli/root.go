package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "agent-daemon",
	Short: "Scheduled job execution daemon with web UI",
	Long: `agent-daemon — a cross-platform scheduled job runner.

Runs as a system service (launchd / systemd / Windows Service) with:
  - Cron, interval, and immediate job triggers
  - Web UI at http://localhost:7700
  - Natural language job management via Claude AI
  - Job deduplication and bounded concurrency queue`,
}

func Execute() {
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
		serveCmd, // internal: invoked by service manager
	)
}
