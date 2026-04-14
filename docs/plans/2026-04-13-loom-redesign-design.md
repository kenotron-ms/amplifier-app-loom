# Loom Application Redesign

## Goal

Refocus Loom from a terminal-hosting IDE into a lightweight project launcher and background jobs organizer, replacing the xterm.js-based web terminal with project cards that launch native terminals, Amplifier session management surfaced from Amplifier's own session store, and a redesigned Jobs view as the primary value surface.

## Background

Loom was over-solving by trying to host a web terminal (xterm.js) inside a 3-pane IDE layout. Native terminals handle terminal emulation better than any web-based approach can. The real value of Loom lies in two areas: organizing projects and managing background jobs/schedules. This redesign surgically removes the xterm stack and replaces it with a project card grid, native terminal launching, and a cleaner Jobs view.

## Approach

Surgical removal of the xterm stack and replacement of the 3-pane `WorkspaceApp` with a project card grid, native terminal launch integration, and a master-detail Jobs view. Sessions are no longer hosted by Loom — they are read from Amplifier's own session store on disk and surfaced in a read-only list. Workspaces become a simple label grouping on projects with no dedicated CRUD. The sessions bbolt bucket is dropped entirely.

## Component Change Summary

| Component | Action | Notes |
|---|---|---|
| `ui/src/views/projects/terminal/` | **Remove** | Entire folder: `XTermTerminal.tsx`, `TerminalPanel.tsx`, `useTerminalSocket.ts` |
| `ui/src/views/projects/WorkspaceApp.tsx` | **Remove** | 3-pane IDE layout, replaced by `ProjectsGrid` |
| `ui/src/views/projects/SessionStats.tsx` | **Remove** | Tied to PTY sessions |
| `window.__terminalRegistry` global | **Remove** | 256KB client-side output buffer |
| `internal/pty/` | **Remove** | Entire package: PTY manager, ring buffer, WebSocket bridge |
| `internal/amplifier/prepare_session.go` | **Remove** | Pre-created Amplifier sessions for terminal connection |
| PTY/session API routes in `server.go` | **Remove** | 7 routes (see Removals section) |
| Sessions bucket in bbolt | **Remove** | Dropped entirely |
| `Session` struct + session CRUD in `workspaces.go` | **Remove** | `Project` struct kept |
| `ProjectsGrid` | **Add** | Card grid with workspace grouping |
| Project detail view (tabs: Sessions, Settings, Files) | **Add** | Replaces `WorkspaceApp` |
| `GET /api/projects/{id}/amplifier-sessions` | **Add** | Reads Amplifier session store on disk |
| `POST /api/projects/{id}/open-terminal` | **Add** | Launches/focuses native terminal |
| `Workspace` field on `Project` struct | **Add** | Simple label grouping |
| Jobs master-detail split view | **Redesign** | Replaces tab-per-run pattern |
| `ProjectSettingsPanel.tsx` | **Keep** | Slides into new tab layout unchanged |
| `FileViewer.tsx` | **Keep** | Slides into new tab layout unchanged |
| Project CRUD routes | **Keep** | Unchanged |
| Project settings routes | **Keep** | Unchanged |
| File browsing routes | **Keep** | Unchanged |
| Jobs view, Mirror view, Bundles view | **Keep** | Untouched |

## Architecture

The redesigned Loom has three layers:

1. **Frontend (React)** — Project card grid as the home view, with project detail pages containing tabs for sessions, settings, and files. A redesigned Jobs view uses master-detail layout instead of tabs.
2. **Backend (Go)** — Existing project/settings/file CRUD unchanged. Two new endpoints for reading Amplifier sessions from disk and launching native terminals. All PTY and WebSocket infrastructure removed.
3. **External** — Native terminal apps (Terminal.app, iTerm2, Warp, Ghostty) handle terminal emulation. Amplifier's session store (`~/.amplifier/sessions/`) is the source of truth for session state.

Loom no longer hosts terminals. It launches them and reads session state from Amplifier's own store.

## Removals

### Frontend (delete entirely)

- `ui/src/views/projects/terminal/` — entire folder
  - `XTermTerminal.tsx`
  - `TerminalPanel.tsx`
  - `useTerminalSocket.ts`
- `ui/src/views/projects/WorkspaceApp.tsx` — the 3-pane IDE layout
- `ui/src/views/projects/SessionStats.tsx` — PTY session stats widget
- `window.__terminalRegistry` global and the 256KB client-side output buffer

