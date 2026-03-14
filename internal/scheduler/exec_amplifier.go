package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ms/agent-daemon/internal/types"
)

func execAmplifier(ctx context.Context, job *types.Job) (output string, exitCode int, err error) {
	cfg := job.Amplifier
	if cfg == nil {
		return "", -1, fmt.Errorf("amplifier executor requires config")
	}

	if cfg.RecipePath != "" {
		return execAmplifierRecipe(ctx, job, cfg)
	}
	return execAmplifierPrompt(ctx, job, cfg)
}

// execAmplifierPrompt runs one or more prompt steps via `amplifier run`.
func execAmplifierPrompt(ctx context.Context, job *types.Job, cfg *types.AmplifierConfig) (output string, exitCode int, err error) {
	if cfg.Prompt == "" {
		return "", -1, fmt.Errorf("amplifier executor requires a prompt or recipe_path")
	}

	steps := append([]string{cfg.Prompt}, cfg.Steps...)
	var sessionID string
	var allOutput strings.Builder

	for i, step := range steps {
		args := buildAmplifierArgs(cfg, step, sessionID)
		cmd := exec.CommandContext(ctx, "amplifier", args...)
		if job.CWD != "" {
			cmd.Dir = job.CWD
		}

		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf

		runErr := cmd.Run()
		stepOut := buf.String()

		if i > 0 {
			allOutput.WriteString(fmt.Sprintf("\n--- step %d ---\n", i+1))
		}
		allOutput.WriteString(stepOut)

		if runErr != nil {
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			} else {
				exitCode = -1
			}
			return allOutput.String(), exitCode, runErr
		}

		// Extract session_id from JSON output to chain steps
		if sessionID == "" {
			if id := extractAmplifierSessionID(stepOut); id != "" {
				sessionID = id
			}
		}
	}

	return allOutput.String(), 0, nil
}

// execAmplifierRecipe runs a recipe via `amplifier tool invoke recipe_execute`.
func execAmplifierRecipe(ctx context.Context, job *types.Job, cfg *types.AmplifierConfig) (output string, exitCode int, err error) {
	contextJSON := "{}"
	if len(cfg.Context) > 0 {
		b, _ := json.Marshal(cfg.Context)
		contextJSON = string(b)
	}

	args := []string{
		"tool", "invoke", "recipe_execute",
		fmt.Sprintf("recipe_path=%s", cfg.RecipePath),
		fmt.Sprintf("context=%s", contextJSON),
	}
	if cfg.Bundle != "" {
		args = append([]string{"--bundle", cfg.Bundle}, args...)
	}

	cmd := exec.CommandContext(ctx, "amplifier", args...)
	if job.CWD != "" {
		cmd.Dir = job.CWD
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err = cmd.Run()
	output = buf.String()
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return
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
