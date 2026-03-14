package nl

import (
	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/store"
)

// NewClientFromConfig creates the appropriate NLClient based on config.
// Returns nil if no provider is configured.
func NewClientFromConfig(cfg *config.Config, s store.Store) NLClient {
	switch cfg.AIProvider {
	case "openai":
		if cfg.OpenAIKey != "" {
			return NewOpenAIClient(cfg.OpenAIKey, cfg.OpenAIModel, s)
		}
	default: // "anthropic" or empty
		if cfg.AnthropicKey != "" {
			return NewAnthropicClient(cfg.AnthropicKey, cfg.AnthropicModel, s)
		}
	}
	return nil
}
