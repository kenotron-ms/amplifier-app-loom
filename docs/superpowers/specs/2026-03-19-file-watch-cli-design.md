# CLI: Full Trigger & Executor Support for `loom add`

**Date:** 2026-03-19  
**Status:** Approved  
**File changed:** `internal/cli/jobs.go` (only)

---

## Problem

`loom add` is incomplete. It only supports the `shell` executor (via the deprecated
top-level `Command` field) and only documents `cron`, `loop`, `once` as trigger options. The
`watch` trigger and the `claude-code`/`amplifier` executors are fully implemented in the backend
but unreachable from the CLI — only accessible via the REST API or the web UI.

---

## Goal

Extend `loom add` so every trigger type (`cron`, `loop`, `once`, `watch`) and every
executor type (`shell`, `claude-code`, `amplifier`) can be configured from the CLI. Help text must
be comprehensive enough for agents to self-discover all combinations without consulting external
documentation.

---

## Approach: Flat flags on `add`

Single command, all options as flags. Backward compatible — existing scripts using `--command`
continue to work unchanged.

---

## New Flags

### Executor selection

| Flag | Type | Default | Description |
|---|---|---|---|
| `--executor` | string | `shell` | Executor type: `shell`, `claude-code`, `amplifier` |

### AI executor flags

| Flag | Type | Description |
|---|---|---|
| `--prompt` | string | Prompt text (required for `claude-code`; required for `amplifier` unless `--recipe` is set) |
| `--recipe` | string | Path to `.yaml` recipe file (`amplifier` only; may be combined with `--prompt`) |
| `--model` | string | Model override, e.g. `sonnet`, `opus` (AI executors only — invalid with `--executor shell`) |

### Watch trigger flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--watch-path` | string | *(required)* | File or directory to watch |
| `--watch-recursive` | bool | `false` | Watch subdirectories recursively |
| `--watch-events` | string | *(all)* | Comma-separated: `create,write,modify,remove,delete,rename,chmod`. `modify`=`write`, `delete`=`remove`. Empty = all events. `rename`/`chmod` unsupported by `poll` mode. |
| `--watch-debounce` | string | `""` | Quiet window before firing, e.g. `500ms` (backend default: 300ms) |
| `--watch-mode` | string | `notify` | `notify` (OS-level inotify/FSEvents) or `poll` |
| `--watch-poll-interval` | string | `""` | Poll mode only, e.g. `2s` |

---

## Validation Rules

Enforced in `addCmd.RunE` before the API call, with explicit error messages:

| Rule | Error message |
|---|---|
| `--name` always required | `--name is required` |
| `--trigger` must be one of the four valid values | `invalid trigger "X": must be cron, loop, once, or watch` |
| `--executor` must be one of the three valid values | `invalid executor "X": must be shell, claude-code, or amplifier` |
| `--executor shell` requires `--command` | `--command is required for executor "shell"` |
| `--command` only valid for `--executor shell` | `--command is only valid with --executor shell` |
| `--executor claude-code` requires `--prompt` | `--prompt is required for executor "claude-code"` |
| `--executor amplifier` requires `--prompt` or `--recipe` (or both) | `--prompt or --recipe is required for executor "amplifier"` |
| `--recipe` only valid for `amplifier` | `--recipe is only valid with --executor amplifier` |
| `--model` only valid for `claude-code` or `amplifier` | `--model is only valid with --executor claude-code or amplifier` |
| `--trigger watch` requires `--watch-path` | `--watch-path is required when --trigger watch` |
| Any `--watch-<flag>` requires `--trigger watch` | `--watch-<flag> requires --trigger watch` (message uses the actual flag name, e.g. `--watch-recursive requires --trigger watch`) |
| `--watch-mode` must be `notify` or `poll` | `invalid --watch-mode "X": must be notify or poll` |
| `--watch-poll-interval` requires `--watch-mode poll` | `--watch-poll-interval requires --watch-mode poll` |
| Each token in `--watch-events` must be valid (only when flag is non-empty) | `invalid event "X": valid events are create, write, modify, remove, delete, rename, chmod` |
| `rename`/`chmod` events are unsupported with `--watch-mode poll` | `--watch-mode poll does not support event "rename": poll mode detects only create, write, remove` |
| `--watch-debounce` must be a valid Go duration | `invalid --watch-debounce "X": use time.ParseDuration format, e.g. "500ms", "1s"` |
| `--watch-poll-interval` must be a valid Go duration | `invalid --watch-poll-interval "X": use time.ParseDuration format, e.g. "2s", "500ms"` |

**Watch-flag guard — use `cmd.Flags().Changed()`, not value comparison:** All six `--watch-*`
flags must be detected via `cmd.Flags().Changed("watch-<flag>")`, not by checking whether the
value is non-empty. This is critical for `--watch-mode` whose default is `"notify"` (a non-empty
string) — a value check would falsely fire for every non-watch job. Apply `Changed()` uniformly
to all six watch flags so the pattern is consistent:

```go
if cmd.Flags().Changed("watch-path") && triggerType != "watch" {
    return fmt.Errorf("--watch-path requires --trigger watch")
}
// repeat for watch-recursive, watch-events, watch-debounce, watch-mode, watch-poll-interval
```

**Duration validation:** Pre-validate `--watch-debounce` and `--watch-poll-interval` with
`time.ParseDuration` **only when the flag was explicitly set** (`cmd.Flags().Changed(...)`). An
empty string (flag not provided) is valid and means "use backend default." Do not call
`time.ParseDuration("")` — it returns an error. The backend guards the same way
(`watcher.go:31-34`). Validation error message:
`invalid --watch-debounce "X": use time.ParseDuration format, e.g. "500ms", "1s"`

**`--watch-events` token validation:** Only validate tokens when `--watch-events` is non-empty.
An empty string (flag not provided or set to `""`) produces a nil slice via `splitTrimmed` and
the backend applies the all-events default. No validation needed for the empty case.

Valid tokens include backend aliases: `create`, `write`, `modify` (alias for `write`),
`remove`, `delete` (alias for `remove`), `rename`, `chmod`. Accept all seven in the CLI validator
to match backend parity (`watcher.go:322-325`).

**Validation execution order** (implement checks in this sequence in `addCmd.RunE`):
1. `--name` required
2. `--trigger` valid value
3. `--executor` valid value
4. Executor cross-checks: `--command` required/exclusive, `--prompt`/`--recipe` required, `--recipe` only for amplifier, `--model` only for AI executors
5. Watch-flag guards: any `--watch-*` Changed() requires `--trigger watch`; `--watch-path` required when trigger is watch; `--watch-mode` valid value
6. Duration format checks: `--watch-debounce` and `--watch-poll-interval` (non-empty only)
7. Watch-events token validation (non-empty only)
7b. If `--watch-mode poll` and any token in `--watch-events` is `rename` or `chmod`, return error: `--watch-mode poll does not support event "X": poll mode detects only create, write, remove`
8. `--watch-poll-interval` requires `--watch-mode poll`

**`splitTrimmed` helper:** Implement as a package-level function in `jobs.go`:
```go
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

**`init()` changes required** — update two existing registrations and add ten new ones:

```go
// UPDATE existing:
addCmd.Flags().String("trigger", "once", "Trigger type: cron, loop, once, watch (default \"once\")\n  cron: runs on a cron schedule (--schedule required)\n  loop: repeats on an interval (--schedule required, e.g. 5m)\n  once: runs once then auto-disables (--schedule = optional delay)\n  watch: fires when files change (--watch-path required)")
addCmd.Flags().String("command", "", "Shell command to run (required for --executor shell)")

// ADD new executor flags:
addCmd.Flags().String("executor", "shell", "Executor type: shell, claude-code, amplifier (default \"shell\")")
addCmd.Flags().String("prompt", "", "AI prompt text (required for --executor claude-code; required for --executor amplifier unless --recipe is set)")
addCmd.Flags().String("recipe", "", "Path to .yaml recipe file (--executor amplifier only; may be combined with --prompt)")
addCmd.Flags().String("model", "", "Model override, e.g. \"sonnet\", \"opus\" (AI executors only: claude-code, amplifier)")

// ADD new watch flags:
addCmd.Flags().String("watch-path", "", "File or directory to watch (required for --trigger watch)")
addCmd.Flags().Bool("watch-recursive", false, "Watch subdirectories recursively")
addCmd.Flags().String("watch-events", "", "Comma-separated events: create,write,modify,remove,delete,rename,chmod\n  modify=write, delete=remove; empty means all events\n  Note: rename and chmod are not supported by --watch-mode poll")
addCmd.Flags().String("watch-debounce", "", "Quiet window before firing after last event, e.g. \"500ms\" (backend default: 300ms)")
addCmd.Flags().String("watch-mode", "notify", "\"notify\" uses OS file events (inotify/FSEvents/kqueue); \"poll\" checks on a fixed interval (default \"notify\")")
addCmd.Flags().String("watch-poll-interval", "", "Polling interval for --watch-mode poll, e.g. \"2s\" (only valid with --watch-mode poll; backend default: 2s)")
```

**Note on `--schedule` with `cron`/`loop`:** The CLI does **not** validate whether `--schedule`
is present for `cron` or `loop` triggers. The backend's scheduler will fail at registration time
with a clear error that propagates through the API response. No duplicate CLI validation needed.

**Note on `--prompt` + `--recipe` for `amplifier`:** Both may be set simultaneously.
`AmplifierConfig` supports both fields; the backend uses `RecipePath` to load the recipe and
`Prompt` as an additional instruction. This is a valid combination and is not an error.

---

## Job Struct Construction

```go
// Executor field uses named type cast to match types.Job.Executor (ExecutorType, not string)
job.Executor = types.ExecutorType(executorType)

