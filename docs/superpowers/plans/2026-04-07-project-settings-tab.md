# Project Settings Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Settings" tab to the right panel of the Projects view that reads and writes `<project>/.amplifier/settings.yaml`, exposing all Amplifier project-level configuration through a structured collapsible UI.

**Architecture:** New Go structs in `internal/amplifier/project_settings.go` define the `settings.yaml` schema. Two new HTTP handlers in `internal/api/handlers_project_settings.go` expose GET/PUT routes at `/api/projects/{id}/settings`. A new React component `ProjectSettingsPanel.tsx` renders collapsible sections (Bundle, Providers, Routing, Filesystem, Notifications, Overrides) using data from those endpoints. The Settings tab is added to the existing right panel tab system in `WorkspaceApp.tsx`.

**Tech Stack:** Go + `gopkg.in/yaml.v3` (already in go.mod), React 18 + TypeScript, existing Loom UI CSS variable design system.

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/amplifier/project_settings.go` | YAML structs + `ReadProjectSettings` / `WriteProjectSettings` helpers |
| Create | `internal/api/handlers_project_settings.go` | `getProjectSettings` and `updateProjectSettings` HTTP handlers |
| Modify | `internal/api/server.go` | Register 2 new routes after existing project routes |
| Modify | `ui/src/api/projects.ts` | `ProjectSettings` TypeScript interface + 2 API functions |
| Create | `ui/src/views/projects/ProjectSettingsPanel.tsx` | Full Settings panel: Bundle, Providers, Routing, Filesystem, Notifications, Overrides |
| Modify | `ui/src/views/projects/WorkspaceApp.tsx` | Add `'settings'` to tab type union, render `ProjectSettingsPanel` |

---

## Task 1: Go YAML structs + file helpers

**Files:**
- Create: `internal/amplifier/project_settings.go`

- [ ] **Step 1: Write the test first**

```go
// internal/amplifier/project_settings_test.go
package amplifier_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/amplifier"
)

func TestReadProjectSettings_missing(t *testing.T) {
	s, err := amplifier.ReadProjectSettings("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if s.Bundle != nil {
		t.Error("expected nil Bundle for missing file")
	}
}

