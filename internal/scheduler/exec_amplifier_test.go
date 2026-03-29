package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/types"
)

// TestExecAmplifier_MissingConfig verifies that a nil Amplifier config returns
// an error mentioning "config" and an exit code of -1.
func TestExecAmplifier_MissingConfig(t *testing.T) {
	b := NewBroadcaster()
	runID := "test-amplifier-missing-config"
	b.Register(runID)

	r := &Runner{broadcaster: b}
	job := &types.Job{Amplifier: nil}
	output, exitCode, err := r.execAmplifier(context.Background(), job, runID)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "config") {
		t.Errorf("expected error to mention 'config', got: %v", err)
	}
	if exitCode != -1 {
		t.Errorf("expected exit code -1, got %d", exitCode)
	}
	if output != "" {
		t.Errorf("expected empty output, got: %q", output)
	}
}

// TestExecAmplifier_MissingPromptAndRecipe verifies that an empty AmplifierConfig
// (no prompt, no recipe_path) returns an error and exit code of -1.
func TestExecAmplifier_MissingPromptAndRecipe(t *testing.T) {
	b := NewBroadcaster()
	runID := "test-amplifier-missing-prompt"
	b.Register(runID)

	r := &Runner{broadcaster: b}
	job := &types.Job{Amplifier: &types.AmplifierConfig{}}
	output, exitCode, err := r.execAmplifier(context.Background(), job, runID)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if exitCode != -1 {
		t.Errorf("expected exit code -1, got %d", exitCode)
	}
	if output != "" {
		t.Errorf("expected empty output, got: %q", output)
	}
}
