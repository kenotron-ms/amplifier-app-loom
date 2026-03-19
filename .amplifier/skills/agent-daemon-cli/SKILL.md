---
name: agent-daemon-cli
description: Full command reference for agent-daemon-cli.mjs — manage jobs, runs, and daemon control via the local scheduler at localhost:7700
---

# agent-daemon-cli — Full Reference

Headless CLI for AI agents. No npm dependencies — requires Node.js 18+.

## Setup (once per session)

```bash
export AGENT_DAEMON_ROOT=$(ls -dt ~/.amplifier/cache/amplifier-bundle-agent-daemon-* 2>/dev/null | head -1)
[ -f "scripts/agent-daemon-cli.mjs" ] && export AGENT_DAEMON_ROOT="."
alias ad="node \"$AGENT_DAEMON_ROOT/scripts/agent-daemon-cli.mjs\""
```

All examples below use `ad` as the alias.

---

## Daemon Status & Control

```bash
# Show status — state, uptime, job count, active runs, queue depth
ad status --json

# Pause / resume job scheduling (running jobs are not affected)
ad pause  --json
ad resume --json

# Flush all pending jobs from the queue (does not delete jobs)
ad flush --json
```

**`status` JSON schema:**
```json
{
  "state": "running | paused",
  "pid": 12345,
  "startedAt": "2026-03-19T00:00:00Z",
  "activeRuns": 2,
  "queueDepth": 0,
  "jobCount": 5,
  "version": "0.1.0"
}
```

---

## Job Management

### List all jobs

```bash
ad list --json
```

Returns `Job[]`. Each job has `id`, `name`, `trigger`, `executor`, `enabled`, `createdAt`.

### Get a single job

```bash
ad get --id <ID-or-prefix> --json
```

ID prefix resolution: `abc123` resolves to the full UUID if unambiguous.

### Create a job — `add`

Required: `--name`. Everything else has defaults.

```bash
# Shell job — cron trigger
ad add \
  --name "Daily cleanup" \
  --trigger cron \
  --schedule "0 2 * * *" \
  --command "find /tmp -mtime +7 -delete" \
  --json

# Shell job — loop (every 5 minutes)
ad add \
  --name "Health check" \
  --trigger loop \
  --schedule 5m \
  --command "curl -sf http://localhost:8080/health" \
  --json

# Shell job — once (runs immediately, then auto-disables)
ad add \
  --name "DB migration" \
  --trigger once \
  --command "/usr/local/bin/migrate.sh" \
  --json

# Shell job — once with a delay
ad add \
  --name "Delayed cleanup" \
  --trigger once \
  --schedule 10m \
  --command "/usr/local/bin/cleanup.sh" \
  --json

# claude-code job — runs a prompt via `claude -p`
ad add \
  --name "Nightly code review" \
  --trigger cron \
  --schedule "0 22 * * *" \
  --executor claude-code \
  --prompt "Review the git diff from the last 24 hours in this repo and write a summary to /tmp/review.md" \
  --model claude-sonnet-4-5 \
  --cwd /Users/ken/workspace/myproject \
  --json

# amplifier job — runs a recipe
ad add \
  --name "Weekly report" \
  --trigger cron \
  --schedule "0 9 * * 1" \
  --executor amplifier \
  --recipe /Users/ken/workspace/recipes/weekly-report.yaml \
  --json

# amplifier job — runs a free-form prompt
ad add \
  --name "Inbox triage" \
  --trigger loop \
  --schedule 30m \
  --executor amplifier \
  --prompt "Check my M365 inbox and summarize any urgent emails to ~/.lifeos/memory/Work/Notes/inbox-summary.md" \
  --bundle connector-m365 \
  --json

# watch trigger — fires when a file/directory changes
ad add \
  --name "Process new scans" \
  --trigger watch \
  --watch-path /Users/ken/Inbox \
  --watch-recursive \
  --watch-events create,write \
  --watch-debounce 500ms \
  --executor amplifier \
  --prompt "A new file arrived in ~/Inbox. Route and process it following Life OS protocols." \
  --json
```

**`add` response schema (created Job):**
```json
{
  "id": "uuid",
  "name": "string",
  "description": "string",
  "trigger": { "type": "cron|loop|once|watch", "schedule": "string" },
  "executor": "shell|claude-code|amplifier",
  "enabled": true,
  "cwd": "string",
  "timeout": "string",
  "maxRetries": 0,
  "createdAt": "ISO8601",
  "updatedAt": "ISO8601",
  "shell":      { "command": "string" },
  "claudeCode": { "prompt": "string", "model": "string", "maxTurns": 0 },
  "amplifier":  { "prompt": "string", "recipePath": "string", "bundle": "string", "model": "string" },
  "watch":      { "path": "string", "recursive": false, "events": [], "debounce": "string" }
}
```
Only the executor-specific block matching `executor` will be set.

