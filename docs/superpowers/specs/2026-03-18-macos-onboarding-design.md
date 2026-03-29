# macOS First-Run Onboarding Design

**Date:** 2026-03-18
**Status:** Draft

---

## Problem Statement

When a user downloads `AgentDaemon.app` and opens it for the first time, nothing guides
them to a working installation. Three silent failure modes exist:

1. **No API key** — the daemon starts but cannot run any AI-powered jobs; no feedback is shown
2. **No Full Disk Access** — the background service silently fails to access job directories,
   dotfiles, or shell tools; jobs error out cryptically
3. **UserContext not captured** — if the user installs the service via the tray (not the CLI),
   `HomeDir` and `Shell` are never persisted; all jobs break under launchd

Additionally, the `BuildServiceConfig` function uses `os.Executable()` verbatim. When a user
runs `loom install` via the `/usr/local/bin` symlink, the LaunchAgent plist records
the symlink path rather than the resolved `.app` binary path. This gives the daemon a
different TCC identity than the tray, so any TCC grants to the bundle may not apply.

---

## Goals

- Guide the user from "just opened the .app" to "daemon is installed and ready" in one flow
- Collect the Anthropic API key (and optionally OpenAI) during setup
- Obtain Full Disk Access for the daemon binary
- Capture `UserContext` (HomeDir, Shell, UID) as part of the install step
- Provide a persistent health indicator in the tray for post-install issues
- Fix the symlink/TCC identity gap in `BuildServiceConfig`

## Non-Goals

- Linux and Windows (existing CLI install flow is unchanged)
- Adding App Sandbox (the app intentionally stays unsandboxed)
- Changing the CLI `loom install` path (still works as before)
- Full Disk Access for the tray `.app` itself (only the background service needs it)

---

## Research Findings

Key findings that shaped this design (gathered from Apple DTS, open source apps, and HIG):

- **No public TCC change event exists.** Apple's Quinn (DTS) confirmed this directly.
  The recommended pattern is `NSApplicationDidBecomeActive` as the primary trigger
  (since the user *must* leave the app to reach System Settings), plus a 1-second poll
  as backup during the active permission-waiting state.
- **Full Disk Access has no system prompt.** Unlike Camera or Contacts, FDA cannot be
  requested via a system dialog. The app must deep-link to System Settings and wait.
- **Production apps poll.** Ice (15k stars) uses a 1-second `Timer` during the onboarding
  phase. MacPaw's PermissionsKit probes `~/Library/Safari/Bookmarks.plist` to infer FDA status.
- **Pre-permission screens increase grant rates ~81%** (NN/g). Explain *why* before the
  System Settings redirect, not after.
- **No full re-wizard for updates.** No well-regarded app resurfaces a full wizard when a
  new permission is needed. The pattern is a tray health indicator with a targeted "Fix →" link.

---

## Design

### Trigger Conditions

The onboarding wizard is shown when the `.app` launches **and any of the following are true:**

| Condition | Check |
|---|---|
| No API key in DB | `store.GetConfig().AnthropicKey == ""` |
| Full Disk Access not granted | `os.Open(filepath.Join(home, "Library/Safari/Bookmarks.plist"))` returns `EPERM`/`EACCES` (tilde is NOT expanded by `os.Open` — must use resolved home path) |
| `UserContext.HomeDir` is empty | `config.UserContext == nil \|\| config.UserContext.HomeDir == ""` (pointer guard required — `UserContext` is `*UserContext`, nil on fresh install) |

If all three conditions pass, the wizard is skipped and the tray loads normally.

**First-run vs. returning user:** `OnboardingComplete` (see Onboarding State Persistence)
controls which surface handles a failed condition:
- `OnboardingComplete == false` → show the full 3-step wizard
- `OnboardingComplete == true` → show the tray health indicator + targeted "Fix →" dialog only

The wizard is **macOS-only** and only shown when running from a `.app` bundle
(i.e., `__CFBundleIdentifier` env var is set).

---

