package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/types"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete all disabled jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		yes, _ := cmd.Flags().GetBool("yes")

		// Fetch disabled jobs
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/jobs", port))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var jobs []*types.Job
		if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
			return err
		}

		var disabled []*types.Job
		for _, j := range jobs {
			if !j.Enabled {
				disabled = append(disabled, j)
			}
		}

		if len(disabled) == 0 {
			fmt.Println("Nothing to prune — no disabled jobs.")
			return nil
		}

		fmt.Printf("Found %d disabled job(s):\n", len(disabled))
		for _, j := range disabled {
			fmt.Printf("  • %s (%s)\n", j.Name, j.ID[:8])
		}

		if dryRun {
			fmt.Println("\nDry run — nothing deleted.")
			return nil
		}

		if !yes {
			fmt.Printf("\nDelete %d job(s)? [y/N] ", len(disabled))
			var answer string
			fmt.Scanln(&answer)
			if answer != "y" && answer != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		// Hit the prune endpoint
		pruneResp, err := http.Post(
			fmt.Sprintf("http://localhost:%d/api/jobs/prune", port),
			"application/json", nil,
		)
		if err != nil {
			return fmt.Errorf("prune failed: %w", err)
		}
		defer pruneResp.Body.Close()

		var result struct {
			Deleted int `json:"deleted"`
		}
		json.NewDecoder(pruneResp.Body).Decode(&result)
		fmt.Printf("✓ Pruned %d disabled job(s).\n", result.Deleted)
		return nil
	},
}

func init() {
	pruneCmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	pruneCmd.Flags().Bool("dry-run", false, "Show what would be deleted without deleting")
	pruneCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}
