package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"net/http"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/index"
	"github.com/ms/amplifier-app-loom/internal/types"
)

// indexCmd is the parent command for GitHub bundle index operations.
var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage the local GitHub bundle index",
	Long: `Scan GitHub for Amplifier bundles and browse the results locally.

The index stores metadata about every repo accessible to your GitHub token
that looks like an Amplifier bundle. Use 'index scan' to populate it and
'index list' to browse what was found.`,
}

// ── index scan ───────────────────────────────────────────────────────────────

var indexScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan GitHub for amplifier bundles",
	Long: `Scan all GitHub repositories accessible to your token and build a local
index of Amplifier bundles.

Token resolution order: GITHUB_TOKEN env var → gh auth token → unauthenticated.

With a token the scan runs at ~50 ms/call; without one it drops to 1.2 s/call
to respect GitHub's anonymous rate limits.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		includeArchived, _ := cmd.Flags().GetBool("include-archived")
		quiet, _ := cmd.Flags().GetBool("quiet")

		dir := index.DefaultDir()
		opts := index.ScanOptions{
			Force:           force,
			IncludeArchived: includeArchived,
			Quiet:           quiet,
		}

		result, err := index.Scan(cmd.Context(), dir, opts)
		if err != nil {
			return err
		}

		if !quiet {
			fmt.Printf("\n✓ Scan complete: %d added, %d updated, %d unchanged (%d API calls remaining)\n",
				len(result.Added), len(result.Updated), result.Unchanged, result.APIRemaining)
			if len(result.Removed) > 0 {
				fmt.Printf("  Removed %d repo(s) no longer accessible.\n", len(result.Removed))
			}
			fmt.Printf("  Run 'loom index list' to browse results.\n")
		}
		return nil
	},
}

// ── index list ───────────────────────────────────────────────────────────────

var indexListCmd = &cobra.Command{
	Use:   "list",
	Short: "List indexed amplifier bundles",
	Long: `Browse the local bundle index, grouped by GitHub organisation.

Shows the primary capability type, a short description, and badges for
private repos (🔒) and star count (★N).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		privateOnly, _ := cmd.Flags().GetBool("private-only")

		dir := index.DefaultDir()
		idx, err := index.LoadIndex(dir)
		if err != nil {
			return err
		}

		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(idx)
		}

		if len(idx.Repos) == 0 {
			fmt.Println("No repos indexed. Run 'loom index scan' first.")
			return nil
		}

		// Group by org (the part before "/" in Remote).
		orgMap := make(map[string][]index.Entry)
		for _, e := range idx.Repos {
			if privateOnly && !e.Private {
				continue
			}
			org := e.Remote
			if i := strings.Index(e.Remote, "/"); i >= 0 {
				org = e.Remote[:i]
			}
			orgMap[org] = append(orgMap[org], e)
		}

		if len(orgMap) == 0 {
			fmt.Println("No repos match the current filters.")
			return nil
		}

		// Sort orgs then entries within each org.
		orgs := make([]string, 0, len(orgMap))
		for org := range orgMap {
			orgs = append(orgs, org)
		}
		sort.Strings(orgs)

		for _, org := range orgs {
			entries := orgMap[org]
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Name < entries[j].Name
			})

			fmt.Printf("%s\n", org)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, e := range entries {
				// Primary capability type.
				capType := ""
				if len(e.Capabilities) > 0 {
					capType = "[" + e.Capabilities[0].Type + "]"
				}

				// Description truncated to 55 runes.
				desc := e.Description
				runes := []rune(desc)
				if len(runes) > 55 {
					desc = string(runes[:55]) + "..."
				}

				// Badges: 🔒 for private, ★N for stars.
				var badges []string
				if e.Private {
					badges = append(badges, "🔒")
				}
				if e.Stars > 0 {
					badges = append(badges, fmt.Sprintf("★%d", e.Stars))
				}
				badge := strings.Join(badges, " ")

				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", e.Name, capType, desc, badge)
			}
			w.Flush() //nolint:errcheck
		}
		return nil
	},
}

// ── index status ─────────────────────────────────────────────────────────────

var indexStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index store statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := index.DefaultDir()

		idx, err := index.LoadIndex(dir)
		if err != nil {
			return err
		}
		st, err := index.LoadState(dir)
		if err != nil {
			return err
		}

		indexed := len(idx.Repos)
		scanned := len(st.Repos)
		skipped := scanned - indexed
		if skipped < 0 {
			skipped = 0
		}

		// Tally capabilities by type.
		typeCounts := make(map[string]int)
		totalCaps := 0
		for _, e := range idx.Repos {
			for _, c := range e.Capabilities {
				typeCounts[c.Type]++
				totalCaps++
			}
		}

		// Build a sorted, count-descending summary string.
		typeNames := make([]string, 0, len(typeCounts))
		for t := range typeCounts {
			typeNames = append(typeNames, t)
		}
		sort.Slice(typeNames, func(i, j int) bool {
			if typeCounts[typeNames[i]] != typeCounts[typeNames[j]] {
				return typeCounts[typeNames[i]] > typeCounts[typeNames[j]]
			}
			return typeNames[i] < typeNames[j]
		})
		capsDetail := ""
		if len(typeNames) > 0 {
			parts := make([]string, 0, len(typeNames))
			for _, t := range typeNames {
				parts = append(parts, fmt.Sprintf("%d %ss", typeCounts[t], t))
			}
			capsDetail = " (" + strings.Join(parts, ", ") + ")"
		}

		// Substitute ~ for home directory in the display path.
		homeDir, _ := os.UserHomeDir()
		displayDir := strings.Replace(dir, homeDir, "~", 1)

		// Format last scan timestamp.
		lastScan := "never"
		if idx.LastScan != "" {
			if t, e := time.Parse(time.RFC3339, idx.LastScan); e == nil {
				lastScan = t.Local().Format("Jan 2, 2006 3:04 PM")
			}
		}

		fmt.Printf("GitHub Bundle Index\n")
		fmt.Printf("  Store:     %s\n", displayDir)
		fmt.Printf("  Repos:     %d indexed (%d scanned, %d skipped)\n", indexed, scanned, skipped)
		fmt.Printf("  Caps:      %d%s\n", totalCaps, capsDetail)
		fmt.Printf("  Last scan: %s\n", lastScan)

		if st.RateLimit != nil && st.RateLimit.ResetAt > 0 {
			resetTime := time.Unix(st.RateLimit.ResetAt, 0).Local().Format("3:04 PM")
			fmt.Printf("  Rate:      %d remaining (resets %s)\n", st.RateLimit.Remaining, resetTime)
		} else {
			fmt.Printf("  Rate:      unknown\n")
		}

		return nil
	},
}

// ── index watch ──────────────────────────────────────────────────────────────

var indexWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Schedule periodic background scans via the loom daemon",
	Long: `Creates a loom loop job that runs 'loom index scan --quiet' on the given
interval. Requires the loom daemon to be running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		every, _ := cmd.Flags().GetString("every")

		execPath, err := os.Executable()
		if err != nil {
			execPath = "loom"
		}

		job := types.Job{
			Name: "amplifier-bundle-index",
			Trigger: types.Trigger{
				Type:     types.TriggerLoop,
				Schedule: every,
			},
			Executor: types.ExecutorShell,
			Shell: &types.ShellConfig{
				Command: execPath + " index scan --quiet",
			},
			Enabled: true,
		}

		body, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("failed to encode job: %w", err)
		}

		resp, err := http.Post(
			fmt.Sprintf("http://localhost:%d/api/jobs", port),
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var e map[string]string
			json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
			return fmt.Errorf("error: %s", e["error"])
		}

		fmt.Printf("✓ Scheduled background scan every %s\n", every)
		fmt.Printf("  Run 'loom index unwatch' to remove the schedule.\n")
		return nil
	},
}

// ── index unwatch ─────────────────────────────────────────────────────────────

var indexUnwatchCmd = &cobra.Command{
	Use:   "unwatch",
	Short: "Remove the periodic scan schedule",
	Long:  `Finds the loom job named "amplifier-bundle-index" and deletes it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")

		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/jobs", port))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var jobs []*types.Job
		if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
			return err
		}

		var jobID string
		for _, j := range jobs {
			if j.Name == "amplifier-bundle-index" {
				jobID = j.ID
				break
			}
		}
		if jobID == "" {
			fmt.Println("No active watch job found (nothing to remove).")
			return nil
		}

		req, _ := http.NewRequest(http.MethodDelete,
			fmt.Sprintf("http://localhost:%d/api/jobs/%s", port, jobID), nil)
		delResp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer delResp.Body.Close()

		if delResp.StatusCode >= 400 {
			return fmt.Errorf("error removing watch job (status %d)", delResp.StatusCode)
		}

		fmt.Println("✓ Background scan schedule removed.")
		return nil
	},
}

