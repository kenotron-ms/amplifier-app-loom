package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ms/agent-daemon/internal/types"
)

// amplifierCmd builds an exec.Cmd for the amplifier binary, resolving its
// absolute path via resolveBinary so it works even when the daemon's inherited
// PATH (e.g. under launchd) omits user bin directories.
func amplifierCmd(ctx context.Context, args ...string) *exec.Cmd {
	bin, err := resolveBinary("amplifier")
	if err != nil {
		return exec.CommandContext(ctx, "amplifier", args...)
	}
	return exec.CommandContext(ctx, bin, args...)
}

func execAmplifier(ctx context.Context, job *types.Job, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	cfg := job.Amplifier
	if cfg == nil {
		return "", -1, fmt.Errorf("amplifier executor requires config")
	}

	if cfg.RecipePath != "" {
		return execAmplifierRecipe(ctx, job, cfg, b, runID)
	}
	return execAmplifierPrompt(ctx, job, cfg, b, runID)
}

// execAmplifierPrompt runs one or more prompt steps via `amplifier run`.
func execAmplifierPrompt(ctx context.Context, job *types.Job, cfg *types.AmplifierConfig, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	if cfg.Prompt == "" {
		return "", -1, fmt.Errorf("amplifier executor requires a prompt or recipe_path")
	}

	steps := append([]string{cfg.Prompt}, cfg.Steps...)
	var sessionID string
	var allOutput strings.Builder

	for i, step := range steps {
		args := buildAmplifierArgs(cfg, step, sessionID)
		cmd := amplifierCmd(ctx, args...)
		if job.CWD != "" {
			cmd.Dir = job.CWD
		}

		if i > 0 {
			sep := fmt.Sprintf("\n--- step %d ---\n", i+1)
			b.Write(runID, sep)
			allOutput.WriteString(sep)
		}

		stepOut, code, runErr := streamCommand(cmd, b, runID)
		allOutput.WriteString(stepOut)

		if runErr != nil {
			return allOutput.String(), code, runErr
		}

		// Extract session_id from JSON output to chain steps.
		if sessionID == "" {
			if id := extractAmplifierSessionID(stepOut); id != "" {
				sessionID = id
			}
		}
	}

	return allOutput.String(), 0, nil
}

// execAmplifierRecipe runs a recipe via `amplifier tool invoke recipe_execute`.
func execAmplifierRecipe(ctx context.Context, job *types.Job, cfg *types.AmplifierConfig, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	contextJSON := "{}"
	if len(cfg.Context) > 0 {
		bs, _ := json.Marshal(cfg.Context)
		contextJSON = string(bs)
	}

	args := []string{
		"tool", "invoke", "recipe_execute",
		fmt.Sprintf("recipe_path=%s", cfg.RecipePath),
		fmt.Sprintf("context=%s", contextJSON),
	}
	if cfg.Bundle != "" {
		args = append([]string{"--bundle", cfg.Bundle}, args...)
	}

	cmd := amplifierCmd(ctx, args...)
	if job.CWD != "" {
		cmd.Dir = job.CWD
	}

	return streamCommand(cmd, b, runID)
}

func buildAmplifierArgs(cfg *types.AmplifierConfig, prompt, sessionID string) []string {
	args := []string{"run", "--output-format", "json"}

	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	if cfg.Bundle != "" {
		args = append(args, "--bundle", cfg.Bundle)
	}
	if cfg.Model != "" {
		args = append(args, "-m", cfg.Model)
	}
	args = append(args, prompt)
	return args
}

func extractAmplifierSessionID(jsonOutput string) string {
	lines := strings.Split(strings.TrimSpace(jsonOutput), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var result struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal([]byte(line), &result); err == nil && result.SessionID != "" {
			return result.SessionID
		}
	}
	return ""
}