### Update a job

Same flags as `add`, plus `--id`. Only provided fields are updated.

```bash
ad update --id abc123 --schedule "0 10 * * *" --json
ad update --id abc123 --command "npm run build && npm test" --json
ad update --id abc123 --timeout 10m --retries 2 --json
```

### Delete a job

```bash
ad delete --id abc123 --yes --json
```

Without `--yes`: returns `{ "deleted": false, "reason": "confirmation required..." }`.

### Trigger / enable / disable / prune

```bash
# Run a job immediately (regardless of schedule)
ad trigger --id abc123 --json
# → { "status": "triggered" }

# Enable a disabled job
ad enable  --id abc123 --json

# Disable a job (keeps it, stops it running)
ad disable --id abc123 --json

# Delete all disabled jobs at once
ad prune --json
# → { "deleted": N }
```

---

## Trigger Types

| Type | `--schedule` | Behaviour |
|------|-------------|-----------|
| `cron` | Standard cron expression `"0 9 * * *"` | Runs on schedule |
| `loop` | Go duration `"5m"`, `"1h"`, `"30s"` | Repeats at fixed interval |
| `once` | Optional duration delay `"10m"` | Runs once then auto-disables |
| `watch` | — (use `--watch-path`) | Fires on filesystem events |

---

## Executor Types

| Executor | CLI flags | What runs |
|----------|-----------|-----------|
| `shell` | `--command CMD` | Shell command directly |
| `claude-code` | `--prompt TEXT` `--model MODEL` | `claude -p "..."` |
| `amplifier` | `--prompt TEXT` OR `--recipe PATH` `--bundle NAME` `--model MODEL` | `amplifier run "..."` or recipe |

**Executor inference** (when `--executor` is omitted):
- `--command` present → `shell`
- `--recipe` present → `amplifier`
- `--prompt` present → `claude-code`
- nothing → `shell`

---

## Runs

```bash
# List recent runs (default 50)
ad runs --json
ad runs --limit 10 --json

# Get a single run with full output
ad run --id <run-id> --json

# List runs for a specific job
ad job-runs --id abc123 --json
ad job-runs --id abc123 --limit 5 --json

# Clear all run history
ad clear-runs --json
```

**`JobRun` schema:**
```json
{
  "id": "uuid",
  "jobId": "uuid",
  "jobName": "string",
  "startedAt": "ISO8601",
  "endedAt": "ISO8601 | null",
  "status": "pending|running|success|failed|timeout|skipped",
  "exitCode": 0,
  "output": "combined stdout+stderr",
  "attempt": 1
}
```

---

## Service Lifecycle

These shell out to the `agent-daemon` binary (daemon does not need to be running):

```bash
ad install    # Install as launchd (macOS) / systemd (Linux) / SCM (Windows) service
ad uninstall  # Uninstall service
ad start      # Start the service
ad stop       # Stop the service
```

---

## Error Handling

All commands exit with code `1` on failure. With `--json`:
```json
{ "error": "descriptive error message" }
```

Common errors:
- `"Daemon not reachable at http://localhost:7700"` → daemon is not running; use `ad start`
- `"No job found matching 'abc'"` → bad ID prefix; use `ad list` to find the ID
- `"Ambiguous prefix 'ab' matches 3 jobs"` → use more characters

---

## Common Workflows

**Check what's running then trigger a job:**
```bash
ad status --json
ad list --json
ad trigger --id <prefix> --json
ad job-runs --id <prefix> --limit 3 --json
```

**Schedule an Amplifier recipe to run daily:**
```bash
ad add \
  --name "Daily standup prep" \
  --trigger cron \
  --schedule "0 8 * * 1-5" \
  --executor amplifier \
  --recipe /Users/ken/workspace/recipes/standup.yaml \
  --json
```

**Watch a folder and process new files with Life OS:**
```bash
ad add \
  --name "Inbox processor" \
  --trigger watch \
  --watch-path "$HOME/Inbox" \
  --watch-recursive \
  --watch-events create \
  --watch-debounce 2s \
  --executor amplifier \
  --prompt "A new file arrived in ~/Inbox. Read SYSTEM.md and process it according to Life OS protocols." \
  --json
```

**Inspect a failed run:**
```bash
ad runs --limit 20 --json | node -e "
  const d=JSON.parse(require('fs').readFileSync('/dev/stdin','utf8'));
  d.filter(r=>r.status==='failed').forEach(r=>console.log(r.id,r.jobName,r.endedAt));
"
ad run --id <run-id> --json
```