func TestReadWriteProjectSettings_roundtrip(t *testing.T) {
	dir := t.TempDir()
	amplifierDir := filepath.Join(dir, ".amplifier")
	if err := os.MkdirAll(amplifierDir, 0755); err != nil {
		t.Fatal(err)
	}

	in := amplifier.ProjectSettings{
		Bundle: &amplifier.BundleSettings{
			Active: "foundation",
			App:    []string{"git+https://github.com/microsoft/lifeos@main"},
		},
		Routing: &amplifier.RoutingSettings{Matrix: "balanced"},
	}

	if err := amplifier.WriteProjectSettings(dir, in); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, err := amplifier.ReadProjectSettings(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Bundle == nil || out.Bundle.Active != "foundation" {
		t.Errorf("Bundle.Active: got %v", out.Bundle)
	}
	if out.Routing == nil || out.Routing.Matrix != "balanced" {
		t.Errorf("Routing.Matrix: got %v", out.Routing)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/ken/workspace/ms/loom
go test ./internal/amplifier/... -run TestReadProjectSettings -v
```

Expected: `FAIL — amplifier.ReadProjectSettings undefined`

- [ ] **Step 3: Create the implementation**

```go
// internal/amplifier/project_settings.go
package amplifier

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProjectSettings mirrors the schema of <project>/.amplifier/settings.yaml.
// All fields are pointers/omitempty so absent keys round-trip cleanly.
type ProjectSettings struct {
	Bundle    *BundleSettings         `yaml:"bundle,omitempty"    json:"bundle,omitempty"`
	Config    *ProjectConfigSettings  `yaml:"config,omitempty"    json:"config,omitempty"`
	Modules   *ModulesSettings        `yaml:"modules,omitempty"   json:"modules,omitempty"`
	Overrides map[string]OverrideEntry `yaml:"overrides,omitempty" json:"overrides,omitempty"`
	Sources   *SourcesSettings        `yaml:"sources,omitempty"   json:"sources,omitempty"`
	Routing   *RoutingSettings        `yaml:"routing,omitempty"   json:"routing,omitempty"`
}

type BundleSettings struct {
	Active string            `yaml:"active,omitempty" json:"active,omitempty"`
	App    []string          `yaml:"app,omitempty"    json:"app,omitempty"`
	Added  map[string]string `yaml:"added,omitempty"  json:"added,omitempty"`
}

type ProjectConfigSettings struct {
	Providers     []ProviderEntry       `yaml:"providers,omitempty"     json:"providers,omitempty"`
	Notifications *NotificationsConfig  `yaml:"notifications,omitempty" json:"notifications,omitempty"`
}

type ProviderEntry struct {
	Module string                 `yaml:"module"           json:"module"`
	Source string                 `yaml:"source,omitempty" json:"source,omitempty"`
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

type NotificationsConfig struct {
	Desktop *DesktopNotifConfig `yaml:"desktop,omitempty" json:"desktop,omitempty"`
	Push    *PushNotifConfig    `yaml:"push,omitempty"    json:"push,omitempty"`
}

type DesktopNotifConfig struct {
	Enabled             *bool  `yaml:"enabled,omitempty"               json:"enabled,omitempty"`
	ShowDevice          *bool  `yaml:"show_device,omitempty"           json:"show_device,omitempty"`
	ShowProject         *bool  `yaml:"show_project,omitempty"          json:"show_project,omitempty"`
	ShowPreview         *bool  `yaml:"show_preview,omitempty"          json:"show_preview,omitempty"`
	PreviewLength       *int   `yaml:"preview_length,omitempty"        json:"preview_length,omitempty"`
	Subtitle            string `yaml:"subtitle,omitempty"              json:"subtitle,omitempty"`
	SuppressIfFocused   *bool  `yaml:"suppress_if_focused,omitempty"   json:"suppress_if_focused,omitempty"`
	MinIterations       *int   `yaml:"min_iterations,omitempty"        json:"min_iterations,omitempty"`
	ShowIterationCount  *bool  `yaml:"show_iteration_count,omitempty"  json:"show_iteration_count,omitempty"`
	Sound               string `yaml:"sound,omitempty"                 json:"sound,omitempty"`
	Debug               *bool  `yaml:"debug,omitempty"                 json:"debug,omitempty"`
}

type PushNotifConfig struct {
	Enabled  *bool    `yaml:"enabled,omitempty"  json:"enabled,omitempty"`
	Server   string   `yaml:"server,omitempty"   json:"server,omitempty"`
	Priority string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	Tags     []string `yaml:"tags,omitempty"     json:"tags,omitempty"`
	Debug    *bool    `yaml:"debug,omitempty"    json:"debug,omitempty"`
}

type ModulesSettings struct {
	Tools []ToolModuleEntry `yaml:"tools,omitempty" json:"tools,omitempty"`
}

type ToolModuleEntry struct {
	Module string      `yaml:"module"           json:"module"`
	Config *ToolConfig `yaml:"config,omitempty" json:"config,omitempty"`
}

type ToolConfig struct {
	AllowedWritePaths []string `yaml:"allowed_write_paths,omitempty" json:"allowed_write_paths,omitempty"`
	AllowedReadPaths  []string `yaml:"allowed_read_paths,omitempty"  json:"allowed_read_paths,omitempty"`
	DeniedWritePaths  []string `yaml:"denied_write_paths,omitempty"  json:"denied_write_paths,omitempty"`
}

type OverrideEntry struct {
	Source string                 `yaml:"source,omitempty" json:"source,omitempty"`
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

type SourcesSettings struct {
	Modules map[string]string `yaml:"modules,omitempty" json:"modules,omitempty"`
	Bundles map[string]string `yaml:"bundles,omitempty" json:"bundles,omitempty"`
}

type RoutingSettings struct {
	Matrix    string            `yaml:"matrix,omitempty"    json:"matrix,omitempty"`
	Overrides map[string]string `yaml:"overrides,omitempty" json:"overrides,omitempty"`
}

// ReadProjectSettings reads <projectPath>/.amplifier/settings.yaml.
// Returns empty ProjectSettings (no error) if the file does not exist.
func ReadProjectSettings(projectPath string) (ProjectSettings, error) {
	settingsPath := filepath.Join(projectPath, ".amplifier", "settings.yaml")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProjectSettings{}, nil
		}
		return ProjectSettings{}, err
	}
	var s ProjectSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return ProjectSettings{}, err
	}
	return s, nil
}

// WriteProjectSettings writes settings to <projectPath>/.amplifier/settings.yaml,
// creating the .amplifier directory if needed.
func WriteProjectSettings(projectPath string, s ProjectSettings) error {
	amplifierDir := filepath.Join(projectPath, ".amplifier")
	if err := os.MkdirAll(amplifierDir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(amplifierDir, "settings.yaml"), data, 0644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/ken/workspace/ms/loom
go test ./internal/amplifier/... -run "TestReadProjectSettings|TestReadWriteProjectSettings" -v
```

Expected: both tests PASS

- [ ] **Step 5: Verify the package compiles cleanly**

```bash
cd /Users/ken/workspace/ms/loom
go build ./internal/amplifier/...
```

Expected: no output (clean compile)

- [ ] **Step 6: Commit**

```bash
git add internal/amplifier/project_settings.go internal/amplifier/project_settings_test.go
git commit -m "feat: add ProjectSettings YAML structs and read/write helpers"
```

---

## Task 2: Go HTTP handlers + route registration

**Files:**
- Create: `internal/api/handlers_project_settings.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Write the test first**

```go
// internal/api/handlers_project_settings_test.go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/amplifier"
	"github.com/ms/amplifier-app-loom/internal/workspaces"
)

func TestGetProjectSettings_empty(t *testing.T) {
	srv := newTestServer(t)
	tmp := t.TempDir()

	// create project pointing at tmp (no .amplifier dir yet)
	ctx := t.Context()
	p, err := srv.WorkspaceStore().CreateProject(ctx, "test-proj", tmp)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/projects/"+p.ID+"/settings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got amplifier.ProjectSettings
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// empty settings — no bundle key
	if got.Bundle != nil {
		t.Errorf("expected nil bundle, got %+v", got.Bundle)
	}
}

func TestPutProjectSettings_writesYAML(t *testing.T) {
	srv := newTestServer(t)
	tmp := t.TempDir()

	ctx := t.Context()
	p, err := srv.WorkspaceStore().CreateProject(ctx, "test-proj2", tmp)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	body := amplifier.ProjectSettings{
		Bundle: &amplifier.BundleSettings{Active: "foundation"},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest("PUT", "/api/projects/"+p.ID+"/settings",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// verify file was written on disk
	data, err := os.ReadFile(filepath.Join(tmp, ".amplifier", "settings.yaml"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Contains(data, []byte("foundation")) {
		t.Errorf("expected 'foundation' in settings.yaml, got: %s", data)
	}
}
```

Note: `newTestServer` already exists in `handlers_projects_test.go`. The test requires `srv.WorkspaceStore()` to be accessible — add a thin accessor to `Server` if it isn't already present (see Step 3b below).

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/ken/workspace/ms/loom
go test ./internal/api/... -run "TestGetProjectSettings|TestPutProjectSettings" -v
```

Expected: `FAIL — s.getProjectSettings undefined`

- [ ] **Step 3a: Create the handler file**

```go
// internal/api/handlers_project_settings.go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ms/amplifier-app-loom/internal/amplifier"
)

// GET /api/projects/{id}/settings
func (s *Server) getProjectSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.workspaceStore.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found: "+err.Error())
		return
	}
	settings, err := amplifier.ReadProjectSettings(p.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read settings: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// PUT /api/projects/{id}/settings
func (s *Server) updateProjectSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.workspaceStore.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found: "+err.Error())
		return
	}
	var settings amplifier.ProjectSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := amplifier.WriteProjectSettings(p.Path, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write settings: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
```

- [ ] **Step 3b: Register routes in `server.go`**

In `internal/api/server.go`, find the project routes block (around line 160) and add two lines after `DELETE /api/projects/{id}`:

```go
	mux.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)
	// ↓ add these two lines:
	mux.HandleFunc("GET /api/projects/{id}/settings", s.getProjectSettings)
	mux.HandleFunc("PUT /api/projects/{id}/settings", s.updateProjectSettings)
```

- [ ] **Step 3c: Add `WorkspaceStore()` accessor to `Server` (needed by test)**

In `internal/api/server.go`, after `func (s *Server) Stop()`, add:

```go
// WorkspaceStore exposes the workspace service for testing.
func (s *Server) WorkspaceStore() *workspaces.Service {
	return s.workspaceStore
}
```

Also add `"github.com/ms/amplifier-app-loom/internal/workspaces"` to the import in `server.go` if not already present (it may already be there via `SetWorkspaces`).

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/ken/workspace/ms/loom
go test ./internal/api/... -run "TestGetProjectSettings|TestPutProjectSettings" -v
```

Expected: both tests PASS

- [ ] **Step 5: Verify full build is clean**

```bash
cd /Users/ken/workspace/ms/loom
go build ./...
```

Expected: no output

- [ ] **Step 6: Commit**

```bash
git add internal/api/handlers_project_settings.go \
        internal/api/handlers_project_settings_test.go \
        internal/api/server.go
git commit -m "feat: add GET/PUT /api/projects/{id}/settings endpoints"
```

---

## Task 3: TypeScript types + API client

**Files:**
- Modify: `ui/src/api/projects.ts`

- [ ] **Step 1: Append to `ui/src/api/projects.ts`**

Add after the last existing export:

```typescript
// ── Project settings (.amplifier/settings.yaml) ──────────────────────────────

export interface BundleSettings {
  active?: string
  app?: string[]
  added?: Record<string, string>
}

export interface ProviderEntry {
  module: string
  source?: string
  config?: Record<string, unknown>
}

export interface DesktopNotifConfig {
  enabled?: boolean
  show_device?: boolean
  show_project?: boolean
  show_preview?: boolean
  preview_length?: number
  subtitle?: string
  suppress_if_focused?: boolean
  min_iterations?: number
  show_iteration_count?: boolean
  sound?: string
  debug?: boolean
}

export interface PushNotifConfig {
  enabled?: boolean
  server?: string
  priority?: string
  tags?: string[]
  debug?: boolean
}

export interface NotificationsConfig {
  desktop?: DesktopNotifConfig
  push?: PushNotifConfig
}

export interface ProjectConfigSettings {
  providers?: ProviderEntry[]
  notifications?: NotificationsConfig
}

export interface ToolConfig {
  allowed_write_paths?: string[]
  allowed_read_paths?: string[]
  denied_write_paths?: string[]
}

export interface ToolModuleEntry {
  module: string
  config?: ToolConfig
}

export interface ModulesSettings {
  tools?: ToolModuleEntry[]
}

export interface OverrideEntry {
  source?: string
  config?: Record<string, unknown>
}

export interface SourcesSettings {
  modules?: Record<string, string>
  bundles?: Record<string, string>
}

export interface RoutingSettings {
  matrix?: string
  overrides?: Record<string, string>
}

export interface ProjectSettings {
  bundle?: BundleSettings
  config?: ProjectConfigSettings
  modules?: ModulesSettings
  overrides?: Record<string, OverrideEntry>
  sources?: SourcesSettings
  routing?: RoutingSettings
}

export async function getProjectSettings(projectId: string): Promise<ProjectSettings> {
  const res = await fetch(`/api/projects/${projectId}/settings`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function updateProjectSettings(
  projectId: string,
  settings: ProjectSettings,
): Promise<ProjectSettings> {
  const res = await fetch(`/api/projects/${projectId}/settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd /Users/ken/workspace/ms/loom/ui
npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add ui/src/api/projects.ts
git commit -m "feat: add ProjectSettings TypeScript types and API client"
```

---

## Task 4: ProjectSettingsPanel component

**Files:**
- Create: `ui/src/views/projects/ProjectSettingsPanel.tsx`

- [ ] **Step 1: Create the component**

```tsx
// ui/src/views/projects/ProjectSettingsPanel.tsx
import { useEffect, useRef, useState } from 'react'
import {
  type AppBundle,
  listBundles,
} from '../../api/bundles'
import {
  type ProjectSettings,
  getProjectSettings,
  updateProjectSettings,
} from '../../api/projects'

interface Props {
  projectId: string
}

// ── Collapsible section wrapper ───────────────────────────────────────────────

function Section({
  title,
  summary,
  children,
  defaultOpen = false,
}: {
  title: string
  summary?: string
  children: React.ReactNode
  defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div style={{ borderBottom: '1px solid var(--border)' }}>
      <button
        onClick={() => setOpen((v) => !v)}
        style={{
          display: 'flex',
          alignItems: 'center',
          width: '100%',
          padding: '8px 12px',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          gap: 6,
        }}
      >
        <span
          style={{
            fontSize: 9,
            fontWeight: 700,
            letterSpacing: '0.08em',
            color: 'var(--text-very-muted)',
            textTransform: 'uppercase',
            flex: 1,
            textAlign: 'left',
          }}
        >
          {title}
        </span>
        {summary && !open && (
          <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{summary}</span>
        )}
        <span style={{ fontSize: 10, color: 'var(--text-very-muted)', marginLeft: 4 }}>
          {open ? '▼' : '▶'}
        </span>
      </button>
      {open && (
        <div style={{ padding: '0 12px 12px 12px' }}>{children}</div>
      )}
    </div>
  )
}

// ── Pill toggle (matches the Bundles tab toggle) ──────────────────────────────

function Toggle({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      onClick={() => onChange(!value)}
      style={{
        width: 28,
        height: 16,
        borderRadius: 9999,
        background: value ? '#4CAF74' : '#E8E0D4',
        border: 'none',
        cursor: 'pointer',
        position: 'relative',
        flexShrink: 0,
        transition: 'background 150ms',
      }}
    >
      <div
        style={{
          position: 'absolute',
          top: 2,
          width: 12,
          height: 12,
          borderRadius: '50%',
          background: '#fff',
          transition: 'left 150ms',
          left: value ? 14 : 2,
        }}
      />
    </button>
  )
}

// ── Field label ───────────────────────────────────────────────────────────────

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        fontSize: 10,
        fontWeight: 600,
        letterSpacing: '0.06em',
        textTransform: 'uppercase',
        color: 'var(--text-very-muted)',
        marginBottom: 4,
        marginTop: 8,
      }}
    >
      {children}
    </div>
  )
}

// ── Input styling ─────────────────────────────────────────────────────────────

const inputStyle: React.CSSProperties = {
  width: '100%',
  boxSizing: 'border-box',
  padding: '4px 8px',
  fontSize: 12,
  background: 'var(--bg-input)',
  border: '1px solid var(--border)',
  borderRadius: 2,
  color: 'var(--text-primary)',
  fontFamily: 'monospace',
  outline: 'none',
}

// ── Bundle Section ────────────────────────────────────────────────────────────
//
// SCOPE SEMANTICS (important):
//   bundle.app is a LIST that REPLACES across scopes — it does NOT merge.
//   - No project override → project inherits the global bundle.app list entirely.
//   - Project override present → project's bundle.app completely replaces global.
//
// UI design:
//   - Every row shows two state indicators:
//       [G] = amber dot if globally enabled (AppBundle.enabled === true)
//       [P] = project toggle (green if in project list, sand if not)
//   - If no project override (settings.bundle.app === undefined):
//       • All project toggles are disabled (read-only, showing global state)
//       • Banner: "Inheriting global selections — click any toggle to create project override"
//   - If project override present (settings.bundle.app is an array):
//       • All project toggles are active
//       • Banner: "Project override active (replaces global)" + "Reset to global" button

function BundleSection({
  settings,
  appBundles,
  onChange,
}: {
  settings: ProjectSettings
  appBundles: AppBundle[]
  onChange: (s: ProjectSettings) => void
}) {
  const bundle = settings.bundle ?? {}
  // undefined = no project override (inheriting global); string[] = project override active
  const projectAppSpecs: string[] | undefined = bundle.app

  function setActive(active: string) {
    onChange({ ...settings, bundle: { ...bundle, active: active || undefined } })
  }

  // Called when user clicks a project toggle.
  // If no project override exists yet, clone the global list first, then apply the toggle.
  function toggleProjectBundle(installSpec: string) {
    const baseline = projectAppSpecs
      ?? appBundles.filter((b) => b.enabled).map((b) => b.installSpec)
    const isCurrentlyOn = baseline.includes(installSpec)
    const next = isCurrentlyOn
      ? baseline.filter((s) => s !== installSpec)
      : [...baseline, installSpec]
    onChange({ ...settings, bundle: { ...bundle, app: next } })
  }

  function resetToGlobal() {
    const { app: _removed, ...rest } = bundle
    onChange({ ...settings, bundle: Object.keys(rest).length ? rest : undefined })
  }

  const hasProjectOverride = projectAppSpecs !== undefined

  return (
    <Section title="Bundle" defaultOpen>
      <FieldLabel>Active bundle</FieldLabel>
      <select
        value={bundle.active ?? ''}
        onChange={(e) => setActive(e.target.value)}
        style={{ ...inputStyle, fontFamily: 'monospace' }}
      >
        <option value="">(default — foundation)</option>
        {appBundles.map((b) => (
          <option key={b.id} value={b.name}>
            {b.name}
          </option>
        ))}
      </select>
      {bundle.active && (
        <div style={{ fontSize: 10, color: 'var(--text-very-muted)', marginTop: 4, fontFamily: 'monospace', wordBreak: 'break-all' }}>
          {appBundles.find((b) => b.name === bundle.active)?.installSpec ?? bundle.active}
        </div>
      )}

      <FieldLabel>App bundles</FieldLabel>

      {/* Scope banner */}
      <div
        style={{
          fontSize: 10,
          color: hasProjectOverride ? 'var(--text-primary)' : 'var(--text-muted)',
          background: hasProjectOverride ? 'rgba(245,158,11,0.08)' : 'transparent',
          border: hasProjectOverride ? '1px solid rgba(245,158,11,0.25)' : '1px solid var(--border)',
          borderRadius: 2,
          padding: '4px 8px',
          marginBottom: 8,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 8,
        }}
      >
        <span>
          {hasProjectOverride
            ? '⚡ Project override active — replaces global list'
            : '↳ Inheriting global selections — toggle to override'}
        </span>
        {hasProjectOverride && (
          <button
            onClick={resetToGlobal}
            style={{
              fontSize: 10,
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              color: 'var(--text-muted)',
              padding: 0,
              textDecoration: 'underline',
            }}
          >
            Reset to global
          </button>
        )}
      </div>

      {appBundles.length === 0 && (
        <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>No app bundles installed.</div>
      )}

      {/* Column header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '2px 0 4px', borderBottom: '1px solid var(--border)' }}>
        <span style={{ fontSize: 9, color: 'var(--text-very-muted)', width: 28, textAlign: 'center', letterSpacing: '0.06em' }}>GLOBAL</span>
        <span style={{ fontSize: 9, color: 'var(--text-very-muted)', width: 28, textAlign: 'center', letterSpacing: '0.06em' }}>PROJECT</span>
        <span style={{ fontSize: 9, color: 'var(--text-very-muted)', letterSpacing: '0.06em' }}>BUNDLE</span>
      </div>

      {appBundles.map((b) => {
        const projectOn = projectAppSpecs !== undefined
          ? projectAppSpecs.includes(b.installSpec)
          : b.enabled // when no override, mirrors global state
        return (
          <div
            key={b.id}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              padding: '4px 0',
            }}
          >
            {/* Global indicator — read-only amber/sand dot */}
            <div
              title={b.enabled ? 'Globally enabled' : 'Not in global list'}
              style={{
                width: 28,
                display: 'flex',
                justifyContent: 'center',
              }}
            >
              <div
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: '50%',
                  background: b.enabled ? '#F59E0B' : '#E8E0D4',
                  border: b.enabled ? 'none' : '1px solid #D0C8BC',
                }}
              />
            </div>

            {/* Project toggle — disabled (read-only) when no override exists */}
            <div style={{ width: 28, display: 'flex', justifyContent: 'center', opacity: hasProjectOverride ? 1 : 0.5 }}>
              <Toggle
                value={projectOn}
                onChange={() => toggleProjectBundle(b.installSpec)}
              />
            </div>

            <span
              style={{
                fontSize: 12,
                fontFamily: 'monospace',
                color: 'var(--text-primary)',
                flex: 1,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {b.name}
            </span>
          </div>
        )
      })}
    </Section>
  )
}

