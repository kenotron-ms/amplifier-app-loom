# Loom Workspace Integration Design

**Date:** 2026-03-31  
**Status:** Approved for implementation  
**Branch:** `feature/workspace-integration`

---

## Vision

Loom becomes a unified workspace for managing developer attention across multiple concurrent AI coding sessions and automated jobs. The scheduler is a major pillar inside the product — not the product itself.

The trigger: too many terminal windows, too many simultaneous `amplifier run` sessions, no single place to see what is running, what just finished, or what is waiting for attention.

**Non-goals:**
- Multi-user / auth — loom is a local single-user tool, no login, no JWT, no OAuth
- Linking Jobs to Projects — that relationship is an Amplifier concern, not loom's
- Cloud sync, team sharing, or remote access

---

## Navigation: Hub + Spoke, Three Modes

A persistent top-level tab bar: **Projects | Jobs | Mirror**

Each mode is a full-screen view with its own layout. Switching modes is instant — no navigation hierarchy, no breadcrumbs, no sub-pages. The active tab is underlined in blue.

```
┌─ loom ─────────────────────────────────────────────────────┐
│  Projects   Jobs   Mirror                                   │
├────────────────────────────────────────────────────────────┤
│  <mode content fills this area>                            │
└────────────────────────────────────────────────────────────┘
```

Projects and Jobs are independent domains. No ownership relationship. The linkage between a coding project and a scheduled job is an Amplifier-level concern.

---

## Mode: Projects

Grove's `WorkspaceApp` — copied and lightly adapted. A project is a codebase path on disk. Within a project, each worktree checkout is a session with its own persistent PTY process.

**Layout (inherited from grove):**
- Left sidebar: project picker + session list per project
- Center: full-screen xterm.js PTY terminal (`amplifier run` in the worktree)
- Right panel: file browser tab + session stats tab (token usage, tool calls from `events.jsonl`)

PTY processes are keyed by `projectID::worktreePath` and survive tab switches (`visibility: hidden`, not `display: none`). Switching sessions resumes the exact terminal state.

**Frontend changes to grove's `WorkspaceApp`:**
- Remove auth (`DISABLE_AUTH=true` is already supported in grove — remove the auth guards entirely)
- Change API base URL from `localhost:3001` → `localhost:7700`
- Remove `apps/desktop/`, `backend/`, `packages/` — only `apps/web/src/` is copied

---

## Mode: Jobs

A React port of loom's existing job scheduler UI. Same data (bbolt), new skin.

**Layout:**
- Left panel: job list, sorted by last run. Status dot (green = running, grey = idle). Trigger type shown (cron expression, interval, once, watch).
- Right panel: selected job detail — run history list, active run log streamed via SSE, `▶ Run Now` button.
- Top right: `+ New Job` button.

**Streaming:** SSE via existing `GET /api/runs/:id/stream` and `broadcaster.go`. No changes to the backend — the React frontend replaces the DOM polling of the vanilla JS UI.

---

## Mode: Mirror

A React port of loom's existing mirror/connector UI.

**Layout:**
- Left panel: connector list with health status (live / idle / error).
- Right panel: selected connector — entity list, last sync time, diff viewer for recent changes.

No new backend routes needed for the initial implementation — the existing `/api/mirror/*` routes are consumed as-is.

---

## Frontend Integration

### Approach
Copy grove's `apps/web/src/` into `loom/ui/src/`. Loom owns the frontend from this point forward — it diverges from upstream grove intentionally. Grove is the starting point, not an ongoing dependency.

### Repo structure
```
loom/
├── ui/                      ← React SPA (from grove's apps/web)
│   ├── src/
│   │   ├── App.tsx           ← hub nav: Projects | Jobs | Mirror tabs
│   │   ├── views/
│   │   │   ├── projects/     ← grove's WorkspaceApp (auth removed, URL patched)
│   │   │   ├── jobs/         ← new: job list + run detail + SSE log viewer
│   │   │   └── mirror/       ← new: connector list + entity browser
│   │   └── api/client.ts     ← base URL → localhost:7700
│   ├── package.json
│   └── vite.config.ts
├── web/embed.go              ← updated: embeds ui/dist/ (replaces web/)
└── Makefile                  ← `make ui` → npm run build in ui/; `make build` embeds
```

The old `web/` directory (vanilla JS) is deleted. `web/embed.go` is updated to embed `ui/dist/` instead.

### Build
```makefile
ui:
    cd ui && npm install && npm run build

build: ui
    go build ./...
```

CI runs `make build`. The embedded SPA is baked into the Go binary at compile time.

---

## Backend: New Go Packages

### `internal/workspaces/`
Projects and sessions CRUD. Backed by bbolt — extends the existing `internal/store/` with two new bucket groups.

**bbolt buckets:**
```
projects/                   projectID (UUID) → Project{}
sessions/                   sessionID (UUID) → Session{}
sessions_by_project/        projectID/sessionID → sessionID   (index)
```

