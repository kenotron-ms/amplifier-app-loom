package onboarding

import (
	"context"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/store"
)

// state holds wizard session data shared between the pure-Go state machine and
// the platform-specific CGo UI callbacks (wizard_darwin_callbacks.go).
type state struct {
	st           store.Store
	anthropicKey string
	openAIKey    string
	fdaGranted   bool
	closed       bool
	onDone       func()
}

// gState is the active wizard session. Set by Show(), read by CGo callbacks.
// Defined here (no build tag) so all platform files can access it.
var gState *state

// NeedsOnboarding returns true if the first-run wizard should be shown.
//
// Three conditions trigger onboarding:
//   - No Anthropic API key — daemon cannot run AI jobs
//   - UserContext.HomeDir missing — daemon won't find tools/configs under launchd
//   - Full Disk Access not granted — daemon will silently fail to access job dirs
func NeedsOnboarding(cfg *config.Config) bool {
	if cfg.AnthropicKey == "" {
		return true
	}
	if cfg.UserContext == nil || cfg.UserContext.HomeDir == "" {
		return true
	}
	if !CheckFDA() {
		return true
	}
	return false
}

// Show presents the onboarding wizard. onDone is called when the wizard
// completes successfully (all steps done, service installed).
// No-op on non-macOS or non-CGo builds (see wizard_other.go).
func Show(st store.Store, onDone func()) {
	cfg, _ := st.GetConfig(context.Background())
	s := &state{
		st:     st,
		onDone: onDone,
	}
	if cfg != nil {
		s.anthropicKey = cfg.AnthropicKey
		s.openAIKey = cfg.OpenAIKey
		s.fdaGranted = CheckFDA()
	}
	gState = s
	showImpl(s)
}
