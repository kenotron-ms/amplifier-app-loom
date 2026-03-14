# agent-daemon

A cross-platform scheduled job runner with a web UI and AI assistant. Run shell commands, Claude Code sessions, or Amplifier recipes on a schedule — or whenever a file changes.

## Features

- **Multiple trigger types**
  - `cron` — standard cron with seconds (e.g. `0 */5 * * * *`)
  - `loop` — repeating interval (e.g. `30s`, `5m`, `1h`)
  - `once` — run once then auto-disable, with optional delay (`10m`, `2h`)
  - `watch` — fire when a file or directory changes (OS-level notify or polling)
- **Multiple executor types**
  - `shell` — run any shell command
  - `claude-code` — run `claude -p` with multi-step/resume support
  - `amplifier` — run `amplifier run` or a YAML recipe file
- **Web UI** at `http://localhost:7700` — add, edit, enable/disable, and run jobs
- **AI assistant** — describe jobs in plain English ("every 5 minutes, ask claude code to check for lint errors")
- **System tray app** (macOS/Windows/Linux with CGO) — start/stop/pause/open UI from the menu bar
- **Job queue** — bounded concurrency (4 parallel), deduplication, configurable retries and timeouts
- **System service** — install as a LaunchAgent (macOS), systemd unit (Linux), or Windows Service
- **Persistent storage** — embedded bbolt database, no external dependencies

## Installation

### macOS / Linux — one-liner

```sh
curl -fsSL https://raw.githubusercontent.com/kenotron-ms/agent-daemon/main/install.sh | sh
```

This detects your OS and architecture, downloads the latest binary from GitHub Releases, installs it to `/usr/local/bin`, and tells you if you need to update your `PATH`.

### Windows — PowerShell

```powershell
irm https://raw.githubusercontent.com/kenotron-ms/agent-daemon/main/install.ps1 | iex
```

Installs to `%LOCALAPPDATA%\Programs\agent-daemon` and adds it to your user `PATH` automatically. To use a different directory:

```powershell
$env:INSTALL_DIR="C:\tools"; irm .../install.ps1 | iex
```

### Manual download

Pre-built binaries are on the [GitHub Releases](https://github.com/kenotron-ms/agent-daemon/releases) page:

| Platform | Binary |
|---|---|
| macOS (Apple Silicon) | `agent-daemon-darwin-arm64` |
| macOS (Intel) | `agent-daemon-darwin-amd64` |
| Linux (amd64) | `agent-daemon-linux-amd64` |
| Linux (arm64) | `agent-daemon-linux-arm64` |
| Windows (amd64) | `agent-daemon-windows-amd64.exe` |

Download, `chmod +x` (Unix), and place in any directory on your `PATH`.

### Build from source

```sh
git clone https://github.com/kenotron-ms/agent-daemon.git
cd agent-daemon
make build          # native binary (with tray support if CGO available)
make cross          # all platforms → dist/
```

## Quick start

```sh
# Install as a user-level service (no sudo required)
agent-daemon install

# Start the daemon
agent-daemon start

# Open the web UI
open http://localhost:7700

# Check status
agent-daemon status

# Stop
agent-daemon stop

# Uninstall
agent-daemon uninstall
```

For a system-level service (starts at boot, requires `sudo`):

```sh
sudo agent-daemon install --system
sudo agent-daemon start --system
```

## CLI reference

```
agent-daemon <command> [flags]

Service management:
  install    Install as a system service (--system for boot-level)
  uninstall  Remove the system service
  start      Start the daemon
  stop       Stop the daemon
  status     Show daemon status

Scheduler control:
  pause      Pause job dispatching (running jobs continue)
  resume     Resume job dispatching
  flush      Clear the pending job queue

Job management:
  list       List all jobs
  add        Add a job (--name, --trigger, --schedule, --command, ...)
  remove     Remove a job by ID or name
  prune      Delete all disabled jobs (--dry-run, -y)

Other:
  tray       Launch the system tray app
  serve      Internal: run the HTTP server (called by the service manager)
```

## Configuration

The daemon reads `ANTHROPIC_API_KEY` from the environment for the AI assistant feature. Set it before installing:

```sh
export ANTHROPIC_API_KEY=sk-ant-...
agent-daemon install
```

Default port is `7700`. The database is stored at:
- macOS: `~/Library/Application Support/agent-daemon/agent-daemon.db`
- Linux: `~/.local/share/agent-daemon/agent-daemon.db`
- Windows: `%APPDATA%\agent-daemon\agent-daemon.db`

## Watch trigger

Monitor a file or directory and run a job whenever it changes:

```json
{
  "trigger": { "type": "watch" },
  "watch": {
    "path": "/path/to/project",
    "recursive": true,
    "events": ["create", "write", "remove"],
    "mode": "notify",
    "debounce": "500ms"
  }
}
```

- `mode: "notify"` uses OS-level events (inotify/FSEvents/kqueue) — efficient, recommended
- `mode: "poll"` checks for changes on a timer — works on network drives and containers
- `debounce` waits for a quiet period before firing to avoid rapid re-triggers

## License

MIT
