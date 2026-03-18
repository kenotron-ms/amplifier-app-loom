package config

import (
	"encoding/json"
	"os/user"
	"strings"
	"testing"
)

func TestCaptureUserContext_ReturnsValidContext(t *testing.T) {
	uc := CaptureUserContext()
	if uc == nil {
		t.Fatal("expected non-nil UserContext")
	}
	if uc.HomeDir == "" {
		t.Error("HomeDir should not be empty")
	}
	if uc.Username == "" {
		t.Error("Username should not be empty")
	}
	if uc.Shell == "" {
		t.Error("Shell should not be empty")
	}
	if !strings.HasPrefix(uc.Shell, "/") {
		t.Errorf("Shell should be an absolute path, got %q", uc.Shell)
	}
}

func TestConfigHasOnboardingComplete(t *testing.T) {
	cfg := Defaults()
	if cfg.OnboardingComplete {
		t.Error("OnboardingComplete should default to false")
	}
	cfg.OnboardingComplete = true
	if !cfg.OnboardingComplete {
		t.Error("OnboardingComplete should be settable to true")
	}
}

func TestOnboardingComplete_JSONRoundTrip(t *testing.T) {
	cfg := Defaults()
	cfg.OnboardingComplete = true
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"onboardingComplete"`) {
		t.Error("expected onboardingComplete in JSON when true")
	}
	cfg.OnboardingComplete = false
	data, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"onboardingComplete"`) {
		t.Error("expected onboardingComplete omitted from JSON when false (omitempty)")
	}
}

func TestCaptureUserContext_SudoUser(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Skip("cannot get current user")
	}
	t.Setenv("SUDO_USER", current.Username)
	uc := CaptureUserContext()
	if uc == nil {
		t.Fatal("expected non-nil UserContext with SUDO_USER set")
	}
	if uc.Username != current.Username {
		t.Errorf("expected username %q, got %q", current.Username, uc.Username)
	}
}
