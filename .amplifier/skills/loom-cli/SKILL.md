---
name: loom-cli
description: Full command reference for loom — manage jobs, connectors, and mirror watches via the local scheduler at localhost:7700. Use when creating jobs, setting up service watchers, querying mirrored data, or triggering jobs on external service changes.
---

# loom — Full Reference

Two interfaces to the same daemon at `localhost:7700`:
- **`loom` binary** — native Go CLI for all commands including connectors and mirror
- **`loom-cli.mjs`** — Node.js wrapper for job/run management (legacy, AI-friendly)

---

## Setup (once per session)

```bash
# Node.js CLI alias (jobs/runs/daemon control)
export LOOM_ROOT=$(ls -dt ~/.amplifier/cache/amplifier-bundle-loom-* 2>/dev/null | head -1)
[ -f "scripts/loom-cli.mjs" ] && export LOOM_ROOT="."
alias loom="node \"$LOOM_ROOT/scripts/loom-cli.mjs\""

# Go binary (connector/mirror commands + everything else)
# Already on PATH if installed: loom connector ..., loom mirror ...
```

---

## Daemon Status & Control

```bash
ad status --json          # state, uptime, job count, active runs, queue depth
ad pause  --json          # pause scheduling (running jobs continue)
ad resume --json
ad flush  --json          # drain queue without killing jobs
```

**`status` schema:**
```json
{
  "state": "running | paused",
  "pid": 12345,
  "startedAt": "ISO8601",
  "activeRuns": 2,
  "queueDepth": 0,
  "jobCount": 5,
  "version": "0.4.1"
}
```

---

## Job Management

### Create a job — `add`

```bash
# Shell — cron
ad add --name "Daily cleanup" --trigger cron --schedule "0 2 * * *" \
  --command "find /tmp -mtime +7 -delete" --json

# Shell — loop every 5 minutes
ad add --name "Health check" --trigger loop --schedule 5m \
  --command "curl -sf http://localhost:8080/health" --json

# Shell — once (runs immediately, then auto-disables)
ad add --name "DB migration" --trigger once \
  --command "/usr/local/bin/migrate.sh" --json

# claude-code — runs a prompt via `claude -p`
ad add --name "Nightly review" --trigger cron --schedule "0 22 * * *" \
  --executor claude-code \
  --prompt "Review the git diff from the last 24 hours and summarise to /tmp/review.md" \
  --model claude-sonnet-4-6 --cwd /Users/ken/workspace/myproject --json

# amplifier — runs a recipe
ad add --name "Weekly report" --trigger cron --schedule "0 9 * * 1" \
  --executor amplifier --recipe /path/to/recipe.yaml --json

# watch — fires when filesystem changes
ad add --name "Inbox processor" --trigger watch \
  --watch-path "$HOME/Inbox" --watch-recursive --watch-events create \
  --watch-debounce 2s --executor amplifier \
  --prompt "A new file arrived. Process it per Life OS protocols." --json

# connector trigger — fires when a connector detects a change (see Connectors section)
ad add --name "Price alert" --trigger connector \
  --connector-id <connector-id> \
  --executor shell --command 'notify "Price changed: $MIRROR_DIFF_JSON"' --json
```

### Trigger types

| Type | `--schedule` | Behaviour |
|------|-------------|-----------|
| `cron` | `"0 9 * * *"` | Runs on cron schedule |
| `loop` | `"5m"`, `"30s"` | Repeats at fixed interval |
| `once` | optional delay `"10m"` | Runs once then auto-disables |
| `watch` | — (use `--watch-path`) | Fires on filesystem events |
| `connector` | — (use `--connector-id`) | Fires when connector detects change |

### Executor types

| Executor | Key flags | What runs |
|----------|-----------|-----------|
| `shell` | `--command CMD` | Shell command |
| `claude-code` | `--prompt TEXT` `--model MODEL` | `claude -p "..."` |
| `amplifier` | `--prompt TEXT` or `--recipe PATH` `--bundle NAME` | `amplifier run` |

**Executor inference** (when `--executor` omitted): `--command` → shell, `--recipe` → amplifier, `--prompt` → claude-code.

### Other job commands

