package nl

import (
	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/store"
)

// NewClientFromConfig creates the appropriate NLClient based on config.
// Returns nil if no provider is configured.
// sched is passed to the client so it can notify the live scheduler after
// job updates; it may be nil (updates will still persist to the DB).
func NewClientFromConfig(cfg *config.Config, s store.Store, sched JobScheduler) NLClient {
	switch cfg.AIProvider {
	case "openai":
		if cfg.OpenAIKey != "" {
			return NewOpenAIClient(cfg.OpenAIKey, cfg.OpenAIModel, s, sched)
		}
	default: // "anthropic" or empty
		if cfg.AnthropicKey != "" {
			return NewAnthropicClient(cfg.AnthropicKey, cfg.AnthropicModel, s, sched)
		}
	}
	return nil
}
