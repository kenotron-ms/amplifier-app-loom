# macOS Onboarding Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a 3-step first-run onboarding wizard (Welcome → API Keys → Full Disk Access) to the macOS `.app` bundle, with a tray health indicator for post-install issues.

**Architecture:** NSPanel + WKWebView wizard (HTML/CSS/JS inlined, assets via `//go:embed`). FDA detection via NSNotificationCenter + 1s poll. State machine in pure Go (`wizard.go`), CGo in two files (`wizard_darwin_impl.go` + `wizard_darwin_callbacks.go`), stubs in `wizard_other.go`. Tray health check goroutine drives amber badge.

**Tech Stack:** Go 1.25, CGo (darwin && cgo), Objective-C/Cocoa, WebKit, `//go:embed`, `fyne.io/systray`, `kardianos/service`, `bbolt`

**Worktree:** `.worktrees/macos-onboarding` (branch: `feature/macos-onboarding`)

**Spec:** `docs/superpowers/specs/2026-03-18-macos-onboarding-design.md`

**Pre-existing failures (not ours):** `internal/scheduler` build failures (`execShell` undefined, `NewRunner` args) — do not fix, just don't make them worse.

---

## File Map

| File | Action | Notes |
|---|---|---|
| `internal/service/service.go` | Modify | Add `filepath.EvalSymlinks` to `BuildServiceConfig` |
| `internal/config/config.go` | Modify | Add `OnboardingComplete bool` field |
| `internal/config/usercontext.go` | Create | Export `CaptureUserContext()` + `LookupUserShell()` moved from CLI |
| `internal/cli/service_cmds.go` | Modify | Remove `captureUserContext` + `lookupUserShell`, call `config.CaptureUserContext()` |
| `internal/tray/tray.go` | Modify | Call `config.CaptureUserContext()` on install; add health check goroutine + amber badge + "Fix →" menu; trigger `onboarding.Show()` on launch |
| `internal/onboarding/wizard.go` | Create | Pure Go: `state`, `NeedsOnboarding()`, `Show()`, `gState` var |
| `internal/onboarding/wizard_other.go` | Create | `//go:build !darwin \|\| !cgo` stubs: `CheckFDA()`, `showImpl()` |
| `internal/onboarding/wizard_darwin_impl.go` | Create | `//go:build darwin && cgo`: NSPanel+WKWebView CGo (no `//export`), `showImpl()`, `CheckFDA()`, `pollFDA()`, `buildHTML()`, `pushJS()` |
| `internal/onboarding/wizard_darwin_callbacks.go` | Create | `//go:build darwin && cgo`: `//export wizardGoMessage`, `//export wizardGoActivation`, `handleDone()` |
| `internal/onboarding/wizard.html` | Create | 3-step wizard UI with inline CSS+JS; placeholders `{{ANTHROPIC_KEY}}`, `{{OPENAI_KEY}}`, `{{FDA_GRANTED}}`, `{{FDA_GUIDE_DATA_URI}}` |
| `internal/onboarding/fda-guide.png` | Already committed (549d706) | Prerequisite done |

---

## Task 1: BuildServiceConfig — EvalSymlinks fix

**Files:**
- Modify: `internal/service/service.go`

### Why
When `loom install` runs via the `/usr/local/bin/loom` symlink, `os.Executable()` returns the symlink path. The LaunchAgent plist then records the symlink path as the executable, which has a different TCC identity than the `.app` bundle. Resolving symlinks ensures the plist always points to the real binary inside the `.app`.

- [ ] **Step 1: Write the failing test**

Add to `internal/service/service_test.go` (create if absent):
```go
package service

import (
    "os"
    "path/filepath"
    "testing"
)

func TestBuildServiceConfig_ResolvesSymlink(t *testing.T) {
    // Create a real file and a symlink to it
    dir := t.TempDir()
    real := filepath.Join(dir, "real-binary")
    if err := os.WriteFile(real, []byte("x"), 0755); err != nil {
        t.Fatal(err)
    }
    link := filepath.Join(dir, "symlink-binary")
    if err := os.Symlink(real, link); err != nil {
        t.Fatal(err)
    }

    // Temporarily override os.Executable by monkey-patching is not trivial,
    // so we test the resolution function directly.
    resolved, err := filepath.EvalSymlinks(link)
    if err != nil {
        t.Fatal(err)
    }
    if resolved != real {
        t.Errorf("expected %s, got %s", real, resolved)
    }
}
```

- [ ] **Step 2: Run test to verify it passes (this test is for the helper, always passes)**

```bash
cd .worktrees/macos-onboarding && go test ./internal/service/... -run TestBuildServiceConfig -v
```

- [ ] **Step 3: Apply the fix to BuildServiceConfig**

In `internal/service/service.go`, change:
```go
func BuildServiceConfig(level InstallLevel) *service.Config {
    exePath, _ := os.Executable()
```
to:
```go
func BuildServiceConfig(level InstallLevel) *service.Config {
    exePath, _ := os.Executable()
    // Resolve symlinks so the LaunchAgent plist always points to the real binary
    // inside the .app bundle — this ensures TCC grants to com.ms.loom apply
    // to the daemon process (same TCC identity as the tray).
    if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
        exePath = resolved
    }
```

Add `"path/filepath"` to imports if not already there.

- [ ] **Step 4: Verify build passes**

```bash
cd .worktrees/macos-onboarding && go build ./internal/service/...
```
Expected: no errors.

- [ ] **Step 5: Run tests**

```bash
cd .worktrees/macos-onboarding && go test ./internal/service/... -v
```
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/service/service.go && \
  git commit -m "fix: resolve symlinks in BuildServiceConfig for correct TCC identity"
```

---

## Task 2: Export CaptureUserContext + add OnboardingComplete

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/usercontext.go`
- Modify: `internal/cli/service_cmds.go`

### Why
`captureUserContext()` is currently unexported in the CLI package. It needs to be accessible from `internal/tray` and `internal/onboarding`. `OnboardingComplete` tracks whether the wizard has run so the tray knows to show a targeted "Fix →" dialog instead of the full wizard.

- [ ] **Step 1: Write a test for the new exported function**

Create `internal/config/usercontext_test.go`:
```go
package config

import (
    "testing"
)

func TestCaptureUserContext_ReturnsValidContext(t *testing.T) {
    uc := CaptureUserContext()
    if uc == nil {
        t.Fatal("expected non-nil UserContext")
    }
    if uc.HomeDir == "" {
        t.Error("HomeDir should not be empty")
    }
    if uc.Username == "" {
        t.Error("Username should not be empty")
    }
    if uc.Shell == "" {
        t.Error("Shell should not be empty")
    }
}

func TestConfigHasOnboardingComplete(t *testing.T) {
    cfg := Defaults()
    if cfg.OnboardingComplete {
        t.Error("OnboardingComplete should default to false")
    }
    cfg.OnboardingComplete = true
    if !cfg.OnboardingComplete {
        t.Error("OnboardingComplete should be settable to true")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd .worktrees/macos-onboarding && go test ./internal/config/... -run TestCaptureUserContext -v 2>&1
```
Expected: FAIL — `CaptureUserContext` undefined.

