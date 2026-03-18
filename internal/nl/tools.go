package nl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"

	"github.com/ms/agent-daemon/internal/store"
	"github.com/ms/agent-daemon/internal/types"
)

func tool(name, description string, schema anthropic.ToolInputSchemaParam) anthropic.ToolUnionParam {
	t := anthropic.ToolParam{
		Name:        name,
		Description: anthropic.String(description),
		InputSchema: schema,
	}
	return anthropic.ToolUnionParam{OfTool: &t}
}

func buildTools() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		tool("create_job", "Create a new scheduled job", anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				// Core
				"name":        map[string]interface{}{"type": "string", "description": "Short job name"},
				"description": map[string]interface{}{"type": "string", "description": "What this job does"},
				"cwd":         map[string]interface{}{"type": "string", "description": "Working directory"},
				"timeout":     map[string]interface{}{"type": "string", "description": "Max run time e.g. '5m', '30s'"},
				"max_retries": map[string]interface{}{"type": "integer", "description": "Retries on failure (default 0)"},

				// Trigger
				"trigger_type": map[string]interface{}{
					"type": "string", "enum": []string{"cron", "loop", "once", "watch"},
					"description": "How the job is triggered",
				},
				"trigger_schedule": map[string]interface{}{
					"type":        "string",
					"description": "Cron expr, loop duration, or once delay. Empty once = run immediately.",
				},

				// watch
				"watch_path":          map[string]interface{}{"type": "string", "description": "File or directory to watch (trigger_type=watch)"},
				"watch_recursive":     map[string]interface{}{"type": "boolean", "description": "Watch subdirectories recursively"},
				"watch_events":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Events: create, write, remove, rename, chmod. Empty = all."},
				"watch_mode":          map[string]interface{}{"type": "string", "enum": []string{"notify", "poll"}, "description": "notify = OS-level (default), poll = polling"},
				"watch_poll_interval": map[string]interface{}{"type": "string", "description": "Polling interval e.g. '2s' (poll mode only)"},
				"watch_debounce":      map[string]interface{}{"type": "string", "description": "Quiet window after last event before firing e.g. '500ms'"},

				// Executor
				"executor": map[string]interface{}{
					"type": "string", "enum": []string{"shell", "claude-code", "amplifier"},
					"description": "How the job runs. Default: shell.",
				},

				// shell
				"shell_command": map[string]interface{}{"type": "string", "description": "Shell command to execute (executor=shell)"},

				// claude-code
				"claude_prompt":       map[string]interface{}{"type": "string", "description": "First prompt for Claude Code (executor=claude-code)"},
				"claude_steps":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Additional follow-up prompts (multi-turn)"},
				"claude_model":        map[string]interface{}{"type": "string", "description": "Model override e.g. 'sonnet', 'opus', 'claude-sonnet-4-6'"},
				"claude_max_turns":    map[string]interface{}{"type": "integer", "description": "--max-turns limit"},
				"claude_allowed_tools": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Allowed tools e.g. ['Bash','Read','Edit']"},

				// amplifier
				"amplifier_prompt":      map[string]interface{}{"type": "string", "description": "Free-form prompt (executor=amplifier)"},
				"amplifier_steps":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Follow-up prompts (multi-turn)"},
				"amplifier_recipe_path": map[string]interface{}{"type": "string", "description": "Path to .yaml recipe file"},
				"amplifier_bundle":      map[string]interface{}{"type": "string", "description": "Bundle name e.g. 'foundation', 'recipes'"},
				"amplifier_model":       map[string]interface{}{"type": "string", "description": "Model override"},
				"amplifier_context":     map[string]interface{}{"type": "object", "description": "Recipe context variables (key-value)"},
			},
			Required: []string{"name", "trigger_type", "executor"},
		}),

		tool("update_job", "Update an existing job. Only provided fields are changed.", anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"id":          map[string]interface{}{"type": "string", "description": "Job ID"},
				"name":        map[string]interface{}{"type": "string"},
				"description": map[string]interface{}{"type": "string"},
				"cwd":         map[string]interface{}{"type": "string"},
				"timeout":     map[string]interface{}{"type": "string"},
				"max_retries": map[string]interface{}{"type": "integer"},
				"enabled":     map[string]interface{}{"type": "boolean"},

				"trigger_type":     map[string]interface{}{"type": "string", "enum": []string{"cron", "loop", "once", "watch"}},
				"trigger_schedule": map[string]interface{}{"type": "string"},

				"executor":             map[string]interface{}{"type": "string", "enum": []string{"shell", "claude-code", "amplifier"}},
				"shell_command":        map[string]interface{}{"type": "string"},
				"claude_prompt":        map[string]interface{}{"type": "string"},
				"claude_steps":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"claude_model":         map[string]interface{}{"type": "string"},
				"claude_max_turns":     map[string]interface{}{"type": "integer"},
				"claude_allowed_tools": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"amplifier_prompt":     map[string]interface{}{"type": "string"},
				"amplifier_steps":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"amplifier_recipe_path": map[string]interface{}{"type": "string"},
				"amplifier_bundle":     map[string]interface{}{"type": "string"},
				"amplifier_model":      map[string]interface{}{"type": "string"},
				"amplifier_context":    map[string]interface{}{"type": "object"},
			"watch_path":          map[string]interface{}{"type": "string"},
			"watch_recursive":     map[string]interface{}{"type": "boolean"},
			"watch_events":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"watch_mode":          map[string]interface{}{"type": "string", "enum": []string{"notify", "poll"}},
			"watch_poll_interval": map[string]interface{}{"type": "string"},
			"watch_debounce":      map[string]interface{}{"type": "string"},
			},
			Required: []string{"id"},
		}),

		tool("delete_job", "Delete a job by ID", anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"id": map[string]interface{}{"type": "string", "description": "Job ID to delete"},
			},
			Required: []string{"id"},
		}),
	}
}

// ── Tool executors ─────────────────────────────────────────────────────────────

type jobParams struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CWD         string `json:"cwd"`
	Timeout     string `json:"timeout"`
	MaxRetries  int    `json:"max_retries"`
	Enabled     *bool  `json:"enabled"`

	TriggerType     string `json:"trigger_type"`
	TriggerSchedule string `json:"trigger_schedule"`

	Executor string `json:"executor"`

	ShellCommand string `json:"shell_command"`

	ClaudePrompt       string   `json:"claude_prompt"`
	ClaudeSteps        []string `json:"claude_steps"`
	ClaudeModel        string   `json:"claude_model"`
	ClaudeMaxTurns     int      `json:"claude_max_turns"`
	ClaudeAllowedTools []string `json:"claude_allowed_tools"`

	AmplifierPrompt     string            `json:"amplifier_prompt"`
	AmplifierSteps      []string          `json:"amplifier_steps"`
	AmplifierRecipePath string            `json:"amplifier_recipe_path"`
	AmplifierBundle     string            `json:"amplifier_bundle"`
	AmplifierModel      string            `json:"amplifier_model"`
	AmplifierContext    map[string]string `json:"amplifier_context"`

	WatchPath         string   `json:"watch_path"`
	WatchRecursive    bool     `json:"watch_recursive"`
	WatchEvents       []string `json:"watch_events"`
	WatchMode         string   `json:"watch_mode"`
	WatchPollInterval string   `json:"watch_poll_interval"`
	WatchDebounce     string   `json:"watch_debounce"`
}

func applyWatchConfig(job *types.Job, p *jobParams) {
	if p.WatchPath != "" {
		job.Watch = &types.WatchConfig{
			Path:         p.WatchPath,
			Recursive:    p.WatchRecursive,
			Events:       p.WatchEvents,
			Mode:         p.WatchMode,
			PollInterval: p.WatchPollInterval,
			Debounce:     p.WatchDebounce,
		}
	}
}