// ── Providers Section ─────────────────────────────────────────────────────────

function ProvidersSection({
  settings,
  onChange,
}: {
  settings: ProjectSettings
  onChange: (s: ProjectSettings) => void
}) {
  const providers = settings.config?.providers ?? []

  function updateProvider(index: number, key: string, value: string) {
    const next = providers.map((p, i) => {
      if (i !== index) return p
      const config = { ...(p.config ?? {}) }
      if (value) config[key] = value
      else delete config[key]
      return { ...p, config }
    })
    onChange({ ...settings, config: { ...(settings.config ?? {}), providers: next } })
  }

  const summary = providers.length > 0 ? `${providers.length} configured` : 'none'

  return (
    <Section title="Providers" summary={summary}>
      {providers.length === 0 && (
        <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>
          No providers configured at project scope. Project inherits global provider settings.
        </div>
      )}
      {providers.map((p, i) => (
        <div key={i} style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 12, fontFamily: 'monospace', color: 'var(--text-primary)', fontWeight: 600, marginBottom: 4 }}>
            {p.module}
          </div>
          <FieldLabel>Default model</FieldLabel>
          <input
            style={inputStyle}
            value={(p.config?.['default_model'] as string) ?? ''}
            onChange={(e) => updateProvider(i, 'default_model', e.target.value)}
            placeholder="e.g. claude-sonnet-4-6"
          />
          <FieldLabel>API key override</FieldLabel>
          <input
            style={inputStyle}
            type="password"
            value={(p.config?.['api_key'] as string) ?? ''}
            onChange={(e) => updateProvider(i, 'api_key', e.target.value)}
            placeholder="${ENV_VAR} or literal key"
          />
        </div>
      ))}
    </Section>
  )
}

