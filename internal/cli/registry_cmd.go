package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/spf13/cobra"
)

// registryCmd is the parent command for browsing the bundle registry.
var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Browse the Amplifier bundle registry",
	Long: `Search and browse the Amplifier bundle registry — both the public community
registry and your private local index (from 'loom index scan').

Use 'registry list' to browse, 'registry search' to filter by keyword,
and 'registry show' to inspect a specific bundle. To install a bundle
found in the registry, use 'loom bundle add <install-spec>'.`,
}

// registryEntry is the minimal shape we need from the registry JSON.
type registryEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Category    string   `json:"category"`
	Author      string   `json:"author"`
	Repo        string   `json:"repo"`
	Install     string   `json:"install"`
	Rating      *float64 `json:"rating"`
	Stars       *int     `json:"stars"`
	Tags        []string `json:"tags"`
	Featured    bool     `json:"featured"`
	LastUpdated string   `json:"lastUpdated"`
	LLMVerdict  string   `json:"llmVerdict"`
	Private     bool     `json:"private"`
	LocalPath   string   `json:"localPath"`
}

// ── registry list ─────────────────────────────────────────────────────────────

var registryListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List bundles in the registry",
	Example: `  loom registry list
  loom registry list --search superpowers
  loom registry list --type agent --category dev
  loom registry list --private`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _     := cmd.Flags().GetInt("port")
		search, _   := cmd.Flags().GetString("search")
		typeF, _    := cmd.Flags().GetString("type")
		catF, _     := cmd.Flags().GetString("category")
		privateOnly, _ := cmd.Flags().GetBool("private")
		asJSON, _   := cmd.Flags().GetBool("json")

		entries, err := fetchAllRegistry(port)
		if err != nil {
			return err
		}

		// Filter
		q := strings.ToLower(search)
		filtered := entries[:0]
		for _, e := range entries {
			if privateOnly && !e.Private {
				continue
			}
			if typeF != "" && !strings.EqualFold(e.Type, typeF) {
				continue
			}
			if catF != "" && !strings.EqualFold(e.Category, catF) {
				continue
			}
			if q != "" {
				haystack := strings.ToLower(e.Name + " " + e.Description + " " + strings.Join(e.Tags, " "))
				if !strings.Contains(haystack, q) {
					continue
				}
			}
			filtered = append(filtered, e)
		}

		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(filtered)
		}

		if len(filtered) == 0 {
			fmt.Println("No registry entries match your filters.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tAUTHOR\tDESCRIPTION")
		for _, e := range filtered {
			desc := e.Description
			if len(desc) > 55 {
				desc = desc[:52] + "..."
			}
			lock := ""
			if e.Private {
				lock = " 🔒"
			}
			stars := ""
			if e.Stars != nil && *e.Stars > 0 {
				stars = fmt.Sprintf(" ★%d", *e.Stars)
			}
			fmt.Fprintf(w, "%s%s%s\t[%s]\t%s\t%s\n",
				e.Name, lock, stars, e.Type, e.Namespace, desc)
		}
		_ = w.Flush()
		fmt.Printf("\n%d bundle(s) shown", len(filtered))
		pub  := 0
		priv := 0
		for _, e := range filtered {
			if e.Private { priv++ } else { pub++ }
		}
		if priv > 0 {
			fmt.Printf(" (%d public, %d private 🔒)", pub, priv)
		}
		fmt.Println()
		return nil
	},
}

// ── registry search ───────────────────────────────────────────────────────────

var registrySearchCmd = &cobra.Command{
	Use:     "search <query>",
	Aliases: []string{"find"},
	Short:   "Search the registry by name, description, or tags",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		q := strings.ToLower(args[0])

		entries, err := fetchAllRegistry(port)
		if err != nil {
			return err
		}

		filtered := entries[:0]
		for _, e := range entries {
			haystack := strings.ToLower(e.Name + " " + e.Description + " " + strings.Join(e.Tags, " "))
			if strings.Contains(haystack, q) {
				filtered = append(filtered, e)
			}
		}

		if len(filtered) == 0 {
			fmt.Printf("No bundles matching %q\n", args[0])
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tAUTHOR\tDESCRIPTION")
		for _, e := range filtered {
			desc := e.Description
			if len(desc) > 55 {
				desc = desc[:52] + "..."
			}
			lock := ""
			if e.Private {
				lock = " 🔒"
			}
			fmt.Fprintf(w, "%s%s\t[%s]\t%s\t%s\n", e.Name, lock, e.Type, e.Namespace, desc)
		}
		_ = w.Flush()
		fmt.Printf("\n%d result(s) for %q\n", len(filtered), args[0])
		return nil
	},
}

// ── registry show ─────────────────────────────────────────────────────────────

var registryShowCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"info", "get"},
	Short:   "Show details for a specific bundle",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		id       := strings.ToLower(args[0])

		entries, err := fetchAllRegistry(port)
		if err != nil {
			return err
		}

		// Find by id, name, or partial match
		var match *registryEntry
		for i := range entries {
			e := &entries[i]
			if strings.EqualFold(e.ID, id) || strings.EqualFold(e.Name, id) {
				match = e
				break
			}
		}
		if match == nil {
			// Try prefix match
			for i := range entries {
				e := &entries[i]
				if strings.HasPrefix(strings.ToLower(e.Name), id) ||
					strings.HasPrefix(strings.ToLower(e.ID), id) {
					match = e
					break
				}
			}
		}
		if match == nil {
			return fmt.Errorf("no bundle found matching %q\nTry: loom registry search %s", id, id)
		}

		// Pretty-print
		lock := ""
		if match.Private {
			lock = " 🔒 private"
		}
		fmt.Printf("\n  %s  [%s]%s\n", bold(match.Name), match.Type, lock)
		if match.Namespace != "" {
			fmt.Printf("  %s\n", dim("by "+match.Namespace))
		}
		if match.Description != "" {
			fmt.Printf("\n  %s\n", match.Description)
		}
		if match.LLMVerdict != "" {
			fmt.Printf("  %q\n", match.LLMVerdict)
		}
		fmt.Println()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if match.Repo != "" {
			fmt.Fprintf(w, "  Repo:\t%s\n", match.Repo)
		}
		if match.Category != "" {
			fmt.Fprintf(w, "  Category:\t%s\n", match.Category)
		}
		if len(match.Tags) > 0 {
			fmt.Fprintf(w, "  Tags:\t%s\n", strings.Join(match.Tags, ", "))
		}
		if match.Rating != nil {
			fmt.Fprintf(w, "  Rating:\t%.1f / 5.0\n", *match.Rating)
		}
		if match.Stars != nil {
			fmt.Fprintf(w, "  Stars:\t%d\n", *match.Stars)
		}
		if match.LastUpdated != "" {
			fmt.Fprintf(w, "  Updated:\t%s\n", match.LastUpdated)
		}
		if match.LocalPath != "" {
			fmt.Fprintf(w, "  Local path:\t%s\n", match.LocalPath)
		}
		_ = w.Flush()

		fmt.Printf("\n  Install:\n    %s\n\n", match.Install)
		return nil
	},
}

// ── helpers ───────────────────────────────────────────────────────────────────

// fetchAllRegistry fetches both /api/registry (public) and /api/local-registry
// (private), deduplicating by ID (local wins on conflict).
func fetchAllRegistry(port int) ([]registryEntry, error) {
	pub,  err1 := fetchRegistryEndpoint(port, "registry")
	priv, err2 := fetchRegistryEndpoint(port, "local-registry")

	if err1 != nil && err2 != nil {
		return nil, fmt.Errorf("daemon not reachable at localhost:%d — is loom running?\n  Start it with: loom start", port)
	}

	// Merge: public first, then private (private overrides on same ID)
	seen    := map[string]bool{}
	merged  := make([]registryEntry, 0, len(pub)+len(priv))

	// Private first so they appear at top
	for _, e := range priv {
		if !seen[e.ID] {
			seen[e.ID] = true
			merged = append(merged, e)
		}
	}
	for _, e := range pub {
		if !seen[e.ID] {
			seen[e.ID] = true
			merged = append(merged, e)
		}
	}
	return merged, nil
}

func fetchRegistryEndpoint(port int, endpoint string) ([]registryEntry, error) {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/%s", port, endpoint))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from /api/%s", resp.StatusCode, endpoint)
	}
	var entries []registryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// bold/dim helpers (ANSI, no-op when not a TTY)
func bold(s string) string {
	if f, ok := os.Stdout.Stat(); ok == nil && f.Mode()&os.ModeCharDevice != 0 {
		return "\x1b[1m" + s + "\x1b[0m"
	}
	return s
}
func dim(s string) string {
	if f, ok := os.Stdout.Stat(); ok == nil && f.Mode()&os.ModeCharDevice != 0 {
		return "\x1b[2m" + s + "\x1b[0m"
	}
	return s
}

func init() {
	for _, cmd := range []*cobra.Command{
		registryListCmd, registrySearchCmd, registryShowCmd,
	} {
		cmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	}

	registryListCmd.Flags().String("search", "", "Filter by name/description/tags")
	registryListCmd.Flags().String("type", "", "Filter by type (bundle, agent, tool, behavior, recipe)")
	registryListCmd.Flags().String("category", "", "Filter by category (dev, infra, knowledge, ...)")
	registryListCmd.Flags().Bool("private", false, "Show only private/local bundles")
	registryListCmd.Flags().Bool("json", false, "Output as JSON")

	registryCmd.AddCommand(registryListCmd, registrySearchCmd, registryShowCmd)
}