### Backend (delete entirely)

- `internal/pty/` — entire package: PTY process manager, ring buffer, WebSocket bridge
- `internal/amplifier/prepare_session.go` — pre-created Amplifier sessions for terminal connection

### Routes removed from `server.go`

- `POST /api/projects/{id}/sessions/{sid}/terminal` (spawn PTY)
- `WS /api/terminal/{processId}` (WebSocket bridge)
- Terminal resize endpoint
- `GET /api/projects/{id}/sessions` (list sessions)
- `POST /api/projects/{id}/sessions` (create session)
- `DELETE /api/projects/{id}/sessions/{sid}` (delete session)

### Data

- Sessions bucket in bbolt — dropped entirely
- `Session` struct and all session CRUD functions removed from `internal/workspaces/workspaces.go`
- `Project` struct is kept

### What Stays

- Project CRUD routes (`GET/POST/PUT/DELETE /api/projects`)
- Project settings routes (`GET/PUT /api/projects/{id}/settings`)
- File browsing routes (`/api/projects/{id}/...`)
- `ProjectSettingsPanel.tsx` — fully built, 7 sections, auto-save; slides into new tab layout
- `FileViewer.tsx` — slides into new tab layout
- Jobs view, Mirror view, Bundles view — untouched

## Projects Card Grid

### Overview

`WorkspaceApp` is replaced by `ProjectsGrid`. The Projects tab in `App.tsx` renders `ProjectsGrid` instead of `WorkspaceApp`.

### Card Grid Layout

- Cards grouped into visual workspace sections (e.g. "WORK", "PERSONAL")
- 3-column grid at standard viewport width
- Workspace section headers: small uppercase, letter-spaced, muted gray

### Data Model Change

Add a `Workspace string` field to the `Project` struct in bbolt (default: `"Default"`). Workspaces are implicit groups — no workspace CRUD. Assigning a workspace label to a project is all that's needed to create a group.

### Project Card

Each card displays:

- **Project name** — bold, white
- **Shortened path** — monospace, muted gray
- **Status dot** — green if any active sessions, gray if none
- **Session count badge** — e.g. "2 sessions", "0 sessions"
- **"New Session" button** — full-width, ghost/outline style, at the bottom of the card

Clicking the card body navigates to the project detail view (full-page, with a back button to return to the grid).

### Project Detail View

Tab-driven layout with three tabs:

1. **Sessions** — Amplifier session list (see Session Management below)
2. **Settings** — existing `ProjectSettingsPanel`, unchanged
3. **Files** — existing `FileViewer`, unchanged

### Visual Style

| Token | Value |
|---|---|
| Background | `#12141a` |
| Card background | `#1c1f27` |
| Card border | 1px `#252832` |
| Card border-radius | 8px |
| Card shadow | `0 2px 8px rgba(0,0,0,0.4)` |
| Active tab underline | Teal `#14b8a6` |
| Active session dot/badge | Green |
| "Add Project" button | Top-right of nav bar |

No gradients, no left-border accents, no decorative elements.

## Session Management

### Philosophy

Loom no longer hosts terminals. It launches them. Amplifier owns the session state on disk — Loom reads it and surfaces it.

### "New Session" Button

- Opens a new native terminal window at the project path
- Runs `amplifier run` (fresh session, no `--resume`)
- Backend endpoint: `POST /api/projects/{id}/open-terminal` with body `{"mode": "new"}`
- macOS implementation: `open -a <preferred_terminal> <path>`
- Preferred terminal is a global config field with options: Terminal.app, iTerm2, Warp, Ghostty (default: Terminal.app)

### Session List

- Reads from Amplifier's session store on disk (`~/.amplifier/sessions/`)
- Lists recent sessions associated with this project path
- Each session row shows: session name, timestamp, and an **"Open"** button

### Open/Focus Logic

When "Open" is clicked on a session row:

1. Check if a terminal process with that session ID is already running: `ps aux | grep <session-id>`
2. If running: use AppleScript (`osascript`) to find and focus the terminal window containing that process
3. If not running: open a new native terminal with `amplifier run --resume <session-id>`

Backend endpoint: `POST /api/projects/{id}/open-terminal` with body `{"mode": "resume", "sessionId": "<id>"}`

### New Backend Endpoints

**`GET /api/projects/{id}/amplifier-sessions`**
- Reads the Amplifier session store on disk
- Returns recent sessions filtered to the project path
- Sorted by recency

