package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/mirror"
)

// connectorCmd is the parent command for connector management.
var connectorCmd = &cobra.Command{
	Use:   "connector",
	Short: "Manage connectors (data sources for the mirror system)",
	Long: `Connectors watch external data sources and mirror them locally.
When a connector detects a change, it fires linked jobs.

Use 'connector add' to create a new connector, 'connector list' to see all,
and 'mirror entities' / 'mirror changes' to inspect mirrored data.`,
}

// ── connector list ───────────────────────────────────────────────────────────

var connectorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all connectors",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		apiURL := fmt.Sprintf("http://localhost:%d/api/mirror/connectors", port)

		resp, err := http.Get(apiURL)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var conns []*mirror.Connector
		if err := json.NewDecoder(resp.Body).Decode(&conns); err != nil {
			return err
		}

		if len(conns) == 0 {
			fmt.Println("No connectors configured.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tMETHOD\tENTITY\tINTERVAL\tHEALTH\tENABLED")
		for _, c := range conns {
			id := c.ID
			if len(id) > 8 {
				id = id[:8]
			}
			enabled := "yes"
			if !c.Enabled {
				enabled = "no"
			}
			health := string(c.Health)
			if health == "" {
				health = "unknown"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				id, c.Name, c.FetchMethod, c.EntityAddress, c.Interval, health, enabled)
		}
		return w.Flush()
	},
}

// ── connector add ────────────────────────────────────────────────────────────

var connectorAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new connector",
	Example: `  # Monitor a GitHub PR via CLI
  loom connector add --name "PR #42" \
    --method command --command "gh api /repos/owner/repo/pulls/42" \
    --entity "github.pr/owner/repo/42" --interval 60s

  # Monitor an API endpoint
  loom connector add --name "Stock Price" \
    --method http --url "https://api.example.com/price/AAPL" \
    --entity "stock.price/AAPL" --interval 5m

  # Monitor a web page via browser
  loom connector add --name "Amazon AirPods" \
    --method browser --url "https://amazon.com/dp/B09V3KXJPB" \
    --site amazon --entity "amazon.product/B09V3KXJPB" \
    --prompt "Extract price and availability" --interval 15m`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")

		name, _ := cmd.Flags().GetString("name")
		method, _ := cmd.Flags().GetString("method")
		command, _ := cmd.Flags().GetString("command")
		url, _ := cmd.Flags().GetString("url")
		site, _ := cmd.Flags().GetString("site")
		entity, _ := cmd.Flags().GetString("entity")
		interval, _ := cmd.Flags().GetString("interval")
		prompt, _ := cmd.Flags().GetString("prompt")

		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if entity == "" {
			return fmt.Errorf("--entity is required (e.g. github.pr/owner/repo/42)")
		}
		if method == "" {
			return fmt.Errorf("--method is required (command, http, or browser)")
		}

		conn := mirror.Connector{
			Name:          name,
			FetchMethod:   mirror.FetchMethod(method),
			Command:       command,
			URL:           url,
			Site:          site,
			EntityAddress: entity,
			Interval:      interval,
			Prompt:        prompt,
			Enabled:       true,
		}

		body, err := json.Marshal(conn)
		if err != nil {
			return err
		}

		apiURL := fmt.Sprintf("http://localhost:%d/api/mirror/connectors", port)
		resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var errResp map[string]string
			json.NewDecoder(resp.Body).Decode(&errResp)
			return fmt.Errorf("error: %s", errResp["error"])
		}

		var created mirror.Connector
		json.NewDecoder(resp.Body).Decode(&created)
		id := created.ID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Printf("\u2713 Connector created: %s (id: %s)\n", created.Name, id)
		return nil
	},
}

// ── connector remove ─────────────────────────────────────────────────────────

var connectorRemoveCmd = &cobra.Command{
	Use:     "remove <connector-id>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a connector by ID (or ID prefix)",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		prefix := args[0]

		// Resolve prefix by listing connectors
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/mirror/connectors", port))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var conns []*mirror.Connector
		json.NewDecoder(resp.Body).Decode(&conns)

		var match *mirror.Connector
		for _, c := range conns {
			if c.ID == prefix || (len(prefix) >= 4 && len(c.ID) >= len(prefix) && c.ID[:len(prefix)] == prefix) {
				if match != nil {
					return fmt.Errorf("ambiguous prefix '%s' matches multiple connectors", prefix)
				}
				match = c
			}
		}
		if match == nil {
			return fmt.Errorf("no connector found matching '%s'", prefix)
		}

		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://localhost:%d/api/mirror/connectors/%s", port, match.ID), nil)
		delResp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer delResp.Body.Close()

		fmt.Printf("\u2713 Deleted connector '%s'\n", match.Name)
		return nil
	},
}

func init() {
	for _, cmd := range []*cobra.Command{connectorListCmd, connectorAddCmd, connectorRemoveCmd} {
		cmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	}

	connectorAddCmd.Flags().String("name", "", "Connector name (required)")
	connectorAddCmd.Flags().String("method", "", "Fetch method: command, http, or browser (required)")
	connectorAddCmd.Flags().String("command", "", "Shell command (for --method command)")
	connectorAddCmd.Flags().String("url", "", "URL to fetch (for --method http or browser)")
	connectorAddCmd.Flags().String("site", "", "Site name for browser profile grouping")
	connectorAddCmd.Flags().String("entity", "", "Entity address, e.g. github.pr/owner/repo/42 (required)")
	connectorAddCmd.Flags().String("interval", "5m", "Sync interval (e.g. 30s, 5m, 1h)")
	connectorAddCmd.Flags().String("prompt", "", "What to extract (natural language or JS expression)")

	connectorCmd.AddCommand(connectorListCmd, connectorAddCmd, connectorRemoveCmd)
}