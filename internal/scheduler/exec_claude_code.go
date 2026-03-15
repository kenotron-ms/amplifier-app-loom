package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ms/agent-daemon/internal/types"
)

func execClaudeCode(ctx context.Context, job *types.Job, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	cfg := job.ClaudeCode
	if cfg == nil || cfg.Prompt == "" {
		return "", -1, fmt.Errorf("claude-code executor requires a prompt")
	}

	steps := append([]string{cfg.Prompt}, cfg.Steps...)
	var sessionID string
	var allOutput strings.Builder

	for i, step := range steps {
		args := buildClaudeArgs(cfg, step, sessionID)
		cmd := exec.CommandContext(ctx, "claude", args...)
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
			if id := extractSessionID(stepOut); id != "" {
				sessionID = id
			}
		}
	}

	return allOutput.String(), 0, nil
}

func buildClaudeArgs(cfg *types.ClaudeCodeConfig, prompt, sessionID string) []string {
	args := []string{"-p", prompt, "--output-format", "json"}

	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(cfg.MaxTurns))
	}
	if len(cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.AllowedTools, ","))
	}
	if cfg.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", cfg.AppendSystemPrompt)
	}
	return args
}

func extractSessionID(jsonOutput string) string {
	// Claude outputs may have non-JSON lines (e.g. progress). Find the last JSON object.
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
