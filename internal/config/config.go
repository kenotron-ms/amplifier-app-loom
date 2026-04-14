package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DefaultPort        = 7700
	DefaultMaxParallel = 4
	DefaultQueueSize   = 100
)

// UserContext holds the identity of the user who ran `loom install`.
// It is captured once at install time — when the process is running in the
// user's interactive shell session — and stored in the DB so the daemon can
// recreate the user's environment when spawning jobs, even under launchd/
// systemd where $HOME, $SHELL, and $PATH are stripped to bare minimums.
type UserContext struct {
	HomeDir  string `json:"homeDir"`
	Username string `json:"username"`
	Shell    string `json:"shell"` // absolute path, e.g. /bin/zsh
	UID      string `json:"uid,omitempty"`
}

type Config struct {
	Port        int    `json:"port"`
	MaxParallel int    `json:"maxParallel"`
	QueueSize   int    `json:"queueSize"`
	Paused      bool   `json:"paused"`
	AnthropicKey   string `json:"anthropicKey,omitempty"`
	AnthropicModel string `json:"anthropicModel,omitempty"` // e.g. "claude-sonnet-4-6"
	OpenAIKey      string `json:"openAIKey,omitempty"`
	OpenAIModel    string `json:"openAIModel,omitempty"` // e.g. "gpt-5.4"
	AIProvider     string `json:"aiProvider,omitempty"`  // "anthropic" | "openai"
	LogLevel    string `json:"logLevel"`
	UserContext *UserContext `json:"userContext,omitempty"`

	// OnboardingComplete is set to true once the user finishes the first-run wizard.
	// When false, the full 3-step wizard is shown. When true, only the tray health
	// indicator's targeted "Fix →" dialog is shown for missing conditions.
	OnboardingComplete bool `json:"onboardingComplete,omitempty"`

	PreferredTerminal string `json:"preferredTerminal,omitempty"`

	// AppBundles is the list of Amplifier bundles the user has added via loom.
	// Each entry tracks the install spec (for re-installation) and enabled state
	// (for toggling without removing).
	AppBundles []AppBundle `json:"appBundles,omitempty"`
}

// AppBundle represents one installed Amplifier app bundle in loom's config.
type AppBundle struct {
	// ID is a stable identifier (from the registry, or a slug of the install spec).
	ID string `json:"id"`
	// InstallSpec is the argument to `amplifier bundle add --app …`.
	// Derived from the registry's `install` field by trimming "amplifier bundle add ".
	// Examples: "superpowers", "git+https://github.com/…@main", "foundation:explorer"
	InstallSpec string `json:"installSpec"`
	// Name is a display name (from the registry, or the InstallSpec itself).
	Name string `json:"name,omitempty"`
	// Enabled controls whether this bundle is composed into new sessions.
	Enabled bool `json:"enabled"`
}

func Defaults() *Config {
	return &Config{
		Port:        DefaultPort,
		MaxParallel: DefaultMaxParallel,
		QueueSize:   DefaultQueueSize,
		Paused:      false,
		LogLevel:    "info",
	}
}

func Load(path string) (*Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
