package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/mirror"
)

// mirrorCmd is the parent command for mirror subcommands.
var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "Inspect the mirror (digital twin) data",
	Long:  `Commands to read connector status, entity snapshots, and change history from the mirror system.`,
}

// ── mirror entities ──────────────────────────────────────────────────────────

var mirrorEntitiesCmd = &cobra.Command{
	Use:   "entities [kind-prefix]",
	Short: "List mirrored entities",
	Long: `List all entities in the mirror, optionally filtered by kind prefix.

Examples:
  loom mirror entities                     # list all
  loom mirror entities github.pr/          # list all GitHub PRs
  loom mirror entities amazon.product/     # list all Amazon products`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		kind := ""
		if len(args) > 0 {
			kind = args[0]
		}

		apiURL := fmt.Sprintf("http://localhost:%d/api/mirror/entities", port)
		if kind != "" {
			apiURL += "?kind=" + url.QueryEscape(kind)
		}

		resp, err := http.Get(apiURL)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var entities []*mirror.Entity
		if err := json.NewDecoder(resp.Body).Decode(&entities); err != nil {
			return err
		}

		if len(entities) == 0 {
			fmt.Println("No mirrored entities.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ADDRESS\tDATA SIZE")
		for _, e := range entities {
			fmt.Fprintf(w, "%s\t%d bytes\n", e.Address, len(e.Data))
		}
		return w.Flush()
	},
}

// ── mirror get ───────────────────────────────────────────────────────────────

var mirrorGetCmd = &cobra.Command{
	Use:   "get <entity-address>",
	Short: "Get the current snapshot of an entity",
	Long: `Retrieve the full JSON snapshot of a mirrored entity.

Examples:
  loom mirror get github.pr/owner/repo/42
  loom mirror get amazon.product/B09V3KXJPB`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		address := args[0]

		apiURL := fmt.Sprintf("http://localhost:%d/api/mirror/entities/%s", port, address)
		resp, err := http.Get(apiURL)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("entity %s not found", address)
		}

		var result json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return err
		}

		// Pretty print
		pretty, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Println(string(result))
			return nil
		}
		fmt.Println(string(pretty))
		return nil
	},
}

// ── mirror changes ───────────────────────────────────────────────────────────

var mirrorChangesCmd = &cobra.Command{
	Use:   "changes [entity-address]",
	Short: "List recent changes detected by connectors",
	Long: `List recent change records, optionally filtered by entity address.

Examples:
  loom mirror changes                              # all recent changes
  loom mirror changes github.pr/owner/repo/42     # changes for one entity
  loom mirror changes --limit 10                   # last 10 changes`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		limit, _ := cmd.Flags().GetInt("limit")
		address := ""
		if len(args) > 0 {
			address = args[0]
		}

		apiURL := fmt.Sprintf("http://localhost:%d/api/mirror/changes?limit=%d", port, limit)
		if address != "" {
			apiURL += "&address=" + url.QueryEscape(address)
		}

		resp, err := http.Get(apiURL)
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		var changes []*mirror.ChangeRecord
		if err := json.NewDecoder(resp.Body).Decode(&changes); err != nil {
			return err
		}

		if len(changes) == 0 {
			fmt.Println("No changes recorded.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIMESTAMP\tENTITY\tVERSION\tCONNECTOR")
		for _, c := range changes {
			fmt.Fprintf(w, "%s\t%s\tv%d\t%s\n",
				c.Timestamp.Format(time.RFC3339),
				c.Address,
				c.Version,
				c.ConnectorID[:8],
			)
		}
		return w.Flush()
	},
}

// ── mirror connectors ────────────────────────────────────────────────────────

var mirrorConnectorsCmd = &cobra.Command{
	Use:   "connectors",
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
		fmt.Fprintln(w, "ID\tNAME\tMETHOD\tENTITY\tHEALTH\tENABLED")
		for _, c := range conns {
			id := c.ID
			if len(id) > 8 {
				id = id[:8]
			}
			enabled := "yes"
			if !c.Enabled {
				enabled = "no"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", id, c.Name, c.FetchMethod, c.EntityAddress, c.Health, enabled)
		}
		return w.Flush()
	},
}

func init() {
	for _, cmd := range []*cobra.Command{mirrorEntitiesCmd, mirrorGetCmd, mirrorChangesCmd, mirrorConnectorsCmd} {
		cmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	}
	mirrorChangesCmd.Flags().Int("limit", 50, "Maximum number of changes to show")

	mirrorCmd.AddCommand(mirrorEntitiesCmd, mirrorGetCmd, mirrorChangesCmd, mirrorConnectorsCmd)
}