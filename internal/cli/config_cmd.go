package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/platform"
	"github.com/ms/amplifier-app-loom/internal/store"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage daemon configuration",
}

var absorbEnvCmd = &cobra.Command{
	Use:   "absorb-env",
	Short: "Copy AI API keys from environment variables into the daemon's config database",
	Long: `Reads ANTHROPIC_API_KEY and OPENAI_API_KEY from the current environment
and saves them persistently in the daemon's config database.

This is useful when installing as a system service, where the daemon process
won't have access to your user environment variables.

For system-level installs, run this with sudo -E to preserve your env vars:

  sudo -E loom config absorb-env`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := platform.DBPath()
		s, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open store at %s: %w", dbPath, err)
		}
		defer s.Close()

		absorbed, err := absorbEnvKeys(s)
		if err != nil {
			return err
		}
		if absorbed == 0 {
			fmt.Println("No API keys found in environment (ANTHROPIC_API_KEY, OPENAI_API_KEY).")
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(absorbEnvCmd)
}

// absorbEnvKeys reads AI API keys from the environment and persists them in the store.
// Returns the number of keys absorbed. Prints progress to stdout.
func absorbEnvKeys(s store.Store) (int, error) {
	ctx := context.Background()
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		cfg = config.Defaults()
	}

	absorbed := 0

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.AnthropicKey = key
		if cfg.AIProvider == "" {
			cfg.AIProvider = "anthropic"
		}
		fmt.Println("  ✓ Absorbed ANTHROPIC_API_KEY")
		absorbed++
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAIKey = key
		if cfg.AIProvider == "" {
			cfg.AIProvider = "openai"
		}
		fmt.Println("  ✓ Absorbed OPENAI_API_KEY")
		absorbed++
	}

	if absorbed > 0 {
		if err := s.SaveConfig(ctx, cfg); err != nil {
			return 0, fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("  Saved to: %s\n", platform.DBPath())
	}

	return absorbed, nil
}
