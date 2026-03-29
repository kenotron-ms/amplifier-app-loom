package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/mirror"
)

// connectorAdmitCmd implements the admission flow for new connectors.
// It guides the user through setting up a data source, including browser
// authentication if needed, and creates the connector.
var connectorAdmitCmd = &cobra.Command{
	Use:   "admit",
	Short: "Interactively set up a new connector with browser auth support",
	Long: `Admit guides you through creating a new connector. For browser-based
sources, it opens a headed browser so you can authenticate once — then the
daemon uses that session headlessly going forward.

The connector's Prompt describes what to watch in natural language. The
browser-operator agent (or command/HTTP fetcher) uses it each sync to
extract the data you care about.

Examples:
  # Monitor an Amazon product — opens browser for auth
  loom connector admit --name "AirPods Pro" \
    --url "https://amazon.com/dp/B09V3KXJPB" \
    --site amazon --method browser \
    --prompt "Extract current price, availability, and any active deals" \
    --entity "amazon.product/B09V3KXJPB"

  # Monitor a GitHub PR via CLI — no browser needed
  loom connector admit --name "PR #42" \
    --url "https://github.com/owner/repo/pull/42" \
    --method command \
    --command "gh api /repos/owner/repo/pulls/42" \
    --entity "github.pr/owner/repo/42"

  # Monitor a Teams channel — auto-detect method
  loom connector admit --name "Engineering General" \
    --url "https://teams.microsoft.com/..." \
    --site teams --method browser \
    --prompt "Extract the 10 most recent messages with author and timestamp" \
    --entity "teams.channel/engineering/general"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		name, _ := cmd.Flags().GetString("name")
		method, _ := cmd.Flags().GetString("method")
		cmdStr, _ := cmd.Flags().GetString("command")
		url, _ := cmd.Flags().GetString("url")
		site, _ := cmd.Flags().GetString("site")
		entity, _ := cmd.Flags().GetString("entity")
		interval, _ := cmd.Flags().GetString("interval")
		prompt, _ := cmd.Flags().GetString("prompt")

		// Validate required fields
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if entity == "" {
			return fmt.Errorf("--entity is required (e.g. github.pr/owner/repo/42)")
		}
		if method == "" {
			// Auto-detect method based on what's provided
			if cmdStr != "" {
				method = "command"
			} else if url != "" {
				method = "http"
			} else {
				return fmt.Errorf("--method is required (command, http, or browser)")
			}
		}

		// For browser-based connectors, run the auth flow
		if method == "browser" {
			if url == "" {
				return fmt.Errorf("--url is required for browser connectors")
			}
			if err := runBrowserAuthFlow(site, url); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: browser auth flow failed: %v\n", err)
				fmt.Fprintln(os.Stderr, "The connector will still be created. You can authenticate later.")
			}
		}

		// Create the connector via the API
		conn := mirror.Connector{
			Name:          name,
			FetchMethod:   mirror.FetchMethod(method),
			Command:       cmdStr,
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
			return fmt.Errorf("error creating connector: %s", errResp["error"])
		}

		var created mirror.Connector
		json.NewDecoder(resp.Body).Decode(&created)
		id := created.ID
		if len(id) > 8 {
			id = id[:8]
		}

		fmt.Printf("✓ Connector admitted: %s (id: %s)\n", created.Name, id)
		fmt.Printf("  Entity:   %s\n", created.EntityAddress)
		fmt.Printf("  Method:   %s\n", created.FetchMethod)
		fmt.Printf("  Interval: %s\n", created.Interval)
		if created.Prompt != "" {
			fmt.Printf("  Prompt:   %s\n", truncate(created.Prompt, 60))
		}
		fmt.Println()
		fmt.Println("The connector is now syncing. Check status with:")
		fmt.Printf("  loom mirror connectors\n")
		fmt.Printf("  loom mirror get %s\n", created.EntityAddress)

		return nil
	},
}

// runBrowserAuthFlow opens a headed browser to the given URL so the user can
// authenticate. The session is saved to a persistent profile directory grouped
// by site name. Subsequent headless syncs reuse this profile.
func runBrowserAuthFlow(site, url string) error {
	if site == "" {
		site = "default"
	}

	profileDir := browserProfileDir(site)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}

	fmt.Println("Opening browser for authentication...")
	fmt.Printf("  Site:    %s\n", site)
	fmt.Printf("  URL:     %s\n", url)
	fmt.Printf("  Profile: %s\n", profileDir)
	fmt.Println()
	fmt.Println("Please log in. The browser will save your session.")
	fmt.Println("Close the browser or press Ctrl+C when done.")
	fmt.Println()

	// Check if agent-browser is available
	agentBrowser, err := exec.LookPath("agent-browser")
	if err != nil {
		return fmt.Errorf("agent-browser not found in PATH — install with: npm install -g agent-browser")
	}

	// Launch agent-browser in headed mode with persistent profile.
	// --headed flag opens a visible browser window.
	cmd := exec.Command(agentBrowser,
		"--profile", profileDir,
		"--headed",
		"open", url,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		// Non-zero exit is OK — user might close browser manually
		if strings.Contains(err.Error(), "exit status") {
			return nil
		}
		return err
	}

	fmt.Println("✓ Browser session saved.")
	return nil
}

// browserProfileDir returns the path for a site's browser profile.
func browserProfileDir(site string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".loom", "browser-profiles", site)
}

// truncate shortens a string with ellipsis if it exceeds maxLen.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func init() {
	connectorAdmitCmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	connectorAdmitCmd.Flags().String("name", "", "Connector name (required)")
	connectorAdmitCmd.Flags().String("method", "", "Fetch method: command, http, or browser (auto-detected if omitted)")
	connectorAdmitCmd.Flags().String("command", "", "Shell command for --method command")
	connectorAdmitCmd.Flags().String("url", "", "URL to watch")
	connectorAdmitCmd.Flags().String("site", "", "Site name for browser profile grouping (e.g. amazon, teams)")
	connectorAdmitCmd.Flags().String("entity", "", "Entity address, e.g. github.pr/owner/repo/42 (required)")
	connectorAdmitCmd.Flags().String("interval", "5m", "Sync interval (e.g. 30s, 5m, 1h)")
	connectorAdmitCmd.Flags().String("prompt", "", "Natural language description of what to extract/watch")

	connectorCmd.AddCommand(connectorAdmitCmd)
}