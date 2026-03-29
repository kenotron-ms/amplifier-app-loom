package onboarding

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/store"
)

// OnboardingSteps captures which setup conditions are unmet at wizard launch time.
// Only steps with true flags are shown in the wizard.
type OnboardingSteps struct {
	NeedsAPIKey  bool // no AI key at all (neither Anthropic nor OpenAI)
	NeedsFDA     bool // always false — FDA is surfaced by the tray health indicator, not the wizard
	NeedsService bool // LaunchAgent/Daemon plist not present
}

// state holds wizard session data shared between the pure-Go state machine and
// the platform-specific CGo UI callbacks (wizard_darwin_callbacks.go).
type state struct {
	mu           sync.Mutex  // guards anthropicKey and openAIKey
	anthropicKey string
	openAIKey    string
	fdaGranted   atomic.Bool // accessed from multiple goroutines; use Load/Store
	closed       atomic.Bool // accessed from multiple goroutines; use Load/Store
	onDone       func()
	steps        OnboardingSteps // which steps are needed (set once at Show() time)
}

// gState is the active wizard session. Set by Show(), read by CGo callbacks.
// Defined here (no build tag) so all platform files can access it.
var gState atomic.Pointer[state]

// DetectNeededSteps inspects live system state to determine which wizard steps
// are required. Each step is only shown if its corresponding condition is unmet.
func DetectNeededSteps(cfg *config.Config) OnboardingSteps {
	return OnboardingSteps{
		// Only prompt for a key if the user has provided neither Anthropic nor OpenAI.
		NeedsAPIKey:  cfg.AnthropicKey == "" && cfg.OpenAIKey == "",
		// FDA is handled by the tray health indicator, not the wizard.
		NeedsFDA:     false,
		NeedsService: !isServiceInstalled(),
	}
}

// NeedsOnboarding returns true if any setup step is incomplete.
// If this returns false the tray loads silently with no wizard shown.
func NeedsOnboarding(cfg *config.Config) bool {
	s := DetectNeededSteps(cfg)
	return s.NeedsAPIKey || s.NeedsFDA || s.NeedsService
}

// Show presents the onboarding wizard. Only the steps returned by
// DetectNeededSteps are shown; completed steps are skipped automatically.
// onDone is called when the wizard closes successfully.
// No-op on non-macOS or non-CGo builds (see wizard_other.go).
//
// Note: st is only used here to read initial config for pre-filling.
// handleDone() opens its own connection to avoid holding st open for the
// wizard's lifetime.
func Show(st store.Store, onDone func()) {
	// Guard: don't open a second wizard if one is already active.
	if gState.Load() != nil {
		slog.Warn("onboarding: wizard already open, ignoring Show() call")
		return
	}
	cfg, err := st.GetConfig(context.Background())
	if err != nil {
		slog.Warn("onboarding: failed to read config from store", "err", err)
	}
	s := &state{
		onDone: onDone,
	}
	if cfg != nil {
		s.anthropicKey = cfg.AnthropicKey
		s.openAIKey = cfg.OpenAIKey
		s.fdaGranted.Store(CheckFDA())
		s.steps = DetectNeededSteps(cfg)
	} else {
		// No config yet — assume API key and service are needed; FDA never needed.
		s.steps = OnboardingSteps{NeedsAPIKey: true, NeedsFDA: false, NeedsService: true}
	}
	gState.Store(s)
	showImpl(s)
}