```bash
ad list --json                               # list all jobs
ad get --id <ID-or-prefix> --json            # get one job
ad update --id abc123 --schedule "0 10 * * *" --json
ad delete --id abc123 --yes --json
ad trigger --id abc123 --json                # run immediately
ad enable  --id abc123 --json
ad disable --id abc123 --json
ad prune   --json                            # delete all disabled jobs
```

---

## Connectors — Watch External Services

Connectors poll external services and maintain a local **mirror** (shadow copy) of the data you care about. AI jobs fire only when something actually changes — not on every poll.

```
connector polls → extracts data → diffs against mirror → if changed → fires job
                  (fetch method)   (JQ / prompt-based)   (deterministic)  (LLM)
```

### Concepts

**Fetch methods:**
- `command` — runs a shell command (e.g. `gh api`, `node teams-cli.mjs`), parses stdout as JSON
- `http` — HTTP GET with optional headers
- `browser` — headless Chrome via `agent-browser` with persistent authenticated profile; agentic extraction per the connector's `--prompt`

**Entity addressing:** `{kind}/{identity}` — e.g. `github.pr/owner/repo/42`, `amazon.product/B09V3KXJPB`, `teams.channel/engineering/general`. Kind is a free-form namespace you define. Multiple connectors can write to the same entity address to build a fuller picture.

**Prompt:** Natural language description of what to extract. Used as the browser-operator instruction for `browser` connectors. Self-healing — adapts to page changes.

### Set up a connector — `admit` (interactive, recommended)

Interactive setup with AI-assisted validation. For browser connectors, opens a headed browser for one-time authentication.

```bash
# Amazon price watch (browser — no public API)
loom connector admit \
  --name "amazon-airpods" \
  --url "https://amazon.com/dp/B09V3KXJPB" \
  --site amazon \
  --prompt "Extract current price, availability (in stock/out of stock), and any active deal. Return JSON." \
  --method browser \
  --interval 15m

# GitHub PR monitoring (CLI — uses gh)
loom connector admit \
  --name "watch-vscode-pr-42" \
  --command "gh api /repos/microsoft/vscode/pulls/42" \
  --entity "github.pr/microsoft/vscode/42" \
  --prompt "Track: state (open/closed/merged), review_count, ci_status, last_comment_body." \
  --method command \
  --interval 60s

# Teams channel messages (CLI — uses m365)
loom connector admit \
  --name "watch-teams-general" \
  --command "node $CONNECTOR_M365_ROOT/scripts/teams-agent-cli.mjs messages --channel general --limit 5 --json" \
  --entity "teams.channel/engineering/general" \
  --prompt "Watch for new messages. Track: last message ID, author, content, timestamp." \
  --method command \
  --interval 30s
```

`admit` flow:
1. Runs one test fetch and shows extracted data
2. For `browser` method — opens headed Chrome if no profile exists for `--site`; user authenticates once
3. Prompts: "Is this what you wanted to monitor? [Y/n]"
4. On confirm — creates connector and starts polling

### Create a connector programmatically — `connector add`

```bash
# Command-based connector
loom connector add \
  --name "github-pr-42" \
  --method command \
  --command "gh api /repos/owner/repo/pulls/42" \
  --entity "github.pr/owner/repo/42" \
  --prompt "Track state, ci_status, last_comment." \
  --interval 60s

# HTTP connector
loom connector add \
  --name "crypto-btc-price" \
  --method http \
  --url "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd" \
  --entity "crypto.price/bitcoin" \
  --prompt "Track the current USD price." \
  --interval 5m

# Browser connector (requires profile — use admit for first-time auth)
loom connector add \
  --name "amazon-airpods" \
  --method browser \
  --url "https://amazon.com/dp/B09V3KXJPB" \
  --site amazon \
  --entity "amazon.product/B09V3KXJPB" \
  --prompt "Extract price, availability, active deal." \
  --interval 15m
```

### Manage connectors

```bash
loom connector list                  # table: id, name, method, interval, health, last-change
loom connector remove <id|name>      # delete connector (mirror data preserved)
loom connector remove <id> --purge   # delete connector AND its mirror data
```

### Connector health states

| State | Meaning | Sync behaviour |
|-------|---------|----------------|
| `healthy` | All recent fetches succeeded | Normal interval |
| `degraded` | 1–2 consecutive failures | Normal interval, flagged |
| `unhealthy` | 3+ consecutive failures | Skips 2 of every 3 sync cycles |

A successful fetch resets health to `healthy` immediately.