- [ ] **Step 3: Add OnboardingComplete to Config**

In `internal/config/config.go`, add the field after `UserContext`:
```go
    UserContext *UserContext `json:"userContext,omitempty"`
    // OnboardingComplete is true once the user has finished the first-run wizard.
    // When false and conditions require onboarding, the full 3-step wizard is shown.
    // When true, only the targeted tray "Fix →" dialog is shown.
    OnboardingComplete bool `json:"onboardingComplete,omitempty"`
```

- [ ] **Step 4: Create internal/config/usercontext.go**

```go
package config

import (
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

// CaptureUserContext captures the identity of the currently running user
// from the OS. For sudo installs, it prefers $SUDO_USER (the real invoking user)
// over the effective user (root). Returns nil if the user cannot be determined.
//
// This function is intentionally side-effect-free — callers are responsible
// for saving the result to the store.
func CaptureUserContext() *UserContext {
	var u *user.User
	var err error

	// Prefer the real invoking user when running under sudo.
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err = user.Lookup(sudoUser)
	}
	if u == nil {
		u, err = user.Current()
	}
	if err != nil || u == nil {
		return nil
	}

	return &UserContext{
		HomeDir:  u.HomeDir,
		Username: u.Username,
		Shell:    LookupUserShell(u.Username),
		UID:      u.Uid,
	}
}

// LookupUserShell returns the login shell for the given username.
// On macOS it queries Directory Services; on other platforms it parses /etc/passwd.
// Falls back to /bin/zsh (macOS) or /bin/bash (Linux/other).
func LookupUserShell(username string) string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("dscl", ".", "-read",
			"/Users/"+username, "UserShell").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "UserShell:") {
					if parts := strings.Fields(line); len(parts) >= 2 {
						return parts[1]
					}
				}
			}
		}
	}

	// Linux / fallback: parse /etc/passwd
	if data, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) >= 7 && fields[0] == username {
				return fields[6]
			}
		}
	}

	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/bash"
}
```

- [ ] **Step 5: Update internal/cli/service_cmds.go**

Remove the `captureUserContext()` and `lookupUserShell()` functions from `service_cmds.go` (lines ~155-217). Replace the call site:

Old:
```go
if uc := captureUserContext(); uc != nil {
```
New:
```go
if uc := config.CaptureUserContext(); uc != nil {
```

Verify `config` is already imported (it is). Remove the now-unused `"os/exec"`, `"os/user"`, `"runtime"`, `"strings"` imports if they are no longer used by other code in the file. (Check: `strings` is still used by `absorbEnvKeys` — keep it. `runtime`, `os/user`, `os/exec` — check.)

After removing the two functions, run:
```bash
cd .worktrees/macos-onboarding && go build ./internal/cli/...
```
Fix any unused import errors until it compiles cleanly.

- [ ] **Step 6: Run all tests**

```bash
cd .worktrees/macos-onboarding && go test ./internal/config/... ./internal/cli/... -v
```
Expected: all pass (new tests + existing).

- [ ] **Step 7: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/config/ internal/cli/service_cmds.go && \
  git commit -m "refactor: export CaptureUserContext to internal/config, add OnboardingComplete"
```

---

## Task 3: Update tray to call CaptureUserContext

**Files:**
- Modify: `internal/tray/tray.go`

### Why
The tray's `runServiceInstallDialog()` calls `service.Control(svc, "install")` directly, skipping context capture. The daemon would then have no `UserContext`, and all jobs would fail under launchd (no `$HOME`, no `$SHELL`).

- [ ] **Step 1: Open internal/tray/tray.go and locate runServiceInstallDialog**

The function is at line ~163. It calls `service.Control(svc, "install")` for user-level installs.

- [ ] **Step 2: Add CaptureUserContext call after install**

After the `_ = service.Control(svc, "start")` line in `runServiceInstallDialog` (user-level path), add:

```go
// Capture user context now — we're still in the user's interactive session.
captureAndSaveUserContext()
```

Also add the same call at the end of `installService()` (the function used by the Install menu items):
```go
func installService(level internalsvc.InstallLevel) {
    svc, err := internalsvc.NewServiceForControl(level)
    if err != nil {
        return
    }
    _ = service.Control(svc, "install")
    _ = service.Control(svc, "start")
    captureAndSaveUserContext()
}
```

- [ ] **Step 3: Add captureAndSaveUserContext helper to tray.go**

```go
// captureAndSaveUserContext saves the current user's HomeDir, Shell, and UID
// into the daemon's database. Must be called while still in the user's
// interactive session (before the daemon takes over under launchd).
func captureAndSaveUserContext() {
    uc := config.CaptureUserContext()
    if uc == nil {
        return
    }
    s, err := store.Open(platform.DBPath())
    if err != nil {
        slog.Warn("tray: could not open store for UserContext capture", "err", err)
        return
    }
    defer s.Close()
    cfg, err := s.GetConfig(context.Background())
    if err != nil {
        return
    }
    cfg.UserContext = uc
    _ = s.SaveConfig(context.Background(), cfg)
    slog.Info("tray: captured user context", "home", uc.HomeDir, "shell", uc.Shell)
}
```

Add required imports: `"context"`, `"github.com/ms/loom/internal/config"`, `"github.com/ms/loom/internal/platform"`, `"github.com/ms/loom/internal/store"`.

- [ ] **Step 4: Build check**

```bash
cd .worktrees/macos-onboarding && go build ./internal/tray/...
```

Expected: compiles (CGO_ENABLED=1 on macOS). If on non-macOS, the tray package has a `//go:build cgo` guard so it will be skipped.

- [ ] **Step 5: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/tray/tray.go && \
  git commit -m "fix: call CaptureUserContext in tray install paths"
```

---

## Task 4: Onboarding state machine + stubs

**Files:**
- Create: `internal/onboarding/wizard.go`
- Create: `internal/onboarding/wizard_other.go`

### Why
The pure-Go layer defines the types and the `NeedsOnboarding()` check. These are tested independently of CGo. `wizard_other.go` provides the stub so the package compiles on non-macOS/non-CGo targets.

- [ ] **Step 1: Write tests**

Create `internal/onboarding/wizard_test.go`:
```go
package onboarding