// ── Routing Section ───────────────────────────────────────────────────────────

function RoutingSection({
  settings,
  onChange,
}: {
  settings: ProjectSettings
  onChange: (s: ProjectSettings) => void
}) {
  const routing = settings.routing ?? {}
  const MATRICES = ['balanced', 'fast', 'quality', 'economy']

  return (
    <Section title="Routing" summary={routing.matrix ?? 'default'}>
      <FieldLabel>Matrix</FieldLabel>
      <select
        value={routing.matrix ?? ''}
        onChange={(e) =>
          onChange({ ...settings, routing: { ...routing, matrix: e.target.value || undefined } })
        }
        style={inputStyle}
      >
        <option value="">(inherit global)</option>
        {MATRICES.map((m) => (
          <option key={m} value={m}>
            {m}
          </option>
        ))}
      </select>
    </Section>
  )
}

// ── Filesystem Section ────────────────────────────────────────────────────────

function FilesystemSection({
  settings,
  onChange,
}: {
  settings: ProjectSettings
  onChange: (s: ProjectSettings) => void
}) {
  const fsTool = settings.modules?.tools?.find((t) => t.module === 'tool-filesystem')
  const cfg = fsTool?.config ?? {}
  const writePaths = cfg.allowed_write_paths ?? []
  const readPaths = cfg.allowed_read_paths ?? []
  const deniedPaths = cfg.denied_write_paths ?? []
  const totalPaths = writePaths.length + readPaths.length + deniedPaths.length
  const summary = totalPaths > 0 ? `${totalPaths} path${totalPaths !== 1 ? 's' : ''}` : 'default'

  function updateFsPaths(
    field: 'allowed_write_paths' | 'allowed_read_paths' | 'denied_write_paths',
    raw: string,
  ) {
    const paths = raw.split('\n').map((s) => s.trim()).filter(Boolean)
    const newCfg = { ...cfg, [field]: paths.length ? paths : undefined }
    const tools = (settings.modules?.tools ?? []).filter((t) => t.module !== 'tool-filesystem')
    tools.push({ module: 'tool-filesystem', config: newCfg })
    onChange({ ...settings, modules: { ...(settings.modules ?? {}), tools } })
  }

  return (
    <Section title="Filesystem" summary={summary}>
      <FieldLabel>Allowed write paths</FieldLabel>
      <textarea
        style={{ ...inputStyle, height: 60, resize: 'vertical' }}
        value={writePaths.join('\n')}
        onChange={(e) => updateFsPaths('allowed_write_paths', e.target.value)}
        placeholder="One path per line"
      />
      <FieldLabel>Allowed read paths</FieldLabel>
      <textarea
        style={{ ...inputStyle, height: 60, resize: 'vertical' }}
        value={readPaths.join('\n')}
        onChange={(e) => updateFsPaths('allowed_read_paths', e.target.value)}
        placeholder="One path per line"
      />
      <FieldLabel>Denied write paths</FieldLabel>
      <textarea
        style={{ ...inputStyle, height: 60, resize: 'vertical' }}
        value={deniedPaths.join('\n')}
        onChange={(e) => updateFsPaths('denied_write_paths', e.target.value)}
        placeholder="One path per line"
      />
    </Section>
  )
}