func applyExecutorConfig(job *types.Job, p *jobParams) {
	exec := types.ExecutorType(p.Executor)
	if exec == "" {
		exec = types.ExecutorShell
	}
	job.Executor = exec

	switch exec {
	case types.ExecutorShell:
		job.Shell = &types.ShellConfig{Command: p.ShellCommand}
	case types.ExecutorClaudeCode:
		job.ClaudeCode = &types.ClaudeCodeConfig{
			Prompt:            p.ClaudePrompt,
			Steps:             p.ClaudeSteps,
			Model:             p.ClaudeModel,
			MaxTurns:          p.ClaudeMaxTurns,
			AllowedTools:      p.ClaudeAllowedTools,
		}
	case types.ExecutorAmplifier:
		job.Amplifier = &types.AmplifierConfig{
			Prompt:     p.AmplifierPrompt,
			Steps:      p.AmplifierSteps,
			RecipePath: p.AmplifierRecipePath,
			Bundle:     p.AmplifierBundle,
			Model:      p.AmplifierModel,
			Context:    p.AmplifierContext,
		}
	}
}

func executeCreateJob(ctx context.Context, s store.Store, input json.RawMessage) (string, string, error) {
	var p jobParams
	if err := json.Unmarshal(input, &p); err != nil {
		return "", "", err
	}

	job := &types.Job{
		ID:          uuid.New().String(),
		Name:        p.Name,
		Description: p.Description,
		CWD:         p.CWD,
		Timeout:     p.Timeout,
		MaxRetries:  p.MaxRetries,
		Trigger: types.Trigger{
			Type:     types.TriggerType(p.TriggerType),
			Schedule: p.TriggerSchedule,
		},
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	applyExecutorConfig(job, &p)
	applyWatchConfig(job, &p)

	if err := s.SaveJob(ctx, job); err != nil {
		return "", "", fmt.Errorf("save job: %w", err)
	}
	action := fmt.Sprintf("Created job '%s' [%s] (id: %s)", job.Name, job.Executor, job.ID)
	return fmt.Sprintf(`{"id":"%s","name":"%s","status":"created"}`, job.ID, job.Name), action, nil
}

func executeUpdateJob(ctx context.Context, s store.Store, sched JobScheduler, input json.RawMessage) (string, string, error) {
	var p jobParams
	if err := json.Unmarshal(input, &p); err != nil {
		return "", "", err
	}

	job, err := s.GetJob(ctx, p.ID)
	if err != nil {
		return "", "", fmt.Errorf("job not found: %w", err)
	}

	if p.Name != "" {
		job.Name = p.Name
	}
	if p.Description != "" {
		job.Description = p.Description
	}
	if p.CWD != "" {
		job.CWD = p.CWD
	}
	if p.Timeout != "" {
		job.Timeout = p.Timeout
	}
	if p.MaxRetries != 0 {
		job.MaxRetries = p.MaxRetries
	}
	if p.Enabled != nil {
		job.Enabled = *p.Enabled
	}
	if p.TriggerType != "" {
		job.Trigger.Type = types.TriggerType(p.TriggerType)
	}
	if p.TriggerSchedule != "" {
		job.Trigger.Schedule = p.TriggerSchedule
	}
	// Apply executor config: if the AI didn't pass executor explicitly, fall back
	// to the job's existing executor so fields like shell_command still apply.
	if p.Executor == "" {
		p.Executor = string(job.Executor)
	}
	applyExecutorConfig(job, &p)
	applyWatchConfig(job, &p)
	job.UpdatedAt = time.Now()

	if err := s.SaveJob(ctx, job); err != nil {
		return "", "", fmt.Errorf("save job: %w", err)
	}
	if sched != nil {
		sched.RemoveJob(job.ID)
		sched.AddJob(job)
	}
	action := fmt.Sprintf("Updated job '%s' (id: %s)", job.Name, job.ID)
	return fmt.Sprintf(`{"id":"%s","name":"%s","status":"updated"}`, job.ID, job.Name), action, nil
}

func executeDeleteJob(ctx context.Context, s store.Store, input json.RawMessage) (string, string, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", "", err
	}
	job, err := s.GetJob(ctx, params.ID)
	if err != nil {
		return "", "", fmt.Errorf("job not found: %w", err)
	}
	name := job.Name
	if err := s.DeleteJob(ctx, params.ID); err != nil {
		return "", "", fmt.Errorf("delete job: %w", err)
	}
	action := fmt.Sprintf("Deleted job '%s' (id: %s)", name, params.ID)
	return fmt.Sprintf(`{"id":"%s","status":"deleted"}`, params.ID), action, nil
}
