# CLI: Full Trigger & Executor Support for `agent-daemon add`

**Date:** 2026-03-19  
**Status:** Approved  
**File changed:** `internal/cli/jobs.go` (only)

---

## Problem

`agent-daemon add` is incomplete. It only supports the `shell` executor (via the deprecated
top-level `Command` field) and only documents `cron`, `loop`, `once` as trigger options. The
`watch` trigger and the `claude-code`/`amplifier` executors are fully implemented in the backend
but unreachable from the CLI — only accessible via the REST API or the web UI.

---

## Goal

Extend `agent-daemon add` so every trigger type (`cron`, `loop`, `once`, `watch`) and every
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
| `--prompt` | string | Prompt text (required for `claude-code` or `amplifier` without `--recipe`) |
| `--recipe` | string | Path to `.yaml` recipe file (`amplifier` only) |
| `--model` | string | Model override, e.g. `sonnet`, `opus` |

### Watch trigger flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--watch-path` | string | *(required)* | File or directory to watch |
| `--watch-recursive` | bool | `false` | Watch subdirectories recursively |
| `--watch-events` | string | *(all)* | Comma-separated: `create,write,remove,rename,chmod`. Empty = all. |
| `--watch-debounce` | string | `""` | Quiet window before firing, e.g. `500ms` (backend default: 300ms) |
| `--watch-mode` | string | `notify` | `notify` (OS-level inotify/FSEvents) or `poll` |
| `--watch-poll-interval` | string | `""` | Poll mode only, e.g. `2s` |

---

## Validation Rules

Enforced in `addCmd.RunE` before the API call, with explicit error messages:

| Rule | Error message |
|---|---|
| `--name` always required | `--name is required` |
| `--trigger watch` requires `--watch-path` | `--watch-path is required when --trigger watch` |
| `--watch-*` flags require `--trigger watch` | `--watch-recursive requires --trigger watch` |
| `--watch-poll-interval` requires `--watch-mode poll` | `--watch-poll-interval requires --watch-mode poll` |
| `--executor shell` requires `--command` | `--command is required for executor "shell"` |
| `--executor claude-code` requires `--prompt` | `--prompt is required for executor "claude-code"` |
| `--executor amplifier` requires `--prompt` or `--recipe` | `--prompt or --recipe is required for executor "amplifier"` |
| `--recipe` only valid for `amplifier` | `--recipe is only valid with --executor amplifier` |

---

## Job Struct Construction

```
--executor shell       → job.Executor = "shell"
                         job.Shell = &ShellConfig{Command: command}

--executor claude-code → job.Executor = "claude-code"
                         job.ClaudeCode = &ClaudeCodeConfig{Prompt: prompt, Model: model}

--executor amplifier   → job.Executor = "amplifier"
                         job.Amplifier = &AmplifierConfig{Prompt: prompt, RecipePath: recipe, Model: model}

--trigger watch        → job.Watch = &WatchConfig{
                             Path: watchPath, Recursive: watchRecursive,
                             Events: split(watchEvents, ","),
                             Mode: watchMode, Debounce: watchDebounce,
                             PollInterval: watchPollInterval,
                         }
```

`JOB_WATCH_PATH` and `JOB_EVENT_PATH` env vars are injected by the scheduler at dispatch time —
no CLI changes needed for that.

---

## Bug Fixes (bundled)

**Bug 1 — deprecated `Command` field:** Current code sets `job.Command = command` (top-level
deprecated field). Fix: set `job.Shell = &ShellConfig{Command: command}` and
`job.Executor = "shell"`.

**Bug 2 — dead `"immediate"` branch:** The `--trigger` flag defaults to `"once"`, making the
`if triggerType == "" { triggerType = "immediate" }` block unreachable. Remove it.

---

## Help Text (agent self-discovery)

### Flag descriptions

```
--executor string        Executor type: shell, claude-code, amplifier (default "shell")
--command string         Shell command to run (required for --executor shell)
--prompt string          AI prompt text (required for --executor claude-code or amplifier)
--recipe string          Path to .yaml recipe file (--executor amplifier only)
--model string           Model override, e.g. "sonnet", "opus" (AI executors only)

--trigger string         Trigger type: cron, loop, once, watch (default "once")
                           cron:  runs on a cron schedule (--schedule required)
                           loop:  repeats on an interval (--schedule required, e.g. 5m)
                           once:  runs once then auto-disables (--schedule = optional delay)
                           watch: fires when files change (--watch-path required)

--watch-path string      File or directory to watch (required for --trigger watch)
--watch-recursive        Watch subdirectories recursively (default false)
--watch-events string    Comma-separated events: create,write,remove,rename,chmod
                           Empty means react to all events
--watch-debounce string  Quiet window before firing after last event, e.g. "500ms"
--watch-mode string      "notify" uses OS file events; "poll" checks on interval (default "notify")
--watch-poll-interval string  Check interval for poll mode, e.g. "2s" (poll mode only)
```

### Example block

```
Examples:
  # Shell command on a cron schedule
  agent-daemon add --name "Nightly cleanup" --trigger cron --schedule "0 0 2 * * *" \
    --command "find /tmp -mtime +7 -delete"

  # Shell command repeating every 5 minutes
  agent-daemon add --name "Health check" --trigger loop --schedule 5m \
    --command "curl -sf http://localhost:8080/health"

  # Shell command when files change in a directory
  agent-daemon add --name "Auto-lint" --trigger watch --watch-path ./src \
    --watch-recursive --command "npm run lint"

  # Shell command watching for specific events only
  agent-daemon add --name "On new file" --trigger watch --watch-path ~/inbox \
    --watch-events create --command "/usr/local/bin/process-new.sh"

  # Claude prompt on a cron schedule
  agent-daemon add --name "Daily standup" --trigger cron --schedule "0 0 9 * * *" \
    --executor claude-code --prompt "Summarize my open GitHub issues and PRs"

  # Claude prompt when files change
  agent-daemon add --name "Review on save" --trigger watch --watch-path ./src \
    --watch-recursive --watch-events write \
    --executor claude-code --prompt "Review the changed file for issues"

  # Amplifier recipe on an interval
  agent-daemon add --name "Hourly digest" --trigger loop --schedule 1h \
    --executor amplifier --recipe ~/recipes/digest.yaml

  # Amplifier recipe triggered by file watch
  agent-daemon add --name "Process inbox" --trigger watch --watch-path ~/inbox \
    --executor amplifier --recipe ~/recipes/process-inbox.yaml

  # Amplifier prompt on a cron schedule
  agent-daemon add --name "Weekly review" --trigger cron --schedule "0 0 9 * * 1" \
    --executor amplifier --prompt "Run my weekly review workflow"

  # Run once immediately (default trigger)
  agent-daemon add --name "Migrate DB" --command "/usr/local/bin/migrate.sh"
```

---

## Scope

- **One file changed:** `internal/cli/jobs.go`
- **No changes to:** `internal/types/`, `internal/scheduler/`, `internal/api/`, tests
- **Backward compatible:** existing `--command` usage unchanged