**`POST /api/projects/{id}/open-terminal`**
- Accepts `{mode: "new" | "resume", sessionId?: string}`
- Handles terminal launch via `open -a` and focus detection via `osascript`

## Jobs View Redesign

### Job List Panel

Left column with a vertical list of job cards. Each card has:

- Job name and description
- Enable/Disable toggle
- **Trigger** button for manual runs
- **X icon** — appears on hover only, triggers removal with a confirmation prompt (destructive action)

The X icon is low-weight (hover-only) so it doesn't crowd the card in normal view.

### Run History — Master-Detail Split

When a job is selected, the right side shows a split panel with two columns:

**Left column — Run List:**
- Chronological list of all runs for the selected job
- Each row: timestamp (e.g. "Apr 13 · 3:42 PM"), trigger badge ("Scheduled" in teal, "Manual" in gray), status dot (green = success, red = failed, pulsing = in-progress), duration
- Clicking a row selects it

**Right column — Log Viewer:**
- Output/log for the selected run
- Static for completed runs
- Live SSE stream for in-progress runs
- Scrollable, monospace, dark surface

No tabs anywhere in the Jobs view. Master-detail replaces the tab-per-run pattern entirely.

## Data Flow

### Project Card Grid

1. `App.tsx` renders `ProjectsGrid` in the Projects tab
2. `ProjectsGrid` fetches `GET /api/projects`, groups by `Workspace` field
3. Cards render with session counts from `GET /api/projects/{id}/amplifier-sessions`

### New Session Launch

1. User clicks "New Session" on a card or in the Sessions tab
2. Frontend calls `POST /api/projects/{id}/open-terminal` with `{"mode": "new"}`
3. Backend resolves the project path and preferred terminal from config
4. Backend executes `open -a <terminal> <path>` and returns success
5. A new native terminal window opens at the project directory

### Session Resume

1. User clicks "Open" on a session row in the Sessions tab
2. Frontend calls `POST /api/projects/{id}/open-terminal` with `{"mode": "resume", "sessionId": "<id>"}`
3. Backend checks if a process with that session ID is already running (`ps aux | grep`)
4. If running: backend uses `osascript` to focus the existing terminal window
5. If not running: backend launches `amplifier run --resume <session-id>` in a new native terminal

### Jobs Master-Detail

1. Job list loads from existing job store
2. Selecting a job loads its run history in the left detail column
3. Selecting a run loads its log in the right detail column
4. In-progress runs stream logs via SSE

## Error Handling

- **Terminal launch failure**: If `open -a` fails (terminal app not installed), return a descriptive error to the frontend and suggest changing the preferred terminal in settings.
- **Amplifier session store unavailable**: If `~/.amplifier/sessions/` is missing or unreadable, the Sessions tab shows an empty state with a message explaining that no Amplifier sessions were found.
- **Session focus failure**: If AppleScript cannot find/focus a terminal window, fall back to opening a new terminal with `--resume`.
- **Stale session data**: Sessions listed from Amplifier's store may reference completed or abandoned sessions. The UI shows timestamp and status as-is — no attempt to reconcile or clean up Amplifier's state.

## Testing Strategy

- **Unit tests (Go)**: Test the Amplifier session store reader in isolation with fixture files mimicking `~/.amplifier/sessions/` structure. Test the `open-terminal` endpoint logic with mocked `exec.Command`.
- **Unit tests (React)**: Test `ProjectsGrid` rendering with mocked project data, workspace grouping logic, and card click navigation. Test the Sessions tab component with mocked session data.
- **Integration tests**: Test the full flow from `POST /api/projects/{id}/open-terminal` through to process detection, ensuring the correct shell commands are constructed. Test `GET /api/projects/{id}/amplifier-sessions` against a real (test-fixture) session directory.
- **Manual verification**: Verify native terminal launch across Terminal.app, iTerm2, Warp, and Ghostty. Verify AppleScript focus behavior per terminal app. Verify the Jobs master-detail split with live SSE streaming.
- **Deletion verification**: After removing xterm/PTY code, run the full existing test suite to confirm no regressions from the removal. Verify no orphaned imports or dead routes remain.

## Open Questions

- Amplifier session store exact path and format — need to verify `~/.amplifier/sessions/` structure when implementing.
- AppleScript focusing behavior across different terminal apps — may need per-app implementation.
- Whether project-path filtering of Amplifier sessions is metadata-based or path-scan-based.
