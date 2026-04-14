package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the loom web UI in a browser",
	Long: `Open the loom web UI at http://localhost:7700.

If a browser tab is already open, it will be brought to the
foreground instead of opening a new window.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		url := fmt.Sprintf("http://localhost:%d", port)

		if !uiFocusExistingTab(port) {
			uiOpenBrowser(url)
		}
		fmt.Printf("Opening %s\n", url)
		return nil
	},
}

// uiFocusExistingTab signals an already-open browser tab to come forward.
// Returns true if at least one tab was reached.
func uiFocusExistingTab(port int) bool {
	endpoint := fmt.Sprintf("http://localhost:%d/api/ui/focus", port)
	resp, err := http.Post(endpoint, "application/json", nil) //nolint:noctx
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body struct {
		Clients int `json:"clients"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	return body.Clients > 0
}

// uiOpenBrowser opens url in the system's default browser.
func uiOpenBrowser(url string) {
	var bin string
	switch runtime.GOOS {
	case "darwin":
		bin = "open"
	case "windows":
		bin = "start"
	default:
		bin = "xdg-open"
	}
	_ = exec.Command(bin, url).Start()
}

func init() {
	uiCmd.Flags().Int("port", config.DefaultPort, "Daemon port")
	rootCmd.AddCommand(uiCmd)
}
