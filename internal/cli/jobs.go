package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/types"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		url := fmt.Sprintf("http://localhost:%d/api/jobs", port)

		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var jobs []*types.Job
		if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
			return err
		}

		if len(jobs) == 0 {
			fmt.Println("No jobs configured.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tTRIGGER\tSCHEDULE\tENABLED\tCREATED")
		for _, j := range jobs {
			enabled := "yes"
			if !j.Enabled {
				enabled = "no"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				j.ID[:8],
				j.Name,
				string(j.Trigger.Type),
				j.Trigger.Schedule,
				enabled,
				j.CreatedAt.Format(time.RFC3339),
			)
		}
		return w.Flush()
	},
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new job",
	Example: `  # Add a cron job
  agent-daemon add --name "Daily cleanup" --trigger cron --schedule "0 0 2 * * *" --command "find /tmp -mtime +7 -delete"

  # Add a loop job (every 5 minutes)
  agent-daemon add --name "Health check" --trigger loop --schedule 5m --command "curl -sf http://localhost:8080/health"

  # Add a once job (runs immediately, then auto-disables)
  agent-daemon add --name "Migrate DB" --trigger once --command "/usr/local/bin/migrate.sh"

  # Add a once job with a delay
  agent-daemon add --name "Delayed task" --trigger once --schedule 10m --command "/usr/local/bin/cleanup.sh"

`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")

		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("description")
		triggerType, _ := cmd.Flags().GetString("trigger")
		schedule, _ := cmd.Flags().GetString("schedule")
		command, _ := cmd.Flags().GetString("command")
		cwd, _ := cmd.Flags().GetString("cwd")
		timeout, _ := cmd.Flags().GetString("timeout")
		retries, _ := cmd.Flags().GetInt("retries")

		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if command == "" {
			return fmt.Errorf("--command is required")
		}
		if triggerType == "" {
			triggerType = "immediate"
		}

		job := types.Job{
			Name:        name,
			Description: desc,
			Trigger: types.Trigger{
				Type:     types.TriggerType(triggerType),
				Schedule: schedule,
			},
			Command:    command,
			CWD:        cwd,
			Timeout:    timeout,
			MaxRetries: retries,
			Enabled:    true,
		}

		body, _ := json.Marshal(job)
		url := fmt.Sprintf("http://localhost:%d/api/jobs", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var errResp map[string]string
			json.NewDecoder(resp.Body).Decode(&errResp)
			return fmt.Errorf("error: %s", errResp["error"])
		}

		var created types.Job
		json.NewDecoder(resp.Body).Decode(&created)
		fmt.Printf("✓ Job created: %s (id: %s)\n", created.Name, created.ID)
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove <job-id>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a job by ID (or ID prefix)",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		prefix := args[0]

		// Resolve prefix to full ID
		id, name, err := resolveJobID(port, prefix)
		if err != nil {
			return err
		}

		if confirm, _ := cmd.Flags().GetBool("yes"); !confirm {
			fmt.Printf("Delete job '%s' (%s)? [y/N] ", name, id)
			var answer string
			fmt.Scanln(&answer)
			if answer != "y" && answer != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://localhost:%d/api/jobs/%s", port, id), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("error removing job")
		}
		fmt.Printf("✓ Deleted job '%s'\n", name)
		return nil
	},
}

func resolveJobID(port int, prefix string) (id, name string, err error) {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/jobs", port))
	if err != nil {
		return "", "", fmt.Errorf("daemon not reachable: %w", err)
	}
	defer resp.Body.Close()

	var jobs []*types.Job
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return "", "", err
	}

	var matches []*types.Job
	for _, j := range jobs {
		if j.ID == prefix || (len(prefix) >= 4 && len(j.ID) >= len(prefix) && j.ID[:len(prefix)] == prefix) {
			matches = append(matches, j)
		}
	}

	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("no job found matching '%s'", prefix)
	case 1:
		return matches[0].ID, matches[0].Name, nil
	default:
		return "", "", fmt.Errorf("ambiguous prefix '%s' matches %d jobs; use more characters", prefix, len(matches))
	}
}

func init() {
	for _, cmd := range []*cobra.Command{listCmd, addCmd, removeCmd} {
		cmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	}

	addCmd.Flags().String("name", "", "Job name (required)")
	addCmd.Flags().String("description", "", "Job description")
	addCmd.Flags().String("trigger", "once", "Trigger type: cron, loop, once")
	addCmd.Flags().String("schedule", "", "Cron expression or duration (e.g. 5m)")
	addCmd.Flags().String("command", "", "Shell command to run (required)")
	addCmd.Flags().String("cwd", "", "Working directory")
	addCmd.Flags().String("timeout", "", "Max execution time (e.g. 30s, 5m)")
	addCmd.Flags().Int("retries", 0, "Number of retries on failure")

	removeCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}