// ── Notifications Section ─────────────────────────────────────────────────────

function NotificationsSection({
  settings,
  onChange,
}: {
  settings: ProjectSettings
  onChange: (s: ProjectSettings) => void
}) {
  const desktop = settings.config?.notifications?.desktop ?? {}
  const enabled = desktop.enabled !== false // default true

  function setEnabled(v: boolean) {
    onChange({
      ...settings,
      config: {
        ...(settings.config ?? {}),
        notifications: {
          ...(settings.config?.notifications ?? {}),
          desktop: { ...desktop, enabled: v },
        },
      },
    })
  }

  function setField(field: keyof typeof desktop, value: unknown) {
    onChange({
      ...settings,
      config: {
        ...(settings.config ?? {}),
        notifications: {
          ...(settings.config?.notifications ?? {}),
          desktop: { ...desktop, [field]: value || undefined },
        },
      },
    })
  }

  const summary = enabled ? 'on' : 'off'

  return (
    <Section title="Notifications" summary={summary}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <Toggle value={enabled} onChange={setEnabled} />
        <span style={{ fontSize: 12, color: 'var(--text-primary)' }}>Desktop notifications</span>
      </div>
      {enabled && (
        <>
          <FieldLabel>Min iterations before notifying</FieldLabel>
          <input
            style={inputStyle}
            type="number"
            min={0}
            value={desktop.min_iterations ?? ''}
            onChange={(e) => setField('min_iterations', e.target.value ? Number(e.target.value) : undefined)}
            placeholder="(inherit)"
          />
          <FieldLabel>Sound</FieldLabel>
          <input
            style={inputStyle}
            value={desktop.sound ?? ''}
            onChange={(e) => setField('sound', e.target.value)}
            placeholder="e.g. Glass, Ping (macOS)"
          />
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 8 }}>
            <Toggle
              value={desktop.suppress_if_focused ?? false}
              onChange={(v) => setField('suppress_if_focused', v)}
            />
            <span style={{ fontSize: 12, color: 'var(--text-primary)' }}>Suppress when app is focused</span>
          </div>
        </>
      )}
    </Section>
  )
}

