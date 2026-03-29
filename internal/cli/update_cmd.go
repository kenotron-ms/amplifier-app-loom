package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/api"
	"github.com/ms/amplifier-app-loom/internal/updater"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update loom to the latest release",
	Long: `Download the latest release from GitHub, verify its checksum, stop and
uninstall the current service, atomically swap the binary, reinstall and start
the service, then exit.

The tray app (if running) will need to be relaunched separately after update.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Current version: v%s\n", api.Version)
		fmt.Println("Checking for updates…")

		u := updater.New(api.Version, func(s updater.State, ver string) {
			switch s {
			case updater.StateChecking:
				// already printed above
			case updater.StateDownloading:
				fmt.Printf("New version v%s found — downloading…\n", ver)
			case updater.StateReady:
				fmt.Printf("Download complete — applying update to v%s…\n", ver)
			case updater.StateApplying:
				fmt.Println("Stopping service, swapping binary, reinstalling…")
			case updater.StateFailed:
				// error is returned below
			}
		})

		if err := u.CheckAndStage(context.Background()); err != nil {
			return fmt.Errorf("update check/download: %w", err)
		}

		switch u.State() {
		case updater.StateUpToDate:
			fmt.Printf("Already up to date (v%s).\n", api.Version)
			return nil
		case updater.StateReady:
			// Apply: stop service → swap binary → reinstall service.
			// Pass empty reExecSubcmd so we return instead of exec-ing;
			// the daemon will be restarted by the service manager, and the
			// user should relaunch the tray manually (or it auto-restarts
			// if configured as a login item).
			if err := u.Apply(""); err != nil {
				return fmt.Errorf("apply update: %w", err)
			}
			fmt.Printf("\n✓ Updated to v%s. The daemon has been restarted.\n", u.LatestVersion())
			fmt.Println("  If the tray app is running, quit and relaunch it.")
			return nil
		default:
			return fmt.Errorf("unexpected updater state: %s", u.State())
		}
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
