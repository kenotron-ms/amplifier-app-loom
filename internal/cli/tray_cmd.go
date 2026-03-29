package cli

import (
	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/tray"
)

var trayCmd = &cobra.Command{
	Use:   "tray",
	Short: "Launch the system tray management app",
	Long: `Launch the loom system tray app.

The tray app shows daemon status in the menu bar and lets you:
  - Start / stop / pause the daemon
  - Install or uninstall the service (user or system level)
  - Open the web UI

The tray app communicates with a running daemon via HTTP.
The daemon does not need to be installed as a service to use the tray —
you can also start it manually with 'loom _serve'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		return tray.Run(port)
	},
}

func init() {
	trayCmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	rootCmd.AddCommand(trayCmd)
}
