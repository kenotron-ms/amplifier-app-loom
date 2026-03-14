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