// ── index init ────────────────────────────────────────────────────────────────

var indexInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create sources.json to configure which repos to scan",
	Long: `Creates ~/.amplifier/bundle-index/sources.json — a local, non-git-tracked
config that tells 'loom index scan' which GitHub repos to sweep.

Seeded with your own GitHub handle (for private repo access).
Edit the file to add team feeds or extra repos.

Edit the file to add more handles or specific repos.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := index.DefaultDir()

		existing, err := index.LoadSources(dir)
		if err != nil {
			return err
		}
		if len(existing.TeamFeeds) > 0 || len(existing.ExtraHandles) > 0 || len(existing.ExtraRepos) > 0 {
			fmt.Printf("sources.json already exists at:\n  %s/sources.json\n\n", dir)
			for _, f := range existing.TeamFeeds {
				fmt.Printf("  team feed:  %s\n", f.Name)
			}
			if len(existing.ExtraHandles) > 0 {
				fmt.Printf("  handles:    %v\n", existing.ExtraHandles)
			}
			if len(existing.ExtraRepos) > 0 {
				fmt.Printf("  repos:      %v\n", existing.ExtraRepos)
			}
			fmt.Printf("\nEdit directly to add more sources:\n  %s/sources.json\n", dir)
			return nil
		}

		own := ""
		if out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output(); err == nil {
			own = strings.TrimSpace(string(out))
		}

		src := &index.Sources{}
		if own != "" {
			src.ExtraHandles = []string{own}
		}

		if err := index.SaveSources(dir, src); err != nil {
			return err
		}

		fmt.Printf("\u2713 Created %s/sources.json\n\n", dir)
		if own != "" {
			fmt.Printf("Configured:\n  Handle: %s (your private repos)\n", own)
		}
		fmt.Printf("\nAdd team feeds by editing:\n  %s/sources.json\n\n", dir)
		fmt.Printf("Example team_feeds entry:\n")
		fmt.Printf("  {\n    \"name\": \"My team\",\n    \"url\": \"https://raw.githubusercontent.com/org/repo/main/team.json\"\n  }\n\n")
		fmt.Printf("Then run:\n  loom index scan\n")
		return nil
	},
}

// ── init ──────────────────────────────────────────────────────────────────────

func init() {
	// Port flag for daemon-backed commands.
	for _, cmd := range []*cobra.Command{indexWatchCmd, indexUnwatchCmd} {
		cmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	}

	// scan flags
	indexScanCmd.Flags().Int("limit", 0, "Max repos to scan (0 = all)")
	indexScanCmd.Flags().Bool("force", false, "Re-scan even if pushed_at is unchanged")
	indexScanCmd.Flags().Bool("include-archived", false, "Include archived repos")
	indexScanCmd.Flags().BoolP("quiet", "q", false, "Suppress progress output")

	// list flags
	indexListCmd.Flags().Bool("json", false, "Output the full index as JSON")
	indexListCmd.Flags().Bool("private-only", false, "Show only private repos")

	// watch flags
	indexWatchCmd.Flags().String("every", "2h", "Scan interval (e.g. 30m, 2h)")

	indexCmd.AddCommand(
		indexInitCmd,
		indexScanCmd,
		indexListCmd,
		indexStatusCmd,
		indexWatchCmd,
		indexUnwatchCmd,
	)
}