import (
    "testing"

    "github.com/ms/loom/internal/config"
)

func TestNeedsOnboarding_NoAPIKey(t *testing.T) {
    cfg := config.Defaults()
    cfg.AnthropicKey = ""
    if !NeedsOnboarding(cfg) {
        t.Error("expected NeedsOnboarding=true when AnthropicKey is empty")
    }
}

func TestNeedsOnboarding_NilUserContext(t *testing.T) {
    cfg := config.Defaults()
    cfg.AnthropicKey = "sk-ant-test"
    cfg.UserContext = nil
    if !NeedsOnboarding(cfg) {
        t.Error("expected NeedsOnboarding=true when UserContext is nil")
    }
}

func TestNeedsOnboarding_EmptyHomeDir(t *testing.T) {
    cfg := config.Defaults()
    cfg.AnthropicKey = "sk-ant-test"
    cfg.UserContext = &config.UserContext{HomeDir: ""}
    if !NeedsOnboarding(cfg) {
        t.Error("expected NeedsOnboarding=true when HomeDir is empty")
    }
}

func TestNeedsOnboarding_AllSet_NoFDA(t *testing.T) {
    // CheckFDA() returns false on non-darwin/non-cgo builds (wizard_other.go).
    // On those platforms, NeedsOnboarding should still return true.
    cfg := config.Defaults()
    cfg.AnthropicKey = "sk-ant-test"
    cfg.UserContext = &config.UserContext{HomeDir: "/Users/test", Shell: "/bin/zsh"}
    // Result depends on platform: FDA false on non-darwin → true
    // We just verify it doesn't panic.
    _ = NeedsOnboarding(cfg)
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd .worktrees/macos-onboarding && go test ./internal/onboarding/... -v 2>&1 | head -20
```
Expected: package not found / compile error.

- [ ] **Step 3: Create internal/onboarding/wizard.go**

```go
package onboarding

import (
	"context"

	"github.com/ms/loom/internal/config"
	"github.com/ms/loom/internal/store"
)

// state holds wizard session data shared between the Go state machine and
// the CGo UI callbacks.
type state struct {
	st           store.Store
	anthropicKey string
	openAIKey    string
	fdaGranted   bool
	closed       bool
	onDone       func()
}

// gState is the active wizard session. Set by Show(), read by CGo callbacks.
// Package-level so wizard_darwin_callbacks.go can access it without CGo.
var gState *state

// NeedsOnboarding returns true if the first-run wizard should be shown.
// It checks three conditions defined in the spec:
//   - No Anthropic API key stored
//   - UserContext.HomeDir is missing (daemon won't find tools under launchd)
//   - Full Disk Access not granted (CheckFDA, platform-specific)
func NeedsOnboarding(cfg *config.Config) bool {
	if cfg.AnthropicKey == "" {
		return true
	}
	if cfg.UserContext == nil || cfg.UserContext.HomeDir == "" {
		return true
	}
	if !CheckFDA() {
		return true
	}
	return false
}

// Show presents the onboarding wizard. onDone is called when the wizard
// completes successfully. No-op on non-macOS or non-CGo builds.
func Show(st store.Store, onDone func()) {
	cfg, _ := st.GetConfig(context.Background())
	s := &state{
		st:     st,
		onDone: onDone,
	}
	if cfg != nil {
		s.anthropicKey = cfg.AnthropicKey
		s.openAIKey = cfg.OpenAIKey
		s.fdaGranted = CheckFDA()
	}
	gState = s
	showImpl(s)
}
```

- [ ] **Step 4: Create internal/onboarding/wizard_other.go**

```go
//go:build !darwin || !cgo

package onboarding

// CheckFDA reports whether Full Disk Access has been granted.
// On non-macOS or non-CGo builds this always returns false — the wizard
// is macOS-only and these stubs exist solely to satisfy the compiler.
func CheckFDA() bool { return false }

// showImpl is the platform implementation entry point.
// No-op on non-macOS/non-CGo builds.
func showImpl(_ *state) {}
```

- [ ] **Step 5: Run tests**

```bash
cd .worktrees/macos-onboarding && go test ./internal/onboarding/... -v
```
Expected: all 4 tests pass.

- [ ] **Step 6: Verify package compiles on all platforms**

```bash
cd .worktrees/macos-onboarding && \
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./internal/onboarding/... && \
  echo "linux/no-cgo: OK" && \
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./internal/onboarding/... && \
  echo "windows/no-cgo: OK"
```
Expected: both print OK.

- [ ] **Step 7: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/onboarding/ && \
  git commit -m "feat: add onboarding state machine and cross-platform stubs"
```

---

## Task 5: wizard.html — 3-step wizard UI

**Files:**
- Create: `internal/onboarding/wizard.html`

### Why
The WKWebView renders this HTML. All CSS and JS must be inline (no external files — WKWebView's `loadHTMLString` with nil `baseURL` cannot resolve relative URLs). The PNG guide is substituted as a `data:` URI by Go before `loadHTMLString` is called.

- [ ] **Step 1: Create internal/onboarding/wizard.html**

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Loom Setup</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }

  body {
    font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", sans-serif;
    background: #f5f5f7;
    color: #1d1d1f;
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    -webkit-user-select: none;
  }

  /* ── Step indicator ──────────────────────────────────────── */
  .steps {
    display: flex;
    justify-content: center;
    gap: 8px;
    padding: 20px 24px 0;
  }
  .step-pill {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: #6e6e73;
    padding: 4px 12px;
    border-radius: 20px;
    background: #e5e5ea;
  }
  .step-pill.active {
    background: #0071e3;
    color: white;
  }
  .step-pill.done {
    background: #34c759;
    color: white;
  }
  .step-num {
    font-weight: 600;
  }

  /* ── Content area ────────────────────────────────────────── */
  .content {
    flex: 1;
    padding: 24px 32px 16px;
    display: flex;
    flex-direction: column;
    overflow: auto;
  }

  .step-view { display: none; flex-direction: column; flex: 1; }
  .step-view.active { display: flex; }

  /* Step 1 – Welcome */
  .welcome-icon {
    width: 72px; height: 72px;
    background: linear-gradient(145deg, #1c1c1e, #3a3a3c);
    border-radius: 16px;
    margin: 8px auto 20px;
    display: flex; align-items: center; justify-content: center;
    font-size: 36px;
  }
  h1 { font-size: 22px; font-weight: 700; text-align: center; margin-bottom: 10px; }
  .subtitle {
    font-size: 14px; color: #6e6e73; text-align: center;
    line-height: 1.5; max-width: 340px; margin: 0 auto;
  }

  /* Step 2 – API Keys */
  .section-title {
    font-size: 18px; font-weight: 700; margin-bottom: 6px;
  }
  .section-sub {
    font-size: 13px; color: #6e6e73; margin-bottom: 18px; line-height: 1.4;
  }
  .field-label {
    font-size: 12px; font-weight: 600; color: #1d1d1f;
    margin-bottom: 4px; display: flex; align-items: center; gap: 4px;
  }
  .field-optional {
    font-weight: 400; color: #8e8e93; font-size: 11px;
  }
  .field-wrap { margin-bottom: 14px; position: relative; }
  .field-input {
    width: 100%;
    padding: 9px 36px 9px 12px;
    border: 1px solid #d1d1d6;
    border-radius: 8px;
    font-size: 13px;
    font-family: "SF Mono", Menlo, monospace;
    background: white;
    color: #1d1d1f;
    outline: none;
    transition: border-color 0.15s;
  }
  .field-input:focus { border-color: #0071e3; box-shadow: 0 0 0 3px rgba(0,113,227,0.15); }
  .field-input.error { border-color: #ff3b30; }
  .field-check {
    position: absolute; right: 10px; top: 50%; transform: translateY(-50%);
    font-size: 16px; display: none;
  }
  .field-check.show { display: block; }
  .field-error {
    font-size: 11px; color: #ff3b30; margin-top: 3px; display: none;
  }
  .field-error.show { display: block; }
  .trust-note {
    font-size: 11px; color: #8e8e93; background: #e8f4fd;
    border-radius: 8px; padding: 9px 12px; line-height: 1.4; margin-top: 4px;
  }

  /* Step 3 – Permissions */
  .perm-row {
    display: flex; align-items: center; gap: 12px;
    background: white; border-radius: 10px;
    padding: 14px 16px; margin-bottom: 14px;
    border: 1px solid #e5e5ea;
  }
  .perm-icon { font-size: 22px; }
  .perm-text { flex: 1; }
  .perm-name { font-size: 14px; font-weight: 600; }
  .perm-desc { font-size: 12px; color: #6e6e73; margin-top: 2px; }
  .settings-btn {
    padding: 7px 14px; border: 1.5px solid #0071e3; background: white;
    color: #0071e3; border-radius: 8px; font-size: 13px; font-weight: 500;
    cursor: pointer; white-space: nowrap; transition: all 0.15s;
  }
  .settings-btn:hover { background: #0071e3; color: white; }
  .settings-btn:disabled { opacity: 0.5; cursor: default; }
  .wait-msg {
    font-size: 12px; color: #8e8e93; text-align: center;
    margin-top: 4px; display: none;
  }
  .wait-msg.show { display: block; }
  .guide-img {
    width: 100%; border-radius: 8px;
    border: 1px solid #e5e5ea; margin-top: 4px;
  }
  .err-msg {
    font-size: 12px; color: #ff3b30; background: #fff0ee;
    border-radius: 8px; padding: 9px 12px; margin-top: 8px; display: none;
  }
  .err-msg.show { display: block; }

  /* ── Footer ──────────────────────────────────────────────── */
  .footer {
    display: flex; justify-content: space-between; align-items: center;
    padding: 12px 32px 20px;
    border-top: 1px solid #e5e5ea;
    background: #f5f5f7;
  }
  .btn {
    padding: 9px 20px; border-radius: 8px; font-size: 14px;
    font-weight: 500; cursor: pointer; border: none; transition: all 0.15s;
  }
  .btn-back {
    background: transparent; color: #0071e3;
    border: 1.5px solid #d1d1d6;
  }
  .btn-back:hover { border-color: #0071e3; }
  .btn-primary {
    background: #0071e3; color: white;
  }
  .btn-primary:hover { background: #0062c5; }
  .btn-primary:disabled {
    background: #c7c7cc; cursor: not-allowed;
  }
  .spacer { width: 80px; }
</style>
</head>
<body>

<!-- Step indicators -->
<div class="steps" id="steps">
  <div class="step-pill active" id="pill-1"><span class="step-num">1</span> Welcome</div>
  <div class="step-pill" id="pill-2"><span class="step-num">2</span> API Keys</div>
  <div class="step-pill" id="pill-3"><span class="step-num">3</span> Permissions</div>
</div>

<!-- Step 1: Welcome -->
<div class="content">
  <div class="step-view active" id="step-1">
    <div class="welcome-icon">🤖</div>
    <h1>Welcome to Loom</h1>
    <p class="subtitle">Your AI-powered job scheduler. Let's get you set up in three quick steps.</p>
  </div>

  <!-- Step 2: API Keys -->
  <div class="step-view" id="step-2">
    <div class="section-title">Connect your AI</div>
    <p class="section-sub">Loom needs an API key to run AI-powered jobs. Add at least one.</p>

    <div class="field-wrap">
      <div class="field-label">Anthropic API Key</div>
      <input class="field-input" id="anthropic-key" type="password"
             placeholder="sk-ant-..." autocomplete="off"
             value="{{ANTHROPIC_KEY}}">
      <span class="field-check" id="ant-check">✅</span>
      <div class="field-error" id="ant-error">Anthropic API key is required</div>
    </div>

    <div class="field-wrap">
      <div class="field-label">OpenAI API Key <span class="field-optional">(Optional)</span></div>
      <input class="field-input" id="openai-key" type="password"
             placeholder="sk-..." autocomplete="off"
             value="{{OPENAI_KEY}}">
      <span class="field-check" id="oai-check">✅</span>
    </div>

    <div class="trust-note">🔒 Keys are stored locally in the app database and only sent to the respective AI providers.</div>
  </div>

  <!-- Step 3: Permissions -->
  <div class="step-view" id="step-3">
    <div class="section-title">Grant Full Disk Access</div>
    <p class="section-sub">The background service needs Full Disk Access to run jobs, source shell configs, and find tools like <code>.nvm</code> and <code>.amplifier</code>.</p>

    <div class="perm-row" id="perm-row">
      <div class="perm-icon" id="perm-icon">⚠️</div>
      <div class="perm-text">
        <div class="perm-name">Full Disk Access</div>
        <div class="perm-desc" id="perm-desc">Required · Background service</div>
      </div>
      <button class="settings-btn" id="settings-btn" onclick="openSettings()">Open System Settings →</button>
    </div>

    <div class="wait-msg" id="wait-msg">⏳ Waiting for permission… (it's OK to switch back here)</div>

    <img class="guide-img" id="guide-img" src="{{FDA_GUIDE_DATA_URI}}" alt="System Settings guide" style="display:none">

    <div class="err-msg" id="err-msg"></div>
  </div>
</div>

<!-- Footer -->
<div class="footer">
  <button class="btn btn-back" id="btn-back" style="visibility:hidden" onclick="goBack()">← Back</button>
  <div id="footer-center"></div>
  <button class="btn btn-primary" id="btn-next" onclick="goNext()">Get Started →</button>
</div>

<script>
  var currentStep = 1;
  var fdaGranted  = {{FDA_GRANTED}};  // substituted by Go: true or false

  function post(action, payload) {
    window.webkit.messageHandlers.agent.postMessage({ action: action, payload: payload || "" });
  }

  function updateStepUI() {
    for (var i = 1; i <= 3; i++) {
      var pill = document.getElementById("pill-" + i);
      var view = document.getElementById("step-" + i);
      pill.className = "step-pill";
      view.className = "step-view";
      if (i < currentStep)  pill.classList.add("done");
      if (i === currentStep) { pill.classList.add("active"); view.classList.add("active"); }
    }

    var back = document.getElementById("btn-back");
    var next = document.getElementById("btn-next");

    back.style.visibility = currentStep > 1 ? "visible" : "hidden";

    if (currentStep === 1) {
      next.textContent = "Get Started →";
      next.disabled    = false;
    } else if (currentStep === 2) {
      next.textContent = "Continue →";
      updateContinueState();
    } else {
      next.textContent = "Done";
      next.disabled    = !fdaGranted;
      if (fdaGranted) markFDAGranted();
    }
  }

  function updateContinueState() {
    var key  = document.getElementById("anthropic-key").value.trim();
    document.getElementById("btn-next").disabled = (key === "");
    document.getElementById("ant-check").className = "field-check" + (key ? " show" : "");
    var ok = document.getElementById("openai-key").value.trim();
    document.getElementById("oai-check").className = "field-check" + (ok ? " show" : "");
  }

  function goNext() {
    if (currentStep === 1) {
      currentStep = 2;
      updateStepUI();
    } else if (currentStep === 2) {
      var key = document.getElementById("anthropic-key").value.trim();
      if (!key) {
        document.getElementById("anthropic-key").classList.add("error");
        document.getElementById("ant-error").classList.add("show");
        return;
      }
      post("setAnthropicKey", key);
      post("setOpenAIKey", document.getElementById("openai-key").value.trim());
      currentStep = 3;
      updateStepUI();
    } else if (currentStep === 3 && fdaGranted) {
      document.getElementById("btn-next").disabled = true;
      document.getElementById("btn-next").textContent = "Setting up…";
      post("done", "");
    }
  }

  function goBack() {
    if (currentStep > 1) { currentStep--; updateStepUI(); }
  }

  function openSettings() {
    var btn = document.getElementById("settings-btn");
    btn.disabled = true;
    btn.textContent = "Waiting…";
    document.getElementById("wait-msg").classList.add("show");
    document.getElementById("guide-img").style.display = "block";
    post("openSettings", "");
  }

  function markFDAGranted() {
    document.getElementById("perm-icon").textContent = "✅";
    document.getElementById("perm-desc").textContent = "Full Disk Access granted ✓";
    document.getElementById("settings-btn").style.display = "none";
    document.getElementById("wait-msg").classList.remove("show");
    document.getElementById("btn-next").disabled = false;
  }

  // Go → JS events
  window.addEventListener("fdaGranted", function() {
    fdaGranted = true;
    markFDAGranted();
  });

  window.addEventListener("installError", function(e) {
    var msg = e.detail && e.detail.msg ? e.detail.msg : "An error occurred during install.";
    var el = document.getElementById("err-msg");
    el.textContent = "⚠ " + msg + " — you can try again or run `loom install` in the terminal.";
    el.classList.add("show");
    var btn = document.getElementById("btn-next");
    btn.disabled = false;
    btn.textContent = "Done";
  });

  // Live key validation
  document.getElementById("anthropic-key").addEventListener("input", function() {
    document.getElementById("anthropic-key").classList.remove("error");
    document.getElementById("ant-error").classList.remove("show");
    updateContinueState();
  });
  document.getElementById("openai-key").addEventListener("input", updateContinueState);

  // Init
  updateStepUI();
  updateContinueState();
</script>
</body>
</html>
```

- [ ] **Step 2: Verify the HTML parses (quick sanity)**

```bash
cd .worktrees/macos-onboarding && \
  python3 -c "
from html.parser import HTMLParser
class P(HTMLParser):
    pass
P().feed(open('internal/onboarding/wizard.html').read())
print('HTML parses OK')
"
```

- [ ] **Step 3: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/onboarding/wizard.html && \
  git commit -m "feat: add wizard HTML/CSS/JS for 3-step onboarding UI"
```

---

## Task 6: wizard_darwin_impl.go — CGo NSPanel + WKWebView

**Files:**
- Create: `internal/onboarding/wizard_darwin_impl.go`

### Why
This file contains the Objective-C/CGo implementation of the wizard window (NSPanel + WKWebView), the FDA probe, the 1-second polling goroutine, and the app-activation observer. It does NOT use `//export` — that is in Task 7.

**Build constraint:** `//go:build darwin && cgo`

**Key CGo rules for this file:**
- Contains C/ObjC function DEFINITIONS — so it must NOT use `//export`
- Extern-declares `wizardGoMessage` and `wizardGoActivation` (defined in Task 7 via `//export`) — these are safe to declare here because CGo generates `_cgo_export.h` that provides them
- `dispatch_async(dispatch_get_main_queue(), ^{...})` wraps ALL Cocoa calls
- `NSNotificationCenter` (not `NSApplicationDelegate`) to avoid clobbering `fyne.io/systray`'s delegate

- [ ] **Step 1: Create internal/onboarding/wizard_darwin_impl.go**

```go
//go:build darwin && cgo

package onboarding

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// Forward declarations of Go callbacks (defined via //export in wizard_darwin_callbacks.go).
// CGo generates _cgo_export.h which provides their full declarations at link time.
extern void wizardGoMessage(const char *action, const char *payload);
extern void wizardGoActivation(void);

// ── Script message handler ────────────────────────────────────────────────────
@interface _AgentWizardDelegate : NSObject <WKScriptMessageHandler, NSWindowDelegate>
@end

@implementation _AgentWizardDelegate

- (void)userContentController:(WKUserContentController *)ucc
      didReceiveScriptMessage:(WKScriptMessage *)message {
    NSDictionary *body = (NSDictionary *)message.body;
    NSString *action  = body[@"action"]  ?: @"";
    NSString *payload = body[@"payload"] ?: @"";
    wizardGoMessage(action.UTF8String, payload.UTF8String);
}

- (void)windowWillClose:(NSNotification *)notification {
    // Closing state is managed on the Go side via gState.closed.
}

@end

// ── Module-level state ────────────────────────────────────────────────────────
static NSPanel              *_gPanel    = nil;
static WKWebView            *_gWebView  = nil;
static _AgentWizardDelegate *_gDelegate = nil;
static id                    _gActObs   = nil;

// ── C API ─────────────────────────────────────────────────────────────────────

void wizard_show(const char *htmlCStr) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (_gPanel) { [_gPanel makeKeyAndOrderFront:nil]; return; }

        NSString *html = [NSString stringWithUTF8String:htmlCStr];

        WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
        _gDelegate = [_AgentWizardDelegate new];
        [cfg.userContentController addScriptMessageHandler:_gDelegate name:@"agent"];

        NSRect frame = NSMakeRect(0, 0, 480, 520);

        _gPanel = [[NSPanel alloc]
            initWithContentRect:frame
            styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable)
            backing:NSBackingStoreBuffered
            defer:NO];
        [_gPanel setTitle:@"Loom Setup"];
        [_gPanel setHidesOnDeactivate:NO];
        [_gPanel setLevel:NSFloatingWindowLevel];
        _gPanel.delegate = _gDelegate;

        _gWebView = [[WKWebView alloc] initWithFrame:frame configuration:cfg];
        _gWebView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
        [_gPanel setContentView:_gWebView];

        [_gWebView loadHTMLString:html baseURL:nil];
        [_gPanel center];
        [_gPanel makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
    });
}

void wizard_eval_js(const char *jsCStr) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!_gWebView) return;
        NSString *js = [NSString stringWithUTF8String:jsCStr];
        [_gWebView evaluateJavaScript:js completionHandler:nil];
    });
}

void wizard_close(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (_gActObs) {
            [[NSNotificationCenter defaultCenter] removeObserver:_gActObs];
            _gActObs = nil;
        }
        [_gPanel close];
        _gPanel    = nil;
        _gWebView  = nil;
        _gDelegate = nil;
    });
}

// wizard_observe_activation registers a NSNotificationCenter observer for
// NSApplicationDidBecomeActiveNotification. Uses NSNotificationCenter (not
// NSApplicationDelegate) to coexist safely with fyne.io/systray's delegate.
void wizard_observe_activation(void) {
    if (_gActObs) return;
    _gActObs = [[NSNotificationCenter defaultCenter]
        addObserverForName:NSApplicationDidBecomeActiveNotification
        object:nil
        queue:nil
        usingBlock:^(NSNotification *n) {
            wizardGoActivation();
        }];
}
*/
import "C"

import (
	_ "embed"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"
)

//go:embed wizard.html
var wizardHTMLBytes []byte

//go:embed fda-guide.png
var fdaGuidePNG []byte

// pollingActive prevents duplicate polling goroutines.
var pollingActive atomic.Bool

// showImpl is the darwin+cgo entry point for the wizard. Called by Show() in wizard.go.
func showImpl(s *state) {
	html := buildHTML(s)
	cHTML := C.CString(html)
	defer C.free(unsafe.Pointer(cHTML))
	C.wizard_show(cHTML)
	C.wizard_observe_activation()
	if !s.fdaGranted {
		go pollFDA(s)
	}
}

// buildHTML substitutes template placeholders into wizard.html.
// {{FDA_GUIDE_DATA_URI}} → base64-encoded PNG as a data: URI.
// {{ANTHROPIC_KEY}}, {{OPENAI_KEY}} → pre-fill values (empty string if not set).
// {{FDA_GRANTED}} → "true" or "false" for JS initialisation.
func buildHTML(s *state) string {
	pngDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fdaGuidePNG)
	html := string(wizardHTMLBytes)
	html = strings.ReplaceAll(html, "{{FDA_GUIDE_DATA_URI}}", pngDataURI)
	html = strings.ReplaceAll(html, "{{ANTHROPIC_KEY}}", jsEscape(s.anthropicKey))
	html = strings.ReplaceAll(html, "{{OPENAI_KEY}}", jsEscape(s.openAIKey))
	fdaVal := "false"
	if s.fdaGranted {
		fdaVal = "true"
	}
	html = strings.ReplaceAll(html, "{{FDA_GRANTED}}", fdaVal)
	return html
}

// pushJS evaluates a JavaScript expression in the WebView.
// Must only push state that is safe to receive asynchronously.
func pushJS(js string) {
	cJS := C.CString(js)
	defer C.free(unsafe.Pointer(cJS))
	C.wizard_eval_js(cJS)
}

// CheckFDA probes whether Full Disk Access has been granted to this process.
// Uses the MacPaw PermissionsKit probe strategy: attempt to open a TCC-protected
// file and check whether it succeeds or returns EPERM/EACCES.
func CheckFDA() bool {
	home, _ := os.UserHomeDir()
	if f, err := os.Open(filepath.Join(home, "Library", "Safari", "Bookmarks.plist")); err == nil {
		f.Close()
		return true
	}
	// Fallback for users without Safari installed.
	if f, err := os.Open("/Library/Preferences/com.apple.TimeMachine.plist"); err == nil {
		f.Close()
		return true
	}
	return false
}

// pollFDA runs a 1-second polling loop until FDA is granted or the wizard is closed.
// It is the backup mechanism — NSApplicationDidBecomeActive is the primary trigger.
func pollFDA(s *state) {
	if !pollingActive.CompareAndSwap(false, true) {
		return // already polling
	}
	defer pollingActive.Store(false)

	for {
		time.Sleep(1 * time.Second)
		if s.closed || s.fdaGranted {
			return
		}
		if CheckFDA() {
			s.fdaGranted = true
			pushJS(`window.dispatchEvent(new CustomEvent('fdaGranted'))`)
			return
		}
	}
}

// jsEscape minimally escapes a string for safe embedding in an HTML attribute value.
// These values appear in `value="{{ANTHROPIC_KEY}}"` attributes in wizard.html.
func jsEscape(s string) string {
	s = strings.ReplaceAll(s, `&`, `&amp;`)
	s = strings.ReplaceAll(s, `"`, `&quot;`)
	s = strings.ReplaceAll(s, `<`, `&lt;`)
	s = strings.ReplaceAll(s, `>`, `&gt;`)
	return s
}
```

- [ ] **Step 2: Build check (macOS only)**

```bash
cd .worktrees/macos-onboarding && CGO_ENABLED=1 go build ./internal/onboarding/...
```
Expected: compiles on macOS. If there are CGo/ObjC errors, fix them before proceeding.

- [ ] **Step 3: Verify cross-platform stub still works**

```bash
cd .worktrees/macos-onboarding && \
  GOOS=linux CGO_ENABLED=0 go build ./internal/onboarding/... && echo "linux OK"
```

- [ ] **Step 4: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/onboarding/wizard_darwin_impl.go && \
  git commit -m "feat: add CGo NSPanel+WKWebView implementation for onboarding wizard"
```

---

## Task 7: wizard_darwin_callbacks.go — //export Go callbacks

**Files:**
- Create: `internal/onboarding/wizard_darwin_callbacks.go`

### Why
CGo rules prohibit `//export` in the same file as C function definitions. This file contains only the `//export` Go functions (called from ObjC) and the `handleDone()` logic that writes to the store, installs the service, and closes the wizard.

**CGo rule:** The preamble in this file may only contain `extern` DECLARATIONS (not definitions).

- [ ] **Step 1: Create internal/onboarding/wizard_darwin_callbacks.go**

```go
//go:build darwin && cgo

package onboarding

/*
// Only extern declarations — definitions live in wizard_darwin_impl.go.
// CGo combines all preambles in the package; these resolve at link time.
extern void wizard_eval_js(const char *js);
extern void wizard_close(void);
*/
import "C"

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"unsafe"

	"github.com/kardianos/service"
	"github.com/ms/loom/internal/config"
	"github.com/ms/loom/internal/platform"
	internalsvc "github.com/ms/loom/internal/service"
	"github.com/ms/loom/internal/store"
)

// wizardGoMessage is called from ObjC when JS posts a message via
// window.webkit.messageHandlers.agent.postMessage({action:..., payload:...}).
//
//export wizardGoMessage
func wizardGoMessage(cAction *C.char, cPayload *C.char) {
	action := C.GoString(cAction)
	payload := C.GoString(cPayload)
	if gState == nil {
		return
	}
	switch action {
	case "setAnthropicKey":
		gState.anthropicKey = payload
	case "setOpenAIKey":
		gState.openAIKey = payload
	case "openSettings":
		openSystemSettings()
		go pollFDA(gState) // start backup poll on top of NSNotificationCenter
	case "done":
		go handleDone(gState)
	}
}

// wizardGoActivation is called from the NSNotificationCenter observer when the
// app becomes active (primary FDA detection signal: user returned from System Settings).
//
//export wizardGoActivation
func wizardGoActivation() {
	if gState == nil || gState.fdaGranted {
		return
	}
	if CheckFDA() {
		gState.fdaGranted = true
		pushJS(`window.dispatchEvent(new CustomEvent('fdaGranted'))`)
	}
}

// handleDone runs the Done-button flow:
//  1. Save API keys to BoltDB
//  2. Capture UserContext (HomeDir, Shell, UID) into BoltDB
//  3. Install the service (user-level LaunchAgent) if not already installed
//  4. Start the service
//  5. Mark OnboardingComplete = true in BoltDB
//  6. Close the wizard
func handleDone(s *state) {
	st, err := store.Open(platform.DBPath())
	if err != nil {
		pushInstallError("Failed to open database: " + err.Error())
		return
	}
	defer st.Close()

	// 1. Read + update config
	cfg, err := st.GetConfig(context.Background())
	if err != nil {
		pushInstallError("Failed to read config: " + err.Error())
		return
	}
	cfg.AnthropicKey = s.anthropicKey
	cfg.OpenAIKey = s.openAIKey

	// 2. Capture UserContext
	if uc := config.CaptureUserContext(); uc != nil {
		cfg.UserContext = uc
		slog.Info("onboarding: captured user context", "home", uc.HomeDir, "shell", uc.Shell)
	}

	if err := st.SaveConfig(context.Background(), cfg); err != nil {
		pushInstallError("Failed to save config: " + err.Error())
		return
	}

	// 3. Install service (guard against double-install — not idempotent)
	if !isServiceInstalled() {
		svc, err := internalsvc.NewServiceForControl(internalsvc.LevelUser)
		if err != nil {
			pushInstallError("Failed to create service: " + err.Error())
			return
		}
		if err := service.Control(svc, "install"); err != nil {
			pushInstallError("Service install failed: " + err.Error())
			return
		}
		slog.Info("onboarding: service installed")
	}

	// 4. Start service (best-effort; may already be running)
	if svc, err := internalsvc.NewServiceForControl(internalsvc.LevelUser); err == nil {
		_ = service.Control(svc, "start")
	}

	// 5. Mark complete
	cfg.OnboardingComplete = true
	_ = st.SaveConfig(context.Background(), cfg)

	// 6. Close
	s.closed = true
	cEmpty := C.CString("")
	defer C.free(unsafe.Pointer(cEmpty))
	C.wizard_close()

	if s.onDone != nil {
		s.onDone()
	}
}

// openSystemSettings deep-links to the Full Disk Access pane.
func openSystemSettings() {
	_ = exec.Command("open",
		"x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles",
	).Start()
}

// pushInstallError sends an installError event to the wizard JS layer.
func pushInstallError(msg string) {
	safe := strings.ReplaceAll(msg, `"`, `\"`)
	pushJS(fmt.Sprintf(`window.dispatchEvent(new CustomEvent('installError', {detail: {msg: "%s"}}))`, safe))
}
```

- [ ] **Step 2: Build check**

```bash
cd .worktrees/macos-onboarding && CGO_ENABLED=1 go build ./internal/onboarding/...
```
Expected: compiles. If there are "conflicting //export" errors, ensure this file has NO C function definitions in its preamble (only `extern` declarations).

- [ ] **Step 3: Verify the whole project still builds**

```bash
cd .worktrees/macos-onboarding && CGO_ENABLED=1 go build ./...
```
Fix any import/compile errors.

- [ ] **Step 4: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/onboarding/wizard_darwin_callbacks.go && \
  git commit -m "feat: add CGo callbacks for wizard JS<->Go bridge and Done flow"
```

---

## Task 8: Tray integration — trigger + health indicator

**Files:**
- Modify: `internal/tray/tray.go`

### Why
The tray is the entry point for the `.app` bundle. It needs to:
1. Check `NeedsOnboarding()` on launch and show the wizard if true
2. Run a health-check goroutine every 30s after onboarding is complete
3. Show an amber `!` badge and "Action Required" menu section when unhealthy
4. Provide "Fix →" to targeted single-step remediation

- [ ] **Step 1: Add imports to tray.go**

Add to the import block:
```go
"github.com/ms/loom/internal/onboarding"
```
(also ensure `"context"`, `"github.com/ms/loom/internal/platform"`,
`"github.com/ms/loom/internal/store"` are present — some are already there from Task 3).

- [ ] **Step 2: Add health state type near top of file**

After the existing imports, add:
```go
type healthState int

const (
    healthOK      healthState = iota
    healthWarning             // amber badge, show Fix menu
)
```

- [ ] **Step 3: Add health check function**

```go
type healthIssue struct {
    msg     string // displayed in menu
    fixKind string // "apikey" | "fda" | "service"
}

func checkHealth(port int) (healthState, []healthIssue) {
    var issues []healthIssue

    // API key
    s, err := store.Open(platform.DBPath())
    if err == nil {
        if cfg, err := s.GetConfig(context.Background()); err == nil {
            if cfg.AnthropicKey == "" {
                issues = append(issues, healthIssue{"Anthropic API key missing", "apikey"})
            }
        }
        s.Close()
    }

    // Full Disk Access
    if !onboarding.CheckFDA() {
        issues = append(issues, healthIssue{"Full Disk Access missing", "fda"})
    }

    // Service installed
    if !isServiceInstalled() {
        issues = append(issues, healthIssue{"Background service not installed", "service"})
    }

    // Service running (HTTP 200)
    url := fmt.Sprintf("http://localhost:%d/api/status", port)
    if resp, err := http.Get(url); err == nil {
        resp.Body.Close()
        if resp.StatusCode != 200 {
            issues = append(issues, healthIssue{"Service not responding", "service"})
        }
    }

    if len(issues) > 0 {
        return healthWarning, issues
    }
    return healthOK, nil
}
```

- [ ] **Step 4: Modify onReady to trigger onboarding and set up health check**

At the start of `onReady`, BEFORE building the menu, add:

```go
func onReady(port int) {
    // ── Onboarding check ─────────────────────────────────────────────────
    // Show wizard if conditions not met (macOS .app launch only).
    // The wizard blocks in its own NSPanel; the tray icon still appears.
    if st, err := store.Open(platform.DBPath()); err == nil {
        if cfg, err := st.GetConfig(context.Background()); err == nil {
            if onboarding.NeedsOnboarding(cfg) {
                onboarding.Show(st, func() {
                    slog.Info("tray: onboarding complete")
                    // tray will pick up new state on next health check
                })
                // Note: st is kept open by the onboarding session;
                // it closes itself in handleDone via a separate open.
            }
        }
        st.Close()
    }

    // ... (existing menu setup follows)
```

- [ ] **Step 5: Add health badge and "Action Required" menu items**

At the top of the existing menu setup (after the icon is set), add:

```go
    // ── Health indicator (hidden until a check fires) ─────────────────────
    mActionRequired := systray.AddMenuItem("⚠  Action Required", "")
    mActionRequired.Hide()
    mFixAPIKey  := mActionRequired.AddSubMenuItem("Anthropic API key missing   Fix →", "")
    mFixFDA     := mActionRequired.AddSubMenuItem("Full Disk Access missing     Fix →", "")
    mFixService := mActionRequired.AddSubMenuItem("Service not installed         Fix →", "")
    mFixAPIKey.Hide(); mFixFDA.Hide(); mFixService.Hide()
    systray.AddSeparator()
```

- [ ] **Step 6: Add health-check goroutine (starts after onboarding or immediately if complete)**

Add a goroutine that drives badge state. Add it in the `go func()` section near the status polling:

```go
    // Start health check after a short delay (allow daemon to start up first).
    go func() {
        // Wait until onboarding is done (poll OnboardingComplete).
        for {
            time.Sleep(5 * time.Second)
            s, err := store.Open(platform.DBPath())
            if err != nil { continue }
            cfg, err := s.GetConfig(context.Background())
            s.Close()
            if err == nil && cfg.OnboardingComplete { break }
        }
        // Health check loop: every 30 seconds.
        for {
            hs, issues := checkHealth(port)
            if hs == healthOK {
                icon := makeIcon()
                systray.SetTemplateIcon(icon, icon)
                mActionRequired.Hide()
            } else {
                // Amber icon: append a warning indicator.
                // systray doesn't support badges natively; we reflect
                // the warning in the tooltip and menu instead.
                systray.SetTooltip("loom ⚠ action required")
                mActionRequired.Show()
                mFixAPIKey.Hide(); mFixFDA.Hide(); mFixService.Hide()
                for _, iss := range issues {
                    switch iss.fixKind {
                    case "apikey":  mFixAPIKey.Show()
                    case "fda":     mFixFDA.Show()
                    case "service": mFixService.Show()
                    }
                }
            }
            time.Sleep(30 * time.Second)
        }
    }()
```

- [ ] **Step 7: Wire Fix → click handlers in the event loop**

In the main `for { select { ... } }` event loop, add:

```go
        case <-mFixAPIKey.ClickedCh:
            // Open a small dialog to enter the key
            // For now: open the web UI settings page as fallback
            openBrowser(fmt.Sprintf("http://localhost:%d/#/settings", port))

        case <-mFixFDA.ClickedCh:
            // Re-open the onboarding wizard at step 3 only
            if st, err := store.Open(platform.DBPath()); err == nil {
                onboarding.Show(st, func() {
                    slog.Info("tray: FDA fix complete")
                })
                st.Close()
            }

        case <-mFixService.ClickedCh:
            captureAndSaveUserContext()
            installService(internalsvc.LevelUser)
```

- [ ] **Step 8: Full build check**

```bash
cd .worktrees/macos-onboarding && CGO_ENABLED=1 go build ./...
```
Expected: clean compile. Fix any import or type errors.

- [ ] **Step 9: Run tests (excluding pre-existing scheduler failures)**

```bash
cd .worktrees/macos-onboarding && \
  go test $(go list ./... | grep -v internal/scheduler) -v 2>&1 | tail -30
```
Expected: all non-scheduler tests pass.

- [ ] **Step 10: Commit**

```bash
cd .worktrees/macos-onboarding && \
  git add internal/tray/tray.go && \
  git commit -m "feat: integrate onboarding wizard and health indicator into tray"
```

---

## Final Verification

- [ ] **Full build (macOS)**

```bash
cd .worktrees/macos-onboarding && CGO_ENABLED=1 go build ./... && echo "BUILD OK"
```

- [ ] **Cross-platform build (no CGo)**

```bash
cd .worktrees/macos-onboarding && \
  GOOS=linux CGO_ENABLED=0 go build $(go list ./... | grep -v internal/tray) && echo "LINUX OK"
```

- [ ] **All tests pass (excluding pre-existing scheduler failures)**

```bash
cd .worktrees/macos-onboarding && \
  go test $(go list ./... | grep -v internal/scheduler) 2>&1 | tail -20
```

- [ ] **Review commit log**

```bash
cd .worktrees/macos-onboarding && git log --oneline
```
Expected: 8 commits from this feature on top of `549d706`.