**Types:**
```go
type Project struct {
    ID             string
    Name           string
    Path           string  // absolute path on disk
    CreatedAt      int64
    LastActivityAt int64
}

type Session struct {
    ID           string
    ProjectID    string
    Name         string  // e.g. branch name
    WorktreePath string  // absolute path to git worktree
    ProcessID    *string // nil when no PTY is running; active PTY key when set
    CreatedAt    int64
    Status       string  // "idle" | "active" | "stopped"
}
```

No new database file. `loom.db` (bbolt) is the single store for all of loom's data.

### `internal/pty/`
PTY process manager. Processes are keyed by `projectID::worktreePath` and persist across terminal tab switches.

- **Spawn:** `creack/pty` to fork a shell in the given working directory
- **WebSocket bridge:** `gorilla/websocket` — reads PTY output → writes to WS, reads WS input → writes to PTY stdin
- **Cleanup:** process map is cleaned up on project/session delete

`gorilla/websocket` is the one new Go dependency this package introduces.

### `internal/files/`
Read-only file browser scoped to a project's path on disk.

- Directory listings: returns name, size, isDir, modTime
- File contents: returns raw bytes (frontend handles syntax highlighting)
- Scoped: all paths are validated to be within the project's root path (no path traversal)

---

## API Surface

### New routes (Projects domain)
```
GET    /api/projects                              list all projects
POST   /api/projects                              create {name, path}
GET    /api/projects/:id                          get project
PATCH  /api/projects/:id                          update {name}
DELETE /api/projects/:id                          delete + kill PTYs + cleanup sessions

GET    /api/projects/:id/sessions                 list sessions
POST   /api/projects/:id/sessions                 create {name, worktreePath} → git worktree add
DELETE /api/projects/:id/sessions/:sid            delete → git worktree remove + kill PTY

POST   /api/projects/:id/sessions/:sid/terminal   spawn PTY → {processId}
WS     /api/terminal/:processId                   PTY I/O bridge (gorilla/websocket)

GET    /api/projects/:id/sessions/:sid/files      directory listing {path?}
GET    /api/projects/:id/sessions/:sid/files/*    file contents
GET    /api/projects/:id/sessions/:sid/stats      token + tool breakdown from events.jsonl
```

### Streaming
| Channel | Protocol | Direction | Handler |
|---|---|---|---|
| PTY terminal I/O | WebSocket | Bidirectional | `internal/pty/` + `gorilla/websocket` |
| Job run logs | SSE | Server → client | `broadcaster.go` (existing, unchanged) |
| Session stats | HTTP | One-shot | Read `~/.amplifier/.../events.jsonl` |

### Existing routes (unchanged)
```
/api/jobs/*       scheduler (existing)
/api/runs/*       run history + SSE stream (existing)
/api/mirror/*     connectors + entities (existing)
/api/settings     API key + theme (existing)
/api/status       health (existing)
GET *             → serve ui/dist/index.html (SPA catch-all)
```

---

## New Go Dependencies

| Package | Purpose | Notes |
|---|---|---|
| `github.com/creack/pty` | PTY fork + I/O | Standard Go PTY library |
| `github.com/gorilla/websocket` | WebSocket bridge for xterm.js | Already widely used in Go ecosystem |

No new database dependency. `modernc.org/sqlite` is explicitly excluded — bbolt is sufficient.

---

## What Is Not Changing

- `internal/scheduler/` — untouched
- `internal/mirror/` — untouched  
- `internal/store/` — extended (new buckets), not refactored
- `internal/api/` existing route handlers — untouched
- `loom.db` format — additive only (new buckets)
- The `loom` CLI surface (`loom start`, `loom stop`, `loom status`, etc.) — untouched

---

## Feature Branch Scope

Branch name: `feature/workspace-integration`

**In scope:**
1. Copy grove `apps/web/src/` → `loom/ui/src/`
2. New `App.tsx` hub navigation (Projects | Jobs | Mirror)
3. Projects view: auth removed, URL patched to 7700
4. Jobs view: React port of existing vanilla JS job list + run log
5. Mirror view: React port of existing vanilla JS connector + entity browser
6. `internal/workspaces/` — Projects + Sessions CRUD (bbolt)
7. `internal/pty/` — PTY process manager + WebSocket bridge
8. `internal/files/` — read-only file browser
9. New `/api/projects/*` and `WS /api/terminal/*` routes wired into `internal/api/`
10. `web/embed.go` updated to embed `ui/dist/`
11. `Makefile` updated with `make ui` target
12. Delete old `web/` vanilla JS files

**Out of scope for this branch:**
- Memory/engram system (grove has it, loom will not)
- Outcomes API (grove's Claude-backed outcome articulation)
- Auth of any kind
- Session links, attachments
- Git operations API (beyond `git worktree add/remove` on session create/delete)