// ── Overrides Section ─────────────────────────────────────────────────────────

function OverridesSection({
  settings,
  onChange,
}: {
  settings: ProjectSettings
  onChange: (s: ProjectSettings) => void
}) {
  const overrides = settings.overrides ?? {}
  const count = Object.keys(overrides).length
  const summary = count > 0 ? `${count} override${count !== 1 ? 's' : ''}` : 'none'

  // Raw YAML editing — display as JSON for now; a future task can add YAML support
  const [raw, setRaw] = useState(() => JSON.stringify(overrides, null, 2))
  const [parseError, setParseError] = useState<string | null>(null)

  function applyRaw() {
    try {
      const parsed = JSON.parse(raw)
      setParseError(null)
      onChange({ ...settings, overrides: parsed })
    } catch (e) {
      setParseError(String(e))
    }
  }

  return (
    <Section title="Overrides" summary={summary}>
      <div style={{ fontSize: 10, color: 'var(--text-very-muted)', marginBottom: 6 }}>
        Per-module source and config overrides. Edit as JSON.
      </div>
      <textarea
        style={{ ...inputStyle, height: 120, resize: 'vertical', fontSize: 11 }}
        value={raw}
        onChange={(e) => setRaw(e.target.value)}
        onBlur={applyRaw}
        spellCheck={false}
      />
      {parseError && (
        <div style={{ fontSize: 10, color: '#e57373', marginTop: 4 }}>{parseError}</div>
      )}
    </Section>
  )
}

// ── Sources Section ───────────────────────────────────────────────────────────

function SourcesSection({
  settings,
  onChange,
}: {
  settings: ProjectSettings
  onChange: (s: ProjectSettings) => void
}) {
  const sources = settings.sources ?? {}
  const modCount = Object.keys(sources.modules ?? {}).length
  const summary = modCount > 0 ? `${modCount} module override${modCount !== 1 ? 's' : ''}` : 'none'

  const [raw, setRaw] = useState(() => JSON.stringify(sources, null, 2))
  const [parseError, setParseError] = useState<string | null>(null)

  function applyRaw() {
    try {
      const parsed = JSON.parse(raw)
      setParseError(null)
      onChange({ ...settings, sources: parsed })
    } catch (e) {
      setParseError(String(e))
    }
  }

  return (
    <Section title="Sources" summary={summary}>
      <div style={{ fontSize: 10, color: 'var(--text-very-muted)', marginBottom: 6 }}>
        Point modules at local checkouts for dev. Edit as JSON.
      </div>
      <textarea
        style={{ ...inputStyle, height: 80, resize: 'vertical', fontSize: 11 }}
        value={raw}
        onChange={(e) => setRaw(e.target.value)}
        onBlur={applyRaw}
        spellCheck={false}
      />
      {parseError && (
        <div style={{ fontSize: 10, color: '#e57373', marginTop: 4 }}>{parseError}</div>
      )}
    </Section>
  )
}

// ── Root panel ────────────────────────────────────────────────────────────────

