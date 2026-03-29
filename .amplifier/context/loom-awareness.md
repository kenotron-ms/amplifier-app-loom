# Loom Capabilities

This session has access to **loom** — a local job scheduler running at `http://localhost:7700`.
It runs shell commands, Claude AI prompts, and Amplifier recipes on cron, interval, file-watch, or one-shot triggers.

## Installation

**One-liner (macOS / Linux):**
```bash
curl -fsSL https://raw.githubusercontent.com/kenotron-ms/loom/main/.amplifier/scripts/install.sh | bash
```

This installs the binary, registers it as a background service (auto-starts on login), and on macOS launches the menu bar tray app.

**What the installer does:**
1. Downloads the latest binary for the current OS/arch from GitHub Releases
2. Installs to `/usr/local/bin/loom`
3. Runs `loom install` — registers as a user-level launchd agent (macOS) or systemd user service (Linux)
4. Runs `loom start` — starts the daemon immediately
5. **macOS only:** launches `loom tray` (menu bar icon) and adds it to Login Items

**Manual service commands** (if already installed):
```bash
loom install   # register as background service
loom start     # start the service
loom stop      # stop the service
loom uninstall # remove the service
loom tray      # launch the macOS menu bar app
```

**Check it's running:**
```bash
loom status
# or open http://localhost:7700
```

---

## Using from Amplifier

Load the skill before running any loom command:

```
load_skill(skill_name="loom-cli")
```

## Scripts Path

The CLI script lives inside the loom bundle cache. Resolve it once per session:

```bash
export LOOM_ROOT=$(ls -dt ~/.amplifier/cache/amplifier-bundle-loom-* 2>/dev/null | head -1)
# local dev fallback (when running from the bundle repo itself)
[ -f "scripts/loom-cli.mjs" ] && export LOOM_ROOT="."
```

All commands: `node "$LOOM_ROOT/scripts/loom-cli.mjs" <command> --json`

## Executor Types

| Executor | What it runs |
|----------|-------------|
| `shell` | A shell command (default) |
| `claude-code` | An AI prompt via `claude -p` |
| `amplifier` | An Amplifier prompt or `.yaml` recipe |
