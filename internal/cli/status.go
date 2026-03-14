package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/types"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		url := fmt.Sprintf("http://localhost:%d/api/status", port)

		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("● agent-daemon: offline")
			return nil
		}
		defer resp.Body.Close()

		var s types.DaemonStatus
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			return err
		}

		stateIcon := "●"
		if s.State == "paused" {
			stateIcon = "⏸"
		}

		fmt.Printf("%s agent-daemon  [%s]  v%s\n", stateIcon, s.State, s.Version)
		fmt.Printf("  PID:        %d\n", s.PID)
		fmt.Printf("  Uptime:     %s\n", formatDuration(time.Since(s.StartedAt)))
		fmt.Printf("  Jobs:       %d\n", s.JobCount)
		fmt.Printf("  Running:    %d\n", s.ActiveRuns)
		fmt.Printf("  Queued:     %d\n", s.QueueDepth)
		fmt.Printf("  UI:         http://localhost:%d\n", port)
		return nil
	},
}

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause job scheduling",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemonPost(cmd, "/api/daemon/pause", "Scheduler paused.")
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume job scheduling",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemonPost(cmd, "/api/daemon/resume", "Scheduler resumed.")
	},
}

var flushCmd = &cobra.Command{
	Use:   "flush",
	Short: "Flush pending jobs from the queue",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemonPost(cmd, "/api/daemon/flush", "Queue flushed.")
	},
}

func daemonPost(cmd *cobra.Command, path, successMsg string) error {
	port, _ := cmd.Flags().GetInt("port")
	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("daemon returned %d", resp.StatusCode)
	}
	fmt.Println("✓", successMsg)
	return nil
}

func init() {
	for _, cmd := range []*cobra.Command{statusCmd, pauseCmd, resumeCmd, flushCmd} {
		cmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
