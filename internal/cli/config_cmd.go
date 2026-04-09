package cli

import (
	"context"
	"fmt"

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
	Short: "Auto-detect AI API keys and save them into the daemon's config database",
	Long: `Detects AI provider API keys from common local sources and saves them
persistently in the daemon's config database.

Sources checked in priority order:
  1. Process environment  — ANTHROPIC_API_KEY, OPENAI_API_KEY
  2. ~/.amplifier/keys.env
  3. ~/.anthropic/api_key  (Anthropic CLI format)
  4. ~/.env
  5. Shell dotfiles        — ~/.zshrc, ~/.zshenv, ~/.zprofile, ~/.bash_profile, etc.

This is useful when installing as a system service, where the daemon process
won't have access to your user shell environment.`,
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

// absorbEnvKeys detects AI API keys from local sources and persists them in
// the store.  Returns the number of keys absorbed.  Prints progress to stdout.
func absorbEnvKeys(s store.Store) (int, error) {
	ctx := context.Background()
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		cfg = config.Defaults()
	}

	detected := config.DetectAPIKeys()
	absorbed := 0

	if detected.AnthropicKey != "" {
		cfg.AnthropicKey = detected.AnthropicKey
		if cfg.AIProvider == "" {
			cfg.AIProvider = "anthropic"
		}
		fmt.Printf("  ✓ Absorbed ANTHROPIC_API_KEY  (from %s)\n", detected.AnthropicSource)
		absorbed++
	}

	if detected.OpenAIKey != "" {
		cfg.OpenAIKey = detected.OpenAIKey
		if cfg.AIProvider == "" {
			cfg.AIProvider = "openai"
		}
		fmt.Printf("  ✓ Absorbed OPENAI_API_KEY  (from %s)\n", detected.OpenAISource)
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
