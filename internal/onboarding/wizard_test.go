package onboarding

import (
	"testing"

	"github.com/ms/agent-daemon/internal/config"
)

func TestNeedsOnboarding_NoAPIKey(t *testing.T) {
	cfg := config.Defaults()
	cfg.AnthropicKey = ""
	if !NeedsOnboarding(cfg) {
		t.Error("expected NeedsOnboarding=true when AnthropicKey is empty")
	}
}

func TestNeedsOnboarding_NilUserContext(t *testing.T) {
	cfg := config.Defaults()
	cfg.AnthropicKey = "sk-ant-test"
	cfg.UserContext = nil
	if !NeedsOnboarding(cfg) {
		t.Error("expected NeedsOnboarding=true when UserContext is nil")
	}
}

func TestNeedsOnboarding_EmptyHomeDir(t *testing.T) {
	cfg := config.Defaults()
	cfg.AnthropicKey = "sk-ant-test"
	cfg.UserContext = &config.UserContext{HomeDir: ""}
	if !NeedsOnboarding(cfg) {
		t.Error("expected NeedsOnboarding=true when HomeDir is empty")
	}
}

func TestNeedsOnboarding_AllSet_ChecksNoFDA(t *testing.T) {
	cfg := config.Defaults()
	cfg.AnthropicKey = "sk-ant-test"
	cfg.UserContext = &config.UserContext{HomeDir: "/Users/test", Shell: "/bin/zsh"}
	// On non-darwin/non-cgo builds, CheckFDA() returns false → NeedsOnboarding true.
	// On darwin+cgo builds with FDA, it may return false. Either way: no panic.
	_ = NeedsOnboarding(cfg)
}