export function ProjectSettingsPanel({ projectId }: Props) {
  const [settings, setSettings] = useState<ProjectSettings>({})
  const [appBundles, setAppBundles] = useState<AppBundle[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    setLoading(true)
    Promise.all([getProjectSettings(projectId), listBundles()])
      .then(([s, b]) => {
        setSettings(s)
        setAppBundles(b)
        setLoading(false)
      })
      .catch((e) => {
        setError(String(e))
        setLoading(false)
      })
  }, [projectId])

  // Debounced auto-save: 800 ms after last change
  function handleChange(next: ProjectSettings) {
    setSettings(next)
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      setSaving(true)
      updateProjectSettings(projectId, next)
        .then(() => setSaving(false))
        .catch((e) => {
          setError(String(e))
          setSaving(false)
        })
    }, 800)
  }

  if (loading) {
    return (
      <div style={{ padding: 16, fontSize: 12, color: 'var(--text-muted)' }}>
        Loading settings…
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: 16, fontSize: 12, color: '#e57373' }}>
        {error}
      </div>
    )
  }

  return (
    <div
      style={{
        overflowY: 'auto',
        height: '100%',
        fontSize: 12,
        color: 'var(--text-primary)',
        background: 'var(--bg-panel)',
      }}
    >
      {saving && (
        <div
          style={{
            position: 'sticky',
            top: 0,
            background: 'var(--bg-input)',
            borderBottom: '1px solid var(--border)',
            padding: '4px 12px',
            fontSize: 10,
            color: 'var(--text-very-muted)',
          }}
        >
          Saving…
        </div>
      )}
      <BundleSection settings={settings} appBundles={appBundles} onChange={handleChange} />
      <ProvidersSection settings={settings} onChange={handleChange} />
      <RoutingSection settings={settings} onChange={handleChange} />
      <FilesystemSection settings={settings} onChange={handleChange} />
      <NotificationsSection settings={settings} onChange={handleChange} />
      <OverridesSection settings={settings} onChange={handleChange} />
      <SourcesSection settings={settings} onChange={handleChange} />
    </div>
  )
}
```

- [ ] **Step 2: Check TypeScript**

```bash
cd /Users/ken/workspace/ms/loom/ui
npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add ui/src/views/projects/ProjectSettingsPanel.tsx
git commit -m "feat: add ProjectSettingsPanel component with all settings sections"
```

---

## Task 5: Wire up Settings tab in WorkspaceApp

**Files:**
- Modify: `ui/src/views/projects/WorkspaceApp.tsx`

- [ ] **Step 1: Find the right-panel tab type and extend it**

Search for `type RightTab` or the string `'files'` union in `WorkspaceApp.tsx`. It will look similar to:

```ts
const [rightTab, setRightTab] = useState<'files' | 'stats'>('files')
```

Change it to:

```ts
const [rightTab, setRightTab] = useState<'files' | 'stats' | 'settings'>('files')
```

- [ ] **Step 2: Add the Settings tab button**

Find the right-panel tab bar rendering block. It renders buttons for FILES and STATS with the amber underline pattern. Add a third button immediately after the STATS button:

```tsx
<button
  role="tab"
  aria-selected={rightTab === 'settings'}
  onClick={() => setRightTab('settings')}
  style={{
    background: 'none',
    border: 'none',
    borderBottom: rightTab === 'settings' ? '2px solid var(--amber)' : '2px solid transparent',
    color: rightTab === 'settings' ? 'var(--text-primary)' : 'var(--text-muted)',
    cursor: 'pointer',
    fontSize: 11,
    fontWeight: 600,
    letterSpacing: '0.06em',
    padding: '0 8px',
    height: '100%',
  }}
>
  Settings
</button>
```

- [ ] **Step 3: Add the import**

At the top of `WorkspaceApp.tsx`, add:

```ts
import { ProjectSettingsPanel } from './ProjectSettingsPanel'
```

- [ ] **Step 4: Render the panel when Settings tab is active**

Find the right-panel content area. It will have conditional rendering like:

```tsx
{rightTab === 'files' && <FileViewer ... />}
{rightTab === 'stats' && <SessionStatsPanel ... />}
```

Add immediately after:

```tsx
{rightTab === 'settings' && selectedProject && (
  <ProjectSettingsPanel projectId={selectedProject.id} />
)}
```

- [ ] **Step 5: Verify the build**

```bash
cd /Users/ken/workspace/ms/loom/ui
npx tsc --noEmit
npm run build 2>&1 | tail -20
```

Expected: clean TypeScript, successful Vite build

- [ ] **Step 6: Manual smoke test**

```bash
cd /Users/ken/workspace/ms/loom
go run ./cmd/loom start
# Open http://localhost:7700 in browser
# 1. Click Projects tab
# 2. Select any project
# 3. Confirm "Settings" tab appears in the right panel
# 4. Click Settings tab
# 5. Confirm Bundle section is expanded with Active bundle dropdown and App bundles toggles
# 6. Change Active bundle to any value — confirm "Saving…" flash and no error
# 7. Verify <project>/.amplifier/settings.yaml was written:
#    cat <project-path>/.amplifier/settings.yaml
```

- [ ] **Step 7: Commit**

```bash
git add ui/src/views/projects/WorkspaceApp.tsx
git commit -m "feat: add Settings tab to Projects right panel"
```

---

## Self-Review

**Spec coverage:**
- ✅ Settings tab in right panel alongside Files/Stats
- ✅ Bundle section: active dropdown from `listBundles()`, app bundle toggles
- ✅ Providers section: per-provider model + API key
- ✅ Routing section: matrix dropdown
- ✅ Filesystem section: allowed/denied paths text areas
- ✅ Notifications section: enabled toggle + key fields
- ✅ Overrides section: raw JSON editor
- ✅ Sources section: raw JSON editor (dev workflow)
- ✅ Auto-save on change (800 ms debounce)
- ✅ Backend reads/writes `<project>/.amplifier/settings.yaml`

**Known gaps deferred to follow-up:**
- Providers section only edits existing providers — adding a new provider requires a future "Add provider" button
- Overrides/Sources use raw JSON instead of structured UI (acceptable for advanced fields)
- No YAML scope indicator (project vs global merge) — deferred
- API keys ideally write to `settings.local.yaml` — deferred (writes to `settings.yaml` for now)