### Step 1 — Welcome

A simple entry screen that sets context before asking for anything.

**Contents:**
- App icon (centered, large)
- Heading: "Welcome to Loom"
- Body: "Your AI-powered job scheduler. Let's get you set up in three quick steps."
- Step progress indicator: `[1 Welcome] [2 API Keys] [3 Permissions]` — step 1 active
- Single CTA: "Get Started →" (blue, bottom right)
- No "Back" button (first step)

**Advance condition:** user clicks "Get Started".

---

### Step 2 — API Keys

Collects the credentials the daemon needs to run AI-powered jobs.

**Contents:**
- Heading: "Connect your AI"
- Subtitle: "Loom needs an API key to run AI-powered jobs. Add at least one."
- **Anthropic API Key** field (required)
  - Placeholder: `sk-ant-...`
  - Inline validation: green checkmark when non-empty; red border + "Required" if Next is
    clicked while empty
  - Pre-filled priority: BoltDB `AnthropicKey` first (already stored), then `$ANTHROPIC_API_KEY`
    env var. This ensures returning users (e.g., wizard re-triggered by FDA revocation) are
    never hard-blocked by an empty field for a key they already provided.
- **OpenAI API Key** field (optional, labeled "Optional")
  - Placeholder: `sk-...`
  - Pre-filled priority: BoltDB `OpenAIKey` first, then `$OPENAI_API_KEY` env var
- Trust note (small, gray): "Keys are stored locally in the app database and only sent to
  the respective AI providers."
- Step progress indicator: step 2 active, step 1 shows ✓
- "← Back" (bottom left), "Continue →" (bottom right, disabled until Anthropic key filled)

**On Continue:**
- Save both keys to BoltDB (same `absorbEnvKeys` storage path, bucket `config`)
- Advance to Step 3

---

### Step 3 — Full Disk Access

Guides the user to grant FDA to the daemon binary, with detection that automatically
advances the UI when the grant is detected.

**Contents:**
- Heading: "Grant Full Disk Access"
- Subtitle: "The background service needs Full Disk Access to run jobs, source shell configs
  like `.zshrc`, and find tools like `.nvm` and `.amplifier`."
- Permission row:
  - Amber warning shield icon
  - Bold: "Full Disk Access" / gray subtext: "Required · Background service"
  - Button: "Open System Settings →" (bordered, not filled)
    - On click: `open "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"`
    - Button changes to "Waiting…" and disables after first click
- **Embedded guide graphic** (see below) — shown inline below the permission row
- "Waiting for permission…" spinner (shown after "Open System Settings" is clicked)
- Step progress indicator: step 3 active, steps 1 and 2 show ✓
- "← Back" (bottom left), "Done" (bottom right, **disabled** until FDA granted)

**Step 3 renders in two initial states depending on whether FDA is already granted:**

| FDA state on arrival | Initial UI |
|---|---|
| Not granted | Amber warning row, "Open System Settings →" button active, "Done" disabled, polling not yet started |
| Already granted | Green "Full Disk Access granted ✓" row, "Done" button **enabled immediately**, no polling needed |

If FDA is already granted when Step 3 loads (e.g., the wizard triggered only because the API
key was missing), the user sees the completed state and can click "Done" right away.
`captureUserContext()` and service install still run on "Done" regardless — they are not
gated on the FDA prompt interaction, only on reaching the Done action.

**On FDA detection (during the waiting state):**
- "Waiting for permission…" → green "Full Disk Access granted ✓"
- Permission row icon changes from amber warning to green checkmark
- "Done" button enables
- Polling goroutine stops

