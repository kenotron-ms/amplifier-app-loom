package onboarding

import (
	"testing"

	"github.com/ms/amplifier-app-loom/internal/config"
)

// ── DetectNeededSteps: API key logic ──────────────────────────────────────────

func TestDetectNeededSteps_BothKeysMissing_NeedsAPIKey(t *testing.T) {
	cfg := config.Defaults()
	cfg.AnthropicKey = ""
	cfg.OpenAIKey = ""
	steps := DetectNeededSteps(cfg)
	if !steps.NeedsAPIKey {
		t.Error("expected NeedsAPIKey=true when both keys are empty")
	}
}

func TestDetectNeededSteps_AnthropicKeySet_SkipsAPIStep(t *testing.T) {
	cfg := config.Defaults()
	cfg.AnthropicKey = "sk-ant-test"
	cfg.OpenAIKey = ""
	steps := DetectNeededSteps(cfg)
	if steps.NeedsAPIKey {
		t.Error("expected NeedsAPIKey=false when AnthropicKey is set")
	}
}

func TestDetectNeededSteps_OpenAIKeySet_SkipsAPIStep(t *testing.T) {
	// Any key is sufficient — wizard should not prompt again.
	cfg := config.Defaults()
	cfg.AnthropicKey = ""
	cfg.OpenAIKey = "sk-openai-test"
	steps := DetectNeededSteps(cfg)
	if steps.NeedsAPIKey {
		t.Error("expected NeedsAPIKey=false when OpenAIKey is set")
	}
}

// ── DetectNeededSteps: FDA never blocks the wizard ────────────────────────────

func TestDetectNeededSteps_FDANeverRequired(t *testing.T) {
	// FDA was removed from the wizard (Option A).
	// It surfaces via the tray health indicator instead.
	// NeedsFDA must always be false regardless of system state.
	cfg := config.Defaults()
	steps := DetectNeededSteps(cfg)
	if steps.NeedsFDA {
		t.Error("NeedsFDA must always be false — FDA is shown by tray health indicator, not wizard")
	}
}

// ── NeedsOnboarding: no API key always triggers ───────────────────────────────

func TestNeedsOnboarding_NoAPIKey(t *testing.T) {
	cfg := config.Defaults()
	cfg.AnthropicKey = ""
	cfg.OpenAIKey = ""
	if !NeedsOnboarding(cfg) {
		t.Error("expected NeedsOnboarding=true when no API key is set")
	}
}
