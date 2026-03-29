# CLI Full Trigger & Executor Support — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `loom add` to support all trigger types (`cron`, `loop`, `once`, `watch`) and all executor types (`shell`, `claude-code`, `amplifier`) via CLI flags, with comprehensive help text for agent self-discovery.

**Architecture:** Single file change — `internal/cli/jobs.go`. Validation is extracted into a pure `validateAddOpts(opts, changedFlags)` function for testability. The `addCmd.RunE` reads flags, calls the validator, then builds typed config structs before POSTing to the daemon API.

**Tech Stack:** Go, cobra (`github.com/spf13/cobra`), stdlib (`strings`, `time`)

**Spec:** `docs/superpowers/specs/2026-03-19-file-watch-cli-design.md`

---

## File Map

| File | Action | What changes |
|---|---|---|
| `internal/cli/jobs.go` | **Modify** | `splitTrimmed` helper, `addOpts` struct, `validateAddOpts`, refactored `init()`, refactored `addCmd.RunE` |
| `internal/cli/jobs_test.go` | **Create** | Tests for `splitTrimmed` and `validateAddOpts` |

No other files change.

---

## Running Tests

```bash
cd /Users/ken/workspace/ms/loom
go test ./internal/cli/... -v
```

---

## Task 1: `splitTrimmed` helper

**Files:**
- Create: `internal/cli/jobs_test.go`
- Modify: `internal/cli/jobs.go` (add after imports)

- [ ] **Step 1: Write the failing test**

Create `internal/cli/jobs_test.go`:

```go
package cli

import (
	"testing"
)

func TestSplitTrimmed(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"single token", "create", []string{"create"}},
		{"two tokens", "create,write", []string{"create", "write"}},
		{"tokens with spaces", "create, write, remove", []string{"create", "write", "remove"}},
		{"trailing comma", "create,", []string{"create"}},
		{"comma only", ",", nil},
		{"spaces between commas", " , ", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitTrimmed(tc.input, ",")
			if len(got) != len(tc.want) {
				t.Fatalf("splitTrimmed(%q) = %v (len %d), want %v (len %d)",
					tc.input, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitTrimmed(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test — expect FAIL (function not defined)**

```bash
go test ./internal/cli/... -run TestSplitTrimmed -v
```

Expected: `./jobs_test.go:XX: undefined: splitTrimmed`

- [ ] **Step 3: Add `splitTrimmed` to `jobs.go`**

Add this after the import block in `internal/cli/jobs.go`:

```go
// splitTrimmed splits s on sep, trims whitespace from each token, and discards
// empty tokens. Returns nil (not []string{}) when the result is empty so the
// backend applies its all-events default.
func splitTrimmed(s, sep string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
```

Also add `"strings"` to the import block if not already present.

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./internal/cli/... -run TestSplitTrimmed -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/cli/jobs.go internal/cli/jobs_test.go
git commit -m "feat(cli): add splitTrimmed helper with tests"
```

---

## Task 2: Validation function

**Files:**
- Modify: `internal/cli/jobs.go` (add `addOpts` struct + `validateAddOpts` function)
- Modify: `internal/cli/jobs_test.go` (add validation tests)

- [ ] **Step 1: Write the failing tests**

First update the import block in `internal/cli/jobs_test.go` (add `"strings"`):

```go
import (
	"strings"
	"testing"
)
```

Then append `TestValidateAddOpts` to `internal/cli/jobs_test.go`:

```go
func TestValidateAddOpts(t *testing.T) {
	// changed() helper — simulates cmd.Flags().Changed()
	changed := func(flags ...string) map[string]bool {
		m := make(map[string]bool)
		for _, f := range flags {
			m[f] = true
		}
		return m
	}
	none := map[string]bool{}

	tests := []struct {
		name        string
		opts        addOpts
		changedFlags map[string]bool
		wantErr     string
	}{
		// name required
		{
			name:    "missing name",
			opts:    addOpts{triggerType: "once", executorType: "shell", command: "echo hi"},
			changedFlags: none,
			wantErr: "--name is required",
		},
		// invalid trigger
		{
			name:    "invalid trigger",
			opts:    addOpts{name: "x", triggerType: "foobar", executorType: "shell", command: "echo"},
			changedFlags: none,
			wantErr: `invalid trigger "foobar"`,
		},
		// invalid executor
		{
			name:    "invalid executor",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "bash", command: "echo"},
			changedFlags: none,
			wantErr: `invalid executor "bash"`,
		},
		// shell requires command
		{
			name:    "shell missing command",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "shell"},
			changedFlags: none,
			wantErr: `--command is required for executor "shell"`,
		},
		// command only valid for shell
		{
			name:    "command with claude-code",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "claude-code", command: "echo", prompt: "hi"},
			changedFlags: changed("command"),
			wantErr: "--command is only valid with --executor shell",
		},
		// claude-code requires prompt
		{
			name:    "claude-code missing prompt",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "claude-code"},
			changedFlags: none,
			wantErr: `--prompt is required for executor "claude-code"`,
		},
		// amplifier requires prompt or recipe
		{
			name:    "amplifier missing both",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "amplifier"},
			changedFlags: none,
			wantErr: `--prompt or --recipe is required for executor "amplifier"`,
		},
		// recipe only for amplifier
		{
			name:    "recipe with shell",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "shell", command: "echo", recipe: "r.yaml"},
			changedFlags: changed("recipe"),
			wantErr: "--recipe is only valid with --executor amplifier",
		},
		// model only for AI executors
		{
			name:    "model with shell",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "shell", command: "echo", model: "opus"},
			changedFlags: changed("model"),
			wantErr: "--model is only valid with --executor claude-code or amplifier",
		},
		// watch requires watch-path
		{
			name:    "watch missing watch-path",
			opts:    addOpts{name: "x", triggerType: "watch", executorType: "shell", command: "echo"},
			changedFlags: none,
			wantErr: "--watch-path is required when --trigger watch",
		},
		// watch flags require trigger watch (use watch-recursive as example)
		{
			name:    "watch-recursive without watch trigger",
			opts:    addOpts{name: "x", triggerType: "once", executorType: "shell", command: "echo", watchRecursive: true},
			changedFlags: changed("watch-recursive"),
			wantErr: "--watch-recursive requires --trigger watch",
		},
		// watch-mode must be notify or poll
		{
			name:    "invalid watch-mode",
			opts:    addOpts{name: "x", triggerType: "watch", executorType: "shell", command: "echo", watchPath: "/tmp", watchMode: "inotify"},
			changedFlags: changed("watch-mode"),
			wantErr: `invalid --watch-mode "inotify"`,
		},
		// invalid watch-events token
		{
			name:    "invalid watch-events token",
			opts:    addOpts{name: "x", triggerType: "watch", executorType: "shell", command: "echo", watchPath: "/tmp", watchMode: "notify", watchEvents: "created"},
			changedFlags: changed("watch-events"),
			wantErr: `invalid event "created"`,
		},
		// rename unsupported in poll mode
		{
			name:    "poll mode with rename event",
			opts:    addOpts{name: "x", triggerType: "watch", executorType: "shell", command: "echo", watchPath: "/tmp", watchMode: "poll", watchEvents: "rename"},
			changedFlags: changed("watch-mode", "watch-events"),
			wantErr: `--watch-mode poll does not support event "rename"`,
		},
		// watch-poll-interval requires watch-mode poll
		{
			name:    "poll-interval without poll mode",
			opts:    addOpts{name: "x", triggerType: "watch", executorType: "shell", command: "echo", watchPath: "/tmp", watchMode: "notify", watchPollInterval: "2s"},
			changedFlags: changed("watch-poll-interval"),
			wantErr: "--watch-poll-interval requires --watch-mode poll",
		},
		// invalid duration for watch-debounce
		{
			name:    "invalid watch-debounce duration",
			opts:    addOpts{name: "x", triggerType: "watch", executorType: "shell", command: "echo", watchPath: "/tmp", watchMode: "notify", watchDebounce: "five seconds"},
			changedFlags: changed("watch-debounce"),
			wantErr: `invalid --watch-debounce "five seconds"`,
		},
		// valid shell+watch — no error
		{
			name: "valid shell+watch",
			opts: addOpts{
				name: "watcher", triggerType: "watch", executorType: "shell",
				command: "make", watchPath: "/tmp", watchMode: "notify",
			},
			changedFlags: none,
			wantErr:     "",
		},
		// valid claude+cron — no error
		{
			name: "valid claude+cron",
			opts: addOpts{
				name: "daily", triggerType: "cron", executorType: "claude-code",
				prompt: "summarize", schedule: "0 0 9 * * *",
			},
			changedFlags: none,
			wantErr:     "",
		},
		// valid amplifier+watch with recipe only — no error
		{
			name: "valid amplifier+watch recipe only",
			opts: addOpts{
				name: "proc", triggerType: "watch", executorType: "amplifier",
				recipe: "r.yaml", watchPath: "/tmp", watchMode: "notify",
			},
			changedFlags: none,
			wantErr:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAddOpts(tc.opts, tc.changedFlags)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test — expect FAIL (type/function not defined)**

```bash
go test ./internal/cli/... -run TestValidateAddOpts -v
```

Expected: compilation errors for undefined `addOpts` and `validateAddOpts`

- [ ] **Step 3: Add `addOpts` struct and `validateAddOpts` to `jobs.go`**

Add after `splitTrimmed` in `internal/cli/jobs.go`:

```go
// addOpts holds all parsed flag values for the add command.
type addOpts struct {
	name              string
	description       string
	triggerType       string
	schedule          string
	executorType      string
	command           string
	prompt            string
	recipe            string
	model             string
	cwd               string
	timeout           string
	retries           int
	watchPath         string
	watchRecursive    bool
	watchEvents       string
	watchDebounce     string
	watchMode         string
	watchPollInterval string
}

// validTriggers and validExecutors are the accepted string values.
var validTriggers = map[string]bool{
	"cron": true, "loop": true, "once": true, "watch": true,
}
var validExecutors = map[string]bool{
	"shell": true, "claude-code": true, "amplifier": true,
}

// validWatchEvents is the full set of accepted event tokens (including aliases).
var validWatchEvents = map[string]bool{
	"create": true, "write": true, "modify": true,
	"remove": true, "delete": true, "rename": true, "chmod": true,
}

// pollUnsupportedEvents are events the poll watcher cannot detect.
var pollUnsupportedEvents = map[string]bool{"rename": true, "chmod": true}

// validateAddOpts validates flag combinations before constructing a job or calling
// the API. changedFlags simulates cobra's cmd.Flags().Changed() — keys are flag
// names that were explicitly set by the user (not just defaulted).
func validateAddOpts(opts addOpts, changedFlags map[string]bool) error {
	// 1. name
	if opts.name == "" {
		return fmt.Errorf("--name is required")
	}
	// 2. trigger value
	if !validTriggers[opts.triggerType] {
		return fmt.Errorf("invalid trigger %q: must be cron, loop, once, or watch", opts.triggerType)
	}
	// 3. executor value
	if !validExecutors[opts.executorType] {
		return fmt.Errorf("invalid executor %q: must be shell, claude-code, or amplifier", opts.executorType)
	}
	// 4. executor cross-checks
	if changedFlags["command"] && opts.executorType != "shell" {
		return fmt.Errorf("--command is only valid with --executor shell")
	}
	if changedFlags["recipe"] && opts.executorType != "amplifier" {
		return fmt.Errorf("--recipe is only valid with --executor amplifier")
	}
	if changedFlags["model"] && opts.executorType == "shell" {
		return fmt.Errorf("--model is only valid with --executor claude-code or amplifier")
	}
	switch opts.executorType {
	case "shell":
		if opts.command == "" {
			return fmt.Errorf("--command is required for executor \"shell\"")
		}
	case "claude-code":
		if opts.prompt == "" {
			return fmt.Errorf("--prompt is required for executor \"claude-code\"")
		}
	case "amplifier":
		if opts.prompt == "" && opts.recipe == "" {
			return fmt.Errorf("--prompt or --recipe is required for executor \"amplifier\"")
		}
	}
	// 5. watch-flag guards — must use Changed() semantics (not value checks)
	//    because --watch-mode defaults to "notify" (non-empty).
	watchFlagNames := []string{
		"watch-path", "watch-recursive", "watch-events",
		"watch-debounce", "watch-mode", "watch-poll-interval",
	}
	for _, f := range watchFlagNames {
		if changedFlags[f] && opts.triggerType != "watch" {
			return fmt.Errorf("--%s requires --trigger watch", f)
		}
	}
	if opts.triggerType == "watch" && opts.watchPath == "" {
		return fmt.Errorf("--watch-path is required when --trigger watch")
	}
	if changedFlags["watch-mode"] {
		if opts.watchMode != "notify" && opts.watchMode != "poll" {
			return fmt.Errorf("invalid --watch-mode %q: must be notify or poll", opts.watchMode)
		}
	}
	// 6. duration format checks (only when explicitly set)
	if changedFlags["watch-debounce"] {
		if _, err := time.ParseDuration(opts.watchDebounce); err != nil {
			return fmt.Errorf("invalid --watch-debounce %q: use time.ParseDuration format, e.g. \"500ms\", \"1s\"", opts.watchDebounce)
		}
	}
	if changedFlags["watch-poll-interval"] {
		if _, err := time.ParseDuration(opts.watchPollInterval); err != nil {
			return fmt.Errorf("invalid --watch-poll-interval %q: use time.ParseDuration format, e.g. \"2s\", \"500ms\"", opts.watchPollInterval)
		}
	}
	// 7. watch-events token validation (only when non-empty)
	if opts.watchEvents != "" {
		tokens := splitTrimmed(opts.watchEvents, ",")
		for _, tok := range tokens {
			if !validWatchEvents[tok] {
				return fmt.Errorf("invalid event %q: valid events are create, write, modify, remove, delete, rename, chmod", tok)
			}
		}
		// 7b. poll mode doesn't support rename/chmod
		if opts.watchMode == "poll" {
			for _, tok := range tokens {
				if pollUnsupportedEvents[tok] {
					return fmt.Errorf("--watch-mode poll does not support event %q: poll mode detects only create, write, remove", tok)
				}
			}
		}
	}
	// 8. poll-interval requires poll mode
	if changedFlags["watch-poll-interval"] && opts.watchMode != "poll" {
		return fmt.Errorf("--watch-poll-interval requires --watch-mode poll")
	}
	return nil
}
```

Also add `"time"` to the import block if not already present.

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./internal/cli/... -run TestValidateAddOpts -v
```

Expected: all subtests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/jobs.go internal/cli/jobs_test.go
git commit -m "feat(cli): add addOpts struct and validateAddOpts with full test coverage"
```

---

## Task 3: Update `init()` — flag registrations

**Files:**
- Modify: `internal/cli/jobs.go` — `init()` function at the bottom

- [ ] **Step 1: No automated test** — the `init()` changes are purely declarative (registering flags with cobra). Correctness is verified by the build + help-text spot-check in Task 4 Step 3, not a unit test. Skip directly to implementation.

- [ ] **Step 2: Replace the existing `addCmd.Flags()` block in `init()`**

Find the current block (lines ~208–215):

```go
addCmd.Flags().String("name", "", "Job name (required)")
addCmd.Flags().String("description", "", "Job description")
addCmd.Flags().String("trigger", "once", "Trigger type: cron, loop, once")
addCmd.Flags().String("schedule", "", "Cron expression or duration (e.g. 5m)")
addCmd.Flags().String("command", "", "Shell command to run (required)")
addCmd.Flags().String("cwd", "", "Working directory")
addCmd.Flags().String("timeout", "", "Max execution time (e.g. 30s, 5m)")
addCmd.Flags().Int("retries", 0, "Number of retries on failure")
```

Replace with:

```go
addCmd.Flags().String("name", "", "Job name (required)")
addCmd.Flags().String("description", "", "Job description")
addCmd.Flags().String("trigger", "once",
	"Trigger type: cron, loop, once, watch (default \"once\")\n"+
		"  cron:  runs on a cron schedule (--schedule required)\n"+
		"  loop:  repeats on an interval (--schedule required, e.g. 5m)\n"+
		"  once:  runs once then auto-disables (--schedule = optional delay)\n"+
		"  watch: fires when files change (--watch-path required)")
addCmd.Flags().String("schedule", "", "Cron expression or duration (e.g. 5m)")
addCmd.Flags().String("cwd", "", "Working directory")
addCmd.Flags().String("timeout", "", "Max execution time (e.g. 30s, 5m)")
addCmd.Flags().Int("retries", 0, "Number of retries on failure")

// Executor selection
addCmd.Flags().String("executor", "shell", "Executor type: shell, claude-code, amplifier (default \"shell\")")
addCmd.Flags().String("command", "", "Shell command to run (required for --executor shell)")
addCmd.Flags().String("prompt", "", "AI prompt text (required for --executor claude-code;\n  required for --executor amplifier unless --recipe is set)")
addCmd.Flags().String("recipe", "", "Path to .yaml recipe file (--executor amplifier only;\n  may be combined with --prompt)")
addCmd.Flags().String("model", "", "Model override, e.g. \"sonnet\", \"opus\"\n  (AI executors only: claude-code, amplifier)")

// Watch trigger flags
addCmd.Flags().String("watch-path", "", "File or directory to watch (required for --trigger watch)")
addCmd.Flags().Bool("watch-recursive", false, "Watch subdirectories recursively")
addCmd.Flags().String("watch-events", "",
	"Comma-separated events to react to:\n"+
		"  create, write, modify (=write), remove, delete (=remove), rename, chmod\n"+
		"  Empty or omitted means all events.\n"+
		"  Note: rename and chmod are not supported by --watch-mode poll.")
addCmd.Flags().String("watch-debounce", "", "Quiet window before firing after last event, e.g. \"500ms\"\n  (backend default: 300ms)")
addCmd.Flags().String("watch-mode", "notify",
	"\"notify\" uses OS file events (inotify/FSEvents/kqueue);\n"+
		"\"poll\" checks on a fixed interval (default \"notify\")")
addCmd.Flags().String("watch-poll-interval", "", "Polling interval for --watch-mode poll, e.g. \"2s\"\n  (only valid with --watch-mode poll; backend default: 2s)")
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/cli/...
```

Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/cli/jobs.go
git commit -m "feat(cli): register new executor and watch flags in init()"
```

---

## Task 4: Refactor `addCmd.RunE`

**Files:**
- Modify: `internal/cli/jobs.go` — `addCmd.RunE` function and `addCmd.Example`

- [ ] **Step 0: Confirm existing tests still cover the validator**

```bash
go test ./internal/cli/... -run TestValidateAddOpts -v
```

Expected: all subtests PASS. These tests are the RED anchor for the wiring — validation is already covered. The `cmd.Flags().Changed()` wiring layer cannot be unit-tested without a live `*cobra.Command`; correctness is verified by the help-text spot-check and validation smoke-checks in Steps 3-4 below.

- [ ] **Step 1: Replace `addCmd.RunE` entirely**

Replace the existing `RunE` with the new implementation below. Also update `addCmd.Example` at the same time.

New `addCmd`:

```go
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new job",
	Example: `  # Shell command on a cron schedule
  loom add --name "Nightly cleanup" --trigger cron --schedule "0 0 2 * * *" \
    --command "find /tmp -mtime +7 -delete"

  # Shell command repeating every 5 minutes
  loom add --name "Health check" --trigger loop --schedule 5m \
    --command "curl -sf http://localhost:8080/health"

  # Shell command when files change in a directory (with debounce)
  loom add --name "Auto-lint" --trigger watch --watch-path ./src \
    --watch-recursive --watch-debounce 500ms --command "npm run lint"

  # Shell command watching for specific events only
  loom add --name "On new file" --trigger watch --watch-path ~/inbox \
    --watch-events create --command "/usr/local/bin/process-new.sh"

  # Shell command reacting to multiple events (comma-separated, no spaces)
  loom add --name "Compile on change" --trigger watch --watch-path ./src \
    --watch-events "create,write" --command "make build"

  # Shell command with explicit notify mode and rename/chmod events
  loom add --name "Perms watcher" --trigger watch --watch-path /etc/app \
    --watch-mode notify --watch-events "rename,chmod" --command "/usr/local/bin/audit.sh"

  # Watch a network share using poll mode (OS events not available)
  loom add --name "NFS watcher" --trigger watch --watch-path /mnt/share \
    --watch-mode poll --watch-poll-interval 5s --command "/usr/local/bin/sync.sh"

  # Claude prompt on a cron schedule (with model override)
  loom add --name "Daily standup" --trigger cron --schedule "0 0 9 * * *" \
    --executor claude-code --model opus --prompt "Summarize my open GitHub issues and PRs"

  # Claude prompt when files change
  loom add --name "Review on save" --trigger watch --watch-path ./src \
    --watch-recursive --watch-events write --watch-debounce 1s \
    --executor claude-code --prompt "Review the changed file for issues"

  # Claude prompt repeating on an interval
  loom add --name "Periodic check" --trigger loop --schedule 30m \
    --executor claude-code --prompt "Check for any new alerts in my monitoring dashboard"

  # Claude prompt run once immediately
  loom add --name "Onboarding summary" --trigger once \
    --executor claude-code --prompt "Summarize the onboarding docs in ~/docs/onboarding"

  # Amplifier recipe on an interval
  loom add --name "Hourly digest" --trigger loop --schedule 1h \
    --executor amplifier --recipe ~/recipes/digest.yaml

  # Amplifier recipe triggered by file watch
  loom add --name "Process inbox" --trigger watch --watch-path ~/inbox \
    --executor amplifier --recipe ~/recipes/process-inbox.yaml

  # Amplifier prompt with model on a cron schedule
  loom add --name "Weekly review" --trigger cron --schedule "0 0 9 * * 1" \
    --executor amplifier --model sonnet --prompt "Run my weekly review workflow"

  # Amplifier recipe + additional prompt instruction (both may be set)
  loom add --name "Guided process" --trigger watch --watch-path ~/docs \
    --executor amplifier --recipe ~/recipes/process.yaml \
    --prompt "Focus on files modified in the last hour"

  # Amplifier recipe run once with a delay
  loom add --name "Post-deploy check" --trigger once --schedule 5m \
    --executor amplifier --recipe ~/recipes/post-deploy.yaml

  # Run once immediately (default trigger, shell)
  loom add --name "Migrate DB" --command "/usr/local/bin/migrate.sh"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")

		// Read all flags into opts struct.
		opts := addOpts{}
		opts.name, _ = cmd.Flags().GetString("name")
		opts.description, _ = cmd.Flags().GetString("description")
		opts.triggerType, _ = cmd.Flags().GetString("trigger")
		opts.schedule, _ = cmd.Flags().GetString("schedule")
		opts.executorType, _ = cmd.Flags().GetString("executor")
		opts.command, _ = cmd.Flags().GetString("command")
		opts.prompt, _ = cmd.Flags().GetString("prompt")
		opts.recipe, _ = cmd.Flags().GetString("recipe")
		opts.model, _ = cmd.Flags().GetString("model")
		opts.cwd, _ = cmd.Flags().GetString("cwd")
		opts.timeout, _ = cmd.Flags().GetString("timeout")
		opts.retries, _ = cmd.Flags().GetInt("retries")
		opts.watchPath, _ = cmd.Flags().GetString("watch-path")
		opts.watchRecursive, _ = cmd.Flags().GetBool("watch-recursive")
		opts.watchEvents, _ = cmd.Flags().GetString("watch-events")
		opts.watchDebounce, _ = cmd.Flags().GetString("watch-debounce")
		opts.watchMode, _ = cmd.Flags().GetString("watch-mode")
		opts.watchPollInterval, _ = cmd.Flags().GetString("watch-poll-interval")

		// Build changedFlags map for validation (uses Changed() semantics).
		changedFlags := make(map[string]bool)
		for _, f := range []string{
			"command", "recipe", "model",
			"watch-path", "watch-recursive", "watch-events",
			"watch-debounce", "watch-mode", "watch-poll-interval",
		} {
			changedFlags[f] = cmd.Flags().Changed(f)
		}

		// Validate.
		if err := validateAddOpts(opts, changedFlags); err != nil {
			return err
		}

		// Build job.
		job := types.Job{
			Name:        opts.name,
			Description: opts.description,
			Trigger: types.Trigger{
				Type:     types.TriggerType(opts.triggerType),
				Schedule: opts.schedule,
			},
			Executor:   types.ExecutorType(opts.executorType),
			CWD:        opts.cwd,
			Timeout:    opts.timeout,
			MaxRetries: opts.retries,
			Enabled:    true,
		}

		// Executor config.
		switch job.Executor {
		case types.ExecutorShell:
			job.Shell = &types.ShellConfig{Command: opts.command}
		case types.ExecutorClaudeCode:
			job.ClaudeCode = &types.ClaudeCodeConfig{
				Prompt: opts.prompt,
				Model:  opts.model,
			}
		case types.ExecutorAmplifier:
			job.Amplifier = &types.AmplifierConfig{
				Prompt:     opts.prompt,
				RecipePath: opts.recipe,
				Model:      opts.model,
			}
		}

		// Watch config.
		if job.Trigger.Type == types.TriggerWatch {
			job.Watch = &types.WatchConfig{
				Path:         opts.watchPath,
				Recursive:    opts.watchRecursive,
				Events:       splitTrimmed(opts.watchEvents, ","),
				Mode:         opts.watchMode,
				PollInterval: opts.watchPollInterval,
				Debounce:     opts.watchDebounce,
			}
		}

		body, _ := json.Marshal(job)
		url := fmt.Sprintf("http://localhost:%d/api/jobs", port)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("daemon not reachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var errResp map[string]string
			json.NewDecoder(resp.Body).Decode(&errResp)
			return fmt.Errorf("error: %s", errResp["error"])
		}

		var created types.Job
		json.NewDecoder(resp.Body).Decode(&created)
		fmt.Printf("✓ Job created: %s (id: %s)\n", created.Name, created.ID)
		return nil
	},
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./internal/cli/... -v
```

Expected: all tests PASS

- [ ] **Step 3: Verify it compiles and help text looks right**

```bash
go build ./... && go run ./cmd/loom add --help
```

Expected: help text shows all new flags with descriptions. Verify `--trigger` lists `watch`, `--executor` is present, `--watch-path` etc. are present.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/jobs.go
git commit -m "feat(cli): refactor addCmd.RunE — all executors and triggers, typed structs, agent-friendly help"
```

---

## Task 5: Final verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: `ok github.com/ms/loom/internal/cli`

- [ ] **Step 2: Build the binary**

```bash
go build -o /tmp/loom-test ./cmd/loom
```

Expected: binary produced with no warnings

- [ ] **Step 3: Spot-check help text**

```bash
/tmp/loom-test add --help
```

Verify:
- `--trigger` description mentions `watch`
- `--executor` flag is listed
- `--watch-path` through `--watch-poll-interval` all appear
- `--prompt`, `--recipe`, `--model` all appear
- Example block shows all three executors

- [ ] **Step 4: Spot-check validation errors**

```bash
# Should error: watch-path required
/tmp/loom-test add --name "x" --trigger watch --command "echo" 2>&1
# Expected: --watch-path is required when --trigger watch

# Should error: prompt required for claude-code
/tmp/loom-test add --name "x" --executor claude-code 2>&1
# Expected: --prompt is required for executor "claude-code"

# Should error: invalid executor
/tmp/loom-test add --name "x" --executor bash --command "echo" 2>&1
# Expected: invalid executor "bash"
```

- [ ] **Step 5: Commit (if anything was adjusted)**

```bash
git add -A
git commit -m "chore: final cleanup" --allow-empty
```
