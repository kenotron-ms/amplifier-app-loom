package types

import "time"

// ── Trigger ───────────────────────────────────────────────────────────────────

type TriggerType string

const (
	TriggerLoop      TriggerType = "loop"      // repeating interval, e.g. "30s"
	TriggerCron      TriggerType = "cron"       // standard cron expression
	TriggerOnce      TriggerType = "once"       // run once after optional delay, then auto-disable
	TriggerWatch     TriggerType = "watch"      // fires when a file/directory changes
	TriggerConnector TriggerType = "connector"  // fires when a mirror connector detects a change
)

type Trigger struct {
	Type     TriggerType `json:"type"`
	Schedule string      `json:"schedule"` // cron expr OR Go duration string
}

// ── Executor ──────────────────────────────────────────────────────────────────

type ExecutorType string

const (
	ExecutorShell      ExecutorType = "shell"       // run a shell command
	ExecutorClaudeCode ExecutorType = "claude-code" // run via `claude -p`
	ExecutorAmplifier  ExecutorType = "amplifier"   // run via `amplifier run` or recipe
)

// ShellConfig is the config for ExecutorShell.
type ShellConfig struct {
	Command string `json:"command"`
}

// ClaudeCodeConfig is the config for ExecutorClaudeCode.
type ClaudeCodeConfig struct {
	Prompt            string   `json:"prompt"`                      // first / only prompt
	Steps             []string `json:"steps,omitempty"`             // additional turns (multi-step)
	Model             string   `json:"model,omitempty"`             // e.g. "sonnet", "opus", "claude-sonnet-4-6"
	MaxTurns          int      `json:"maxTurns,omitempty"`          // --max-turns
	AllowedTools      []string `json:"allowedTools,omitempty"`      // --allowedTools
	AppendSystemPrompt string  `json:"appendSystemPrompt,omitempty"` // --append-system-prompt
}

// AmplifierConfig is the config for ExecutorAmplifier.
type AmplifierConfig struct {
	Prompt     string            `json:"prompt,omitempty"`     // free-form prompt (amplifier run "...")
	Steps      []string          `json:"steps,omitempty"`      // additional turns (multi-step)
	RecipePath string            `json:"recipePath,omitempty"` // path to .yaml recipe file
	Bundle     string            `json:"bundle,omitempty"`     // --bundle (e.g. "foundation", "recipes")
	Model      string            `json:"model,omitempty"`      // -m flag
	Context    map[string]string `json:"context,omitempty"`    // recipe context variables
}

// ConnectorConfig is the config for TriggerConnector.
// Links a job to a mirror connector — the job fires when the connector detects a change.
type ConnectorConfig struct {
	ConnectorID string `json:"connectorId"` // ID of the mirror connector that triggers this job
}

// WatchConfig is the config for TriggerWatch.
type WatchConfig struct {
	Path         string   `json:"path"`                   // file or directory to watch
	Recursive    bool     `json:"recursive"`              // watch subdirectories
	Events       []string `json:"events,omitempty"`       // create, write, remove, rename, chmod — empty = all
	Mode         string   `json:"mode"`                   // "notify" (OS-level) or "poll"
	PollInterval string   `json:"pollInterval,omitempty"` // poll mode only, e.g. "2s"
	Debounce     string   `json:"debounce,omitempty"`     // quiet window before firing, e.g. "500ms"
}

// ── Job ───────────────────────────────────────────────────────────────────────

type Job struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Trigger     Trigger      `json:"trigger"`
	Executor    ExecutorType `json:"executor"` // "shell", "claude-code", "amplifier"
	CWD         string       `json:"cwd"`
	Enabled     bool         `json:"enabled"`
	MaxRetries  int          `json:"maxRetries"`
	Timeout     string       `json:"timeout"` // Go duration string, "" = no limit
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`

	// Executor configs — only the one matching Executor will be set.
	Shell      *ShellConfig      `json:"shell,omitempty"`
	ClaudeCode *ClaudeCodeConfig `json:"claudeCode,omitempty"`
	Amplifier  *AmplifierConfig  `json:"amplifier,omitempty"`

	// Watch trigger config.
	Watch *WatchConfig `json:"watch,omitempty"`

	// Connector trigger config.
	Connector *ConnectorConfig `json:"connector,omitempty"`

	// Deprecated: top-level Command kept for backward compat with existing DB entries.
	Command string `json:"command,omitempty"`

	// RuntimeEnv holds transient env vars injected at dispatch time (e.g. JOB_WATCH_PATH).
	// Not persisted — zeroed on every load from store.
	RuntimeEnv map[string]string `json:"-"`
}

// ResolvedExecutor returns the effective executor type, defaulting to shell.
func (j *Job) ResolvedExecutor() ExecutorType {
	if j.Executor == "" {
		return ExecutorShell
	}
	return j.Executor
}

// ── Run ───────────────────────────────────────────────────────────────────────

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusSuccess   RunStatus = "success"
	RunStatusFailed    RunStatus = "failed"
	RunStatusTimeout   RunStatus = "timeout"
	RunStatusSkipped   RunStatus = "skipped"
	RunStatusCancelled RunStatus = "cancelled"
)

type JobRun struct {
	ID        string     `json:"id"`
	JobID     string     `json:"jobId"`
	JobName   string     `json:"jobName"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	Status    RunStatus  `json:"status"`
	ExitCode  int        `json:"exitCode"`
	Output    string     `json:"output"` // combined stdout+stderr (capped)
	Attempt   int        `json:"attempt"`
}

// ── Chat history ──────────────────────────────────────────────────────────────

type ChatMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// ── Daemon status ─────────────────────────────────────────────────────────────

type DaemonStatus struct {
	State      string    `json:"state"` // "running" | "paused" | "stopped"
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"startedAt"`
	ActiveRuns int       `json:"activeRuns"`
	QueueDepth int       `json:"queueDepth"`
	JobCount   int       `json:"jobCount"`
	Version    string    `json:"version"`
}
