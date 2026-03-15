package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ms/agent-daemon/internal/api"
	"github.com/ms/agent-daemon/internal/updater"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update agent-daemon to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Current version: %s\n", api.Version)
		fmt.Println("Checking for updates…")

		latest, downloadURL, err := updater.LatestRelease()
		if err != nil {
			return fmt.Errorf("check for updates: %w", err)
		}

		if !updater.IsNewer(api.Version, latest) {
			fmt.Printf("Already up to date (v%s).\n", api.Version)
			return nil
		}

		fmt.Printf("New version available: v%s — downloading…\n", latest)
		if err := updater.Apply(downloadURL); err != nil {
			return fmt.Errorf("apply update: %w", err)
		}

		fmt.Printf("Updated to v%s. Restart agent-daemon to use the new version.\n", latest)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