---

## Mirror — Query the Shadow Copy

The mirror stores the current state of all watched entities and a full change log.

```bash
# List all watched entities
loom mirror entities

# Filter by kind
loom mirror entities github.pr
loom mirror entities amazon.product

# Get current snapshot of an entity
loom mirror get github.pr/owner/repo/42
loom mirror get amazon.product/B09V3KXJPB --json

# Get a specific field
loom mirror get teams.channel/engineering/general --field last_message

# View change history
loom mirror changes                                   # all recent changes
loom mirror changes --entity amazon.product/B09V3KXJPB
loom mirror changes --entity github.pr/owner/repo/42 --limit 20
loom mirror changes --since 1h                        # last hour, all entities

# List connectors with health
loom mirror connectors
```

**Entity snapshot** — raw JSON as the connector fetched and extracted it:
```json
{
  "price": "$89.99",
  "availability": "In Stock",
  "deal": "Save 18% with coupon"
}
```

**Change record** — one entry per detected change:
```json
{
  "entity":    "amazon.product/B09V3KXJPB",
  "timestamp": "2026-03-27T14:00:00Z",
  "version":   17,
  "ops": [
    { "path": "price", "op": "set", "from": "$109.99", "to": "$89.99" }
  ]
}
```

---

## Mirror REST API

```
GET  /api/mirror/connectors              list all connectors
POST /api/mirror/connectors              create connector (body: Connector JSON)
PUT  /api/mirror/connectors/{id}         update connector
DELETE /api/mirror/connectors/{id}       delete connector

GET  /api/mirror/entities                list all entities (?kind=github.pr to filter)
GET  /api/mirror/entities/{address}      current snapshot (address is URL-encoded)
GET  /api/mirror/changes                 change log (?entity=...&limit=N&since=1h)
POST /api/mirror/changes/prune           prune old changes per retention policy
```

**Connector JSON body for POST/PUT:**
```json
{
  "name":          "amazon-airpods",
  "prompt":        "Extract price, availability, active deal.",
  "fetchMethod":   "browser",
  "url":           "https://amazon.com/dp/B09V3KXJPB",
  "site":          "amazon",
  "entityAddress": "amazon.product/B09V3KXJPB",
  "interval":      "15m",
  "enabled":       true
}
```

For `command` method: set `"fetchMethod": "command"`, `"command": "gh api ..."` instead of `url`/`site`.
For `http` method: set `"fetchMethod": "http"`, `"url": "..."`, optional `"headers": {}`.

---

## Firing Jobs on Connector Changes

Use `--trigger connector` on any job to fire it when a connector detects a change.

```bash
# Simple notification on Amazon price change
ad add --name "airpods-price-alert" \
  --trigger connector \
  --connector-id <id-from-connector-list> \
  --executor shell \
  --command 'echo "Price changed: $(echo $MIRROR_DIFF_JSON | jq -r .ops[0].to)" | mail -s "Deal alert" me@example.com' \
  --json

# AI-powered response to Teams message
ad add --name "teams-message-handler" \
  --trigger connector \
  --connector-id <teams-connector-id> \
  --executor claude-code \
  --prompt "A new Teams message was detected. Context: $MIRROR_DIFF_JSON. Read the full entity at $MIRROR_SNAPSHOT_FILE and decide if/how to respond." \
  --json

# React to GitHub PR comment — run review
ad add --name "pr-review-on-comment" \
  --trigger connector \
  --connector-id <github-pr-connector-id> \
  --executor amplifier \
  --recipe /path/to/code-review.yaml \
  --json
```

### Mirror env vars available inside triggered jobs

| Variable | Content |
|----------|---------|
| `$MIRROR_ENTITY` | Entity address e.g. `github.pr/owner/repo/42` |
| `$MIRROR_CONNECTOR_ID` | UUID of the connector that fired |
| `$MIRROR_DIFF_JSON` | JSON array of `ChangeOp` — what changed |
| `$MIRROR_SNAPSHOT_FILE` | Path to temp file with full current entity JSON |
| `$MIRROR_PREV_JSON` | Full previous entity snapshot as JSON string |
| `$MIRROR_CURR_JSON` | Full current entity snapshot as JSON string |

