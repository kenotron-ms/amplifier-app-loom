export interface Project {
  id: string
  name: string
  path: string
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

/** Opens the native OS folder picker via the backend (zenity binary — no browser modal). */
export async function pickFolder(): Promise<{ path?: string; cancelled?: boolean }> {
  const res = await fetch('/api/filesystem/pick-folder')
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

/** Returns true if the zenity binary is available on the backend. */
export async function canPickFolder(): Promise<boolean> {
  const res = await fetch('/api/filesystem/pick-folder?check=1')
  if (!res.ok) return false
  const data = await res.json()
  return data.supported === true
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