**On Done — error handling:** if `installService` fails (e.g., plist write error), show an
inline error message in the wizard ("Service install failed — try again or run
`loom install` from the terminal") and keep the Done button enabled for retry.
Do not close the wizard on install failure.

**On Done (from either initial state) — success path:**
- `captureUserContext()` called (HomeDir, Shell, UID saved to DB)
- Service installed as user-level LaunchAgent **only if not already installed**
  (`isServiceInstalled()` check before calling `installService(LevelUser)`);
  `kardianos/service.Control(svc, "install")` is not idempotent — calling it when
  a plist already exists returns an error
- Service started (idempotent — safe to call even if already running)
- `OnboardingComplete = true` saved to DB
- Wizard closes
- Tray menu reflects installed state (hides "Set up background service" prompt)

#### FDA Detection Mechanism

Two concurrent signals, whichever fires first:

1. **`NSApplicationDidBecomeActive`** (primary) — fires when user returns to the app from
   System Settings. Trigger an immediate probe check.
2. **1-second polling goroutine** (backup) — runs only while Step 3 is active and "Open
   System Settings" has been clicked. Stops as soon as FDA is detected or wizard is dismissed.

**Probe:** attempt `os.Open(filepath.Join(home, "Library/Safari/Bookmarks.plist"))`.
Success (no `EPERM`/`EACCES`) = FDA granted.

MacPaw's PermissionsKit uses this exact probe. If the user does not have Safari installed,
fall back to `os.Open("/Library/Preferences/com.apple.TimeMachine.plist")`.

---

### Embedded Guide Graphic

A static annotated image embedded inside the wizard at Step 3, showing exactly what the
user will see in System Settings and what to click.

**Contents of the graphic:**
- Realistic macOS System Settings window (Privacy & Security → Full Disk Access pane)
- "AgentDaemon" entry visible in the Full Disk Access list (toggle OFF)
- Two callout annotations:
  1. Red circle "1" with arrow → "Find Loom in this list"
  2. Red circle "2" with arrow → toggle switch → "Toggle this ON"

**Delivery:** PNG asset committed to `internal/onboarding/fda-guide.png` (source:
`/tmp/step3-fda-syspreferences.png`). Because `WKWebView` with `loadHTMLString` cannot
resolve relative URLs, the PNG is **not** referenced as `<img src="...">`. Instead,
the Go build reads the embedded bytes and base64-encodes them into the HTML at startup,
producing an inline `<img src="data:image/png;base64,...">`. The image is therefore
self-contained in the HTML string passed to `loadHTMLString`. The file must be committed
before wizard implementation begins — it is a prerequisite for the HTML build step.

---

### Wizard Window

The wizard is a **native macOS panel** (`NSPanel`, floating, non-resizable) sized 480×460pt,
centered on screen. It stays above the tray app.

**Implementation approach: `NSPanel` + `WKWebView` (decided).** A minimal `NSPanel` contains
a `WKWebView` loading a bundled HTML/CSS page (`internal/onboarding/wizard.html`). This is
the chosen approach because:
- The project already has an embedded web UI (HTML/CSS/JS patterns are established)
- The guide graphic is a simple `<img>` tag — no native image-rendering CGo needed
- Go → JS state pushes (FDA granted, validation feedback) use `evaluateJavaScript`
- JS → Go user actions (button clicks, field input) use `WKScriptMessageHandler` message posts
  (`WKScriptMessageHandler` is a JS→Go mechanism only — not bidirectional)
- CGo is already required (systray), so no new build dependency
- Avoids the complexity of wiring NSTextField, NSButton, NSImageView individually in CGo

**CGo surface for this approach (complete list):**
- `NSPanel` creation and show/close
- `WKWebView` creation and `loadHTMLString`
- `WKScriptMessageHandler` registration for JS→Go messages
- `evaluateJavaScript` for Go→JS state pushes
- `NSApplicationDelegate applicationDidBecomeActive` registration

**`NSApplicationDelegate` conflict:** `fyne.io/systray` already installs its own
`NSApplicationDelegate`. Registering a second delegate would silently replace it.
Resolution: use `NSNotificationCenter` to observe
`NSApplicationDidBecomeActiveNotification` directly — this does not require owning the
delegate and coexists safely with systray's delegate.

**JS delivery:** All wizard JavaScript is inline in `wizard.html`. There is no separate
`wizard.js` file. Keeping it inline avoids `WKWebView` same-origin restrictions on
loading local file URLs.

Build tags gate the entire `internal/onboarding` package:
- Cocoa implementation: `//go:build darwin && cgo` (not `cgo` alone — Linux also uses CGo for systray and would fail to compile Cocoa imports)
- Stub: `//go:build !darwin || !cgo`

---

## Tray Health Indicator

Post-onboarding, the tray monitors health continuously and surfaces issues without
re-running the full wizard.

### Health Check

Runs every 30 seconds in a background goroutine. Checks:

| Check | Pass condition |
|---|---|
| Anthropic API key present | `config.AnthropicKey != ""` |
| Full Disk Access | probe succeeds |
| Service installed | LaunchAgent plist exists |
| Service running | HTTP GET `localhost:7700/api/status` returns HTTP 200 |

### Tray Badge

- **All checks pass:** no badge, icon normal
- **Any check fails:** amber `!` badge on tray icon

### Menu Changes (degraded state)

An amber "Action Required" section appears at the top of the menu (above the status section):

```
⚠ Action Required
  Full Disk Access missing  Fix →
─────────────────────────────────
● Checking…
```

"Fix →" opens a targeted mini-dialog (single-step, not the full wizard):
- Missing API key → small input dialog to enter the key
- Missing FDA → jump straight to the Step 3 screen (just the permission row + guide graphic)
- Service not installed → call `config.CaptureUserContext()` then `installService(LevelUser)`
  then start; `CaptureUserContext` must always accompany an install to avoid the original
  "jobs break under launchd" bug

The 30-second health-check goroutine starts **after the wizard closes** (or immediately
on tray launch if `OnboardingComplete == true`). It does not run concurrently with an
in-progress wizard to avoid surfacing amber-badge noise mid-flow.

---

## BuildServiceConfig Fix

**File:** `internal/service/service.go`

Add `filepath.EvalSymlinks` before the path is written to the plist:

```go
func BuildServiceConfig(level InstallLevel) *service.Config {
    exePath, _ := os.Executable()
    // Resolve symlinks so the LaunchAgent plist always points to the real
    // binary inside the .app bundle, not /usr/local/bin/loom.
    // This ensures the daemon shares the .app's TCC identity (com.ms.loom).
    if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
        exePath = resolved
    }
    cfg := &service.Config{
        Executable: exePath,
        ...
    }
```

**Why it matters:** if the user runs `loom install` via the `/usr/local/bin` symlink
before ever opening the `.app`, the LaunchAgent would record the symlink path as the
executable. That path has no bundle identity, so TCC grants to `com.ms.loom` would
not apply to the daemon process.

---

## captureUserContext in Tray Install Path

**File:** `internal/tray/tray.go`

Currently `captureUserContext()` is only called inside the CLI `installCmd`. The tray's
`runServiceInstallDialog()` calls `service.Control(svc, "install")` directly — skipping
context capture.

Fix: call `captureUserContext()` (from `internal/cli/service_cmds.go`) after successful
install in `runServiceInstallDialog`, and also at the end of the onboarding wizard's
Step 3 "Done" flow.

`captureUserContext` must be exported or moved to a shared internal package so both
`internal/cli` and `internal/tray` can call it.

---

## File Structure

| File | Change | Purpose |
|---|---|---|
| `internal/onboarding/fda-guide.png` | **Commit first (prerequisite)** | Annotated System Settings guide graphic — source at `/tmp/step3-fda-syspreferences.png`. Must be committed before wizard implementation begins; all other onboarding tasks depend on it. |
| `internal/onboarding/wizard.go` | Create | Untagged (all platforms). Defines `Step` type, `State` struct, completion-check logic, and the exported `Show()` function. `Show()` calls the unexported `showImpl()` which is defined by a platform file — this is the standard Go platform-dispatch pattern. |
| `internal/onboarding/wizard_darwin.go` | Create (`//go:build darwin && cgo`) | Defines `showImpl()` — real implementation: NSPanel + WKWebView creation, NSNotificationCenter activation observer, 1-second FDA polling goroutine, FDA probe. Also holds `//go:embed wizard.html` and `//go:embed fda-guide.png` (as `var wizardHTML []byte` and `var fdaGuideBytes []byte`). `fda-guide.png` bytes are base64-encoded at startup and substituted into the HTML string before `loadHTMLString` is called. |
| `internal/onboarding/wizard_other.go` | Create (`//go:build !darwin \|\| !cgo`) | Defines `showImpl()` as a no-op so the package compiles on Linux, Windows, and non-CGo builds. |
| `internal/onboarding/wizard.html` | Create | Full wizard UI: 3 steps, all CSS in `<style>` block, all JS in `<script>` block, `fda-guide.png` referenced as `{{FDA_GUIDE_DATA_URI}}` placeholder substituted at runtime. Loaded via `//go:embed` in `wizard_darwin.go` — NOT via runtime `os.ReadFile` (working directory is not `internal/onboarding/` in a deployed `.app`). |
| `internal/config/config.go` | Modify | (1) Add `OnboardingComplete bool` to `Config` struct; (2) move `captureUserContext` here as exported `CaptureUserContext()` — currently unexported in `internal/cli/service_cmds.go` |
| `internal/cli/service_cmds.go` | Modify | Remove `captureUserContext`; call `config.CaptureUserContext()` instead |
| `internal/service/service.go` | Modify | Add `filepath.EvalSymlinks` to `BuildServiceConfig` |
| `internal/tray/tray.go` | Modify | Call `config.CaptureUserContext()` after install; add 30s health-check goroutine; amber badge when unhealthy; "Fix →" menu item; call `onboarding.Show()` on first launch if conditions not met. LaunchAgent plist path for health check: `~/Library/LaunchAgents/loom.plist` (matches `isServiceInstalled()` existing check). |

---

## Onboarding State Persistence

Add `OnboardingComplete bool` to the existing `Config` struct in BoltDB. Set to `true`
when the user clicks "Done" in Step 3.

The wizard trigger logic re-evaluates all three conditions on every launch regardless of
this flag — so if FDA is later revoked or an API key is deleted, the health indicator
catches it without re-running the wizard.

The flag is used only to decide whether to show the full 3-step wizard vs. the targeted
single-step "Fix" dialog.

---

## Sequence Diagram

```
.app launch
    │
    ├─ __CFBundleIdentifier set? No → CLI mode, skip onboarding
    │
    ├─ Yes → check conditions
    │         API key missing? OR FDA not granted? OR UserContext empty?
    │
    ├─ Any true → Show wizard
    │   Step 1: Welcome
    │       ↓ "Get Started"
    │   Step 2: API Keys
    │       ↓ Enter key(s) + "Continue" → save to DB
    │   Step 3: Full Disk Access
    │       ↓ "Open System Settings" → deep-link + start polling
    │       ↓ NSApplicationDidBecomeActive OR 1s poll detects grant
    │       ↓ "Done" button enables
    │       ↓ "Done" click → CaptureUserContext() + installService(LevelUser) + start
    │       ↓ OnboardingComplete = true → wizard closes
    │
    └─ All pass (or after wizard Done) → tray loads normally
           │
           └─ Background health check every 30s
                  Any issue → amber badge + "Fix →" in menu
```

---

## Open Questions

1. **System-level install via onboarding** — The current Step 3 always installs as
   `LevelUser` (LaunchAgent). Should the wizard offer system-level (LaunchDaemon) as an
   option? Recommendation: no — keep it simple; users who need system-level can use the
   tray menu or CLI after onboarding.

2. **FDA for the tray `.app` itself vs. the daemon binary** — FDA granted in System Settings
   will appear under the daemon binary path. Since the tray and daemon are the same binary
   (both at `.app/Contents/MacOS/loom`), the grant covers both. Confirmed via the
   `BuildServiceConfig` + `EvalSymlinks` fix.
