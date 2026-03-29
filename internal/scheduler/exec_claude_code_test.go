package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/types"
)

// TestExecClaudeCode_MissingConfig verifies that a nil ClaudeCode config (or
// empty prompt) is rejected early with an error that mentions "prompt" and an
// exit code of -1.
func TestExecClaudeCode_MissingConfig(t *testing.T) {
	b := NewBroadcaster()
	runID := "test-missing-config"
	b.Register(runID)

	tests := []struct {
		name string
		job  *types.Job
	}{
		{
			name: "nil ClaudeCode",
			job:  &types.Job{ClaudeCode: nil},
		},
		{
			name: "empty prompt",
			job:  &types.Job{ClaudeCode: &types.ClaudeCodeConfig{Prompt: ""}},
		},
	}

	r := &Runner{broadcaster: b}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, exitCode, err := r.execClaudeCode(context.Background(), tc.job, runID)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "prompt") {
				t.Errorf("expected error to mention 'prompt', got: %v", err)
			}
			if exitCode != -1 {
				t.Errorf("expected exit code -1, got %d", exitCode)
			}
			if output != "" {
				t.Errorf("expected empty output, got: %q", output)
			}
		})
	}
}
