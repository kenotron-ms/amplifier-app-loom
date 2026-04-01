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
