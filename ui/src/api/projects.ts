export interface Project {
  id: string
  name: string
  path: string
  workspace?: string
  createdAt: number
  lastActivityAt: number
}

export interface Session {
  id: string
  projectId: string
  name: string
  worktreePath: string
  processId: string | null
  createdAt: number
  status: 'idle' | 'active' | 'stopped'
  amplifierSessionId?: string  // set after first spawn, used for --resume
}

export interface FileEntry {
  name: string
  isDir: boolean
  size: number
}

export async function listProjects(): Promise<Project[]> {
  const res = await fetch('/api/projects')
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function createProject(name: string, path: string): Promise<Project> {
  const res = await fetch('/api/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function deleteProject(id: string): Promise<void> {
  const res = await fetch(`/api/projects/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await res.text())
}

export async function listSessions(projectId: string): Promise<Session[]> {
  const res = await fetch(`/api/projects/${projectId}/sessions`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function createSession(
  projectId: string,
  name: string,
): Promise<Session> {
  const res = await fetch(`/api/projects/${projectId}/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function spawnTerminal(
  projectId: string,
  sessionId: string,
): Promise<{ processId: string }> {
  const res = await fetch(`/api/projects/${projectId}/sessions/${sessionId}/terminal`, {
    method: 'POST',
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export interface BrowseEntry {
  name: string
  hidden: boolean
}

export interface BrowseResult {
  path: string
  home: string
  parent: string
  entries: BrowseEntry[]
}

/** List directories on the SERVER at the given path (defaults to home dir). */
export async function browseDirs(path?: string): Promise<BrowseResult> {
  const url = path
    ? `/api/filesystem/browse?path=${encodeURIComponent(path)}`
    : '/api/filesystem/browse'
  const res = await fetch(url)
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? 'Failed to browse directory')
  }
  return res.json()
}

/** Given a directory name, find the full absolute path via Spotlight/find. */
export async function findDir(name: string): Promise<string[]> {
  const res = await fetch(`/api/filesystem/find-dir?name=${encodeURIComponent(name)}`)
  if (!res.ok) return []
  const data = await res.json()
  return data.paths ?? []
}

export async function deleteSession(projectId: string, sessionId: string): Promise<void> {
  const res = await fetch(`/api/projects/${projectId}/sessions/${sessionId}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await res.text())
}

export interface SessionStats {
  tokens: number
  tools: number
  turns?: number
  startedAt?: string
  model?: string
}

export async function getSessionStats(projectId: string, sessionId: string): Promise<SessionStats> {
  const res = await fetch(`/api/projects/${projectId}/sessions/${sessionId}/stats`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function readFileContent(
  projectId: string,
  sessionId: string,
  path: string,
): Promise<string> {
  const res = await fetch(
    `/api/projects/${projectId}/sessions/${sessionId}/files/${path}`,
  )
  if (!res.ok) throw new Error(await res.text())
  return res.text()
}

export async function listFiles(
  projectId: string,
  sessionId: string,
  path = '',
): Promise<FileEntry[]> {
  const url = `/api/projects/${projectId}/sessions/${sessionId}/files${path ? `?path=${encodeURIComponent(path)}` : ''}`
  const res = await fetch(url)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

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

// ── Amplifier sessions (Phase 2 — reads from Amplifier's session store) ──────

export interface AmplifierSession {
  id: string
  name: string
  createdAt: string
  lastActiveAt: string
}

export async function getProject(id: string): Promise<Project> {
  const res = await fetch(`/api/projects/${id}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function listAmplifierSessions(projectId: string): Promise<AmplifierSession[]> {
  const res = await fetch(`/api/projects/${projectId}/amplifier-sessions`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function openTerminal(
  projectId: string,
  mode: 'new' | 'resume',
  sessionId?: string,
): Promise<void> {
  const res = await fetch(`/api/projects/${projectId}/open-terminal`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode, sessionId }),
  })
  if (!res.ok) throw new Error(await res.text())
}
