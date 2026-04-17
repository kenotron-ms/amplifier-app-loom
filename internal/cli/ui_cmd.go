package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the loom web UI in a browser",
	Long:  `Open the loom web UI at http://localhost:7700 in a new browser window.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		url := fmt.Sprintf("http://localhost:%d", port)
		uiOpenBrowser(url)
		fmt.Printf("Opening %s\n", url)
		return nil
	},
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