switch types.ExecutorType(executorType) {
case types.ExecutorShell:
    job.Shell = &types.ShellConfig{Command: command}
case types.ExecutorClaudeCode:
    job.ClaudeCode = &types.ClaudeCodeConfig{Prompt: prompt, Model: model}
case types.ExecutorAmplifier:
    job.Amplifier = &types.AmplifierConfig{Prompt: prompt, RecipePath: recipe, Model: model}
}

// Watch config — only set when trigger is watch
if types.TriggerType(triggerType) == types.TriggerWatch {
    job.Watch = &types.WatchConfig{
        Path:         watchPath,
        Recursive:    watchRecursive,
        Events:       splitTrimmed(watchEvents, ","),
        Mode:         watchMode,
        PollInterval: watchPollInterval,
        Debounce:     watchDebounce,
    }
}
```

**`splitTrimmed(s, sep)`:** Split `s` on `sep`, then call `strings.TrimSpace` on each token, then
discard empty tokens. If the result is an empty slice (flag not set or set to `""`), assign `nil`
(not `[]string{}`) so the backend applies its all-events default.

**Field order** matches `types.WatchConfig` declaration order: `Path`, `Recursive`, `Events`,
`Mode`, `PollInterval`, `Debounce`.

`JOB_WATCH_PATH` and `JOB_EVENT_PATH` env vars are injected by the scheduler at dispatch time —
no CLI changes needed for that.

---

## Bug Fixes (bundled)

**Bug 1 — deprecated `Command` field + unconditional guard:** Two changes together:
1. Remove the unconditional `if command == "" { return fmt.Errorf("--command is required") }` guard at `addCmd.RunE:92-94`. It is replaced by the new executor cross-check at validation step 4.
2. Change `job.Command = command` (top-level deprecated field) to `job.Shell = &ShellConfig{Command: command}` with `job.Executor = types.ExecutorShell`.

**Bug 2 — dead `"immediate"` branch:** The `--trigger` flag defaults to `"once"`, making the
`if triggerType == "" { triggerType = "immediate" }` block unreachable. Remove it. Note:
`"immediate"` is intentionally retained as a legacy alias in the backend scheduler
(`scheduler.go:130` — `case "immediate", types.TriggerOnce:`) for backward compatibility with
existing DB entries. The CLI should never generate it; removing the CLI branch is correct.

---

## Help Text (agent self-discovery)

### Flag descriptions

```
--executor string        Executor type: shell, claude-code, amplifier (default "shell")
--command string         Shell command to run (required for --executor shell)
--prompt string          AI prompt text (required for --executor claude-code;
                           required for --executor amplifier unless --recipe is set)
--recipe string          Path to .yaml recipe file (--executor amplifier only;
                           may be combined with --prompt)
--model string           Model override, e.g. "sonnet", "opus"
                           (AI executors only: claude-code, amplifier)

--trigger string         Trigger type: cron, loop, once, watch (default "once")
                           cron:  runs on a cron schedule (--schedule required)
                           loop:  repeats on an interval (--schedule required, e.g. 5m)
                           once:  runs once then auto-disables (--schedule = optional delay)
                           watch: fires when files change (--watch-path required)

--watch-path string      File or directory to watch (required for --trigger watch)
--watch-recursive        Watch subdirectories recursively (default false)
--watch-events string    Comma-separated events to react to:
                           create, write, modify (=write), remove, delete (=remove), rename, chmod
                           Empty or omitted means all events.
                           Note: rename and chmod are not supported by --watch-mode poll.
--watch-debounce string  Quiet window before firing after last event, e.g. "500ms"
                           (backend default: 300ms)
--watch-mode string      "notify" uses OS file events (inotify/FSEvents/kqueue);
                           "poll" checks on a fixed interval (default "notify")
--watch-poll-interval string  Polling interval for --watch-mode poll, e.g. "2s"
                           (only valid with --watch-mode poll; backend default: 2s)
```

### Example block

```
Examples:
  # Shell command on a cron schedule
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

  # Claude prompt repeating on an interval
  loom add --name "Periodic check" --trigger loop --schedule 30m \
    --executor claude-code --prompt "Check for any new alerts in my monitoring dashboard"

  # Claude prompt run once immediately
  loom add --name "Onboarding summary" --trigger once \
    --executor claude-code --prompt "Summarize the onboarding docs in ~/docs/onboarding"

  # Amplifier recipe run once with a delay
  loom add --name "Post-deploy check" --trigger once --schedule 5m \
    --executor amplifier --recipe ~/recipes/post-deploy.yaml

  # Run once immediately (default trigger, shell)
  loom add --name "Migrate DB" --command "/usr/local/bin/migrate.sh"
```

---

## Scope

- **One file changed:** `internal/cli/jobs.go`
- **No changes to:** `internal/types/`, `internal/scheduler/`, `internal/api/`, tests
- **Backward compatible:** existing `--command` usage unchanged