**`MIRROR_DIFF_JSON` shape:**
```json
[
  { "path": "price",        "op": "set",    "from": "$109.99", "to": "$89.99" },
  { "path": "deal",         "op": "set",    "from": null,      "to": "Save 18%" },
  { "path": "comments.#",   "op": "append", "to": { "id": 5, "body": "LGTM" } }
]
```

Read the full snapshot for richer context:
```bash
cat $MIRROR_SNAPSHOT_FILE | jq .
```

---

## Runs

```bash
ad runs --json                               # recent runs (default 50)
ad runs --limit 10 --json
ad run --id <run-id> --json                  # single run with full output
ad job-runs --id abc123 --limit 5 --json     # runs for a specific job
ad clear-runs --json                         # clear all run history
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
  "output": "combined stdout+stderr (capped 64KB)",
  "attempt": 1
}
```

---

## Service Lifecycle

```bash
loom install    # install as launchd / systemd / SCM service
loom uninstall
loom start
loom stop
```

---

## Error Handling

All `ad` commands exit `1` on failure. With `--json`:
```json
{ "error": "descriptive message" }
```

Common errors:
- `"Daemon not reachable at http://localhost:7700"` → daemon not running; run `loom start`
- `"No job found matching 'abc'"` → bad prefix; use `ad list` to find the ID
- `"Ambiguous prefix 'ab' matches 3 jobs"` → use more characters

---

## Common Patterns

**Watch Amazon and notify on price drop:**
```bash
# 1. Admit the connector (one-time, handles browser auth)
loom connector admit \
  --name "airpods-pro" --url "https://amazon.com/dp/B09V3KXJPB" \
  --site amazon --prompt "Extract price and availability." \
  --method browser --interval 15m

# 2. Get the connector ID
CONN_ID=$(loom connector list --json | jq -r '.[] | select(.name=="airpods-pro") | .id')

# 3. Add a job that fires on price change
ad add --name "airpods-price-alert" --trigger connector \
  --connector-id $CONN_ID --executor shell \
  --command 'osascript -e "display notification \"$(echo $MIRROR_CURR_JSON | jq -r .price)\" with title \"AirPods price changed\""'
```

**React to GitHub PR comments with AI:**
```bash
loom connector admit \
  --name "vscode-pr-42" \
  --command "gh api /repos/microsoft/vscode/pulls/42/comments --jq 'last'" \
  --entity "github.pr/microsoft/vscode/42" \
  --prompt "Track the most recent comment: id, body, author." \
  --method command --interval 60s

CONN_ID=$(loom connector list --json | jq -r '.[] | select(.name=="vscode-pr-42") | .id')

ad add --name "pr-comment-responder" --trigger connector \
  --connector-id $CONN_ID --executor claude-code \
  --prompt "A new PR comment arrived. Read context from \$MIRROR_SNAPSHOT_FILE. Draft a helpful response to the comment."
```

**Monitor Teams channel and act on keywords:**
```bash
loom connector admit \
  --name "teams-devops" \
  --command "node \$CONNECTOR_M365_ROOT/scripts/teams-agent-cli.mjs messages --channel devops --limit 3 --json" \
  --entity "teams.channel/devops" \
  --prompt "Track last message: id, author, content. Flag if content mentions 'incident' or 'down'." \
  --method command --interval 30s

CONN_ID=$(loom connector list --json | jq -r '.[] | select(.name=="teams-devops") | .id')

ad add --name "incident-detector" --trigger connector \
  --connector-id $CONN_ID --executor amplifier \
  --prompt "A Teams message arrived in the devops channel. Check \$MIRROR_DIFF_JSON — if it mentions an incident or outage, run the incident response playbook."
```

**Query mirror data inside a running job:**
```bash
# Get current snapshot of any entity (no daemon call needed — reads env vars)
echo $MIRROR_CURR_JSON | jq .price

# Or read the full snapshot file for larger payloads
ENTITY_DATA=$(cat $MIRROR_SNAPSHOT_FILE)

# Query across all watched GitHub PRs
loom mirror entities github.pr --json
loom mirror get github.pr/owner/repo/42 --json
```

**Inspect what changed recently:**
```bash
loom mirror changes --since 1h --json
loom mirror changes --entity amazon.product/B09V3KXJPB --limit 5 --json
```

**Check connector health:**
```bash
loom mirror connectors
# Shows: name | entity | health | fail-count | last-sync | last-change
```
