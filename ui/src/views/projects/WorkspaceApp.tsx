import { useEffect, useState } from 'react'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import {
  Project, Session,
  listProjects, createProject, deleteProject,
  listSessions, createSession, deleteSession, spawnTerminal,
  pickFolder, canPickFolder,
} from '../../api/projects'
import FileViewer from './FileViewer'
import SessionStatsPanel from './SessionStats'
import { TerminalPanel } from './terminal/TerminalPanel'

// ── Main workspace ────────────────────────────────────────────────────────────

export default function WorkspaceApp() {
  const [projects, setProjects] = useState<Project[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [activeProject, setActiveProject] = useState<Project | null>(null)
  const [activeSession, setActiveSession] = useState<Session | null>(null)
  // Grove pattern: keep ALL seen processIds in DOM, show active via visibility:hidden
  const [liveProcessIds, setLiveProcessIds] = useState<Set<string>>(new Set())
  const [activeProcessId, setActiveProcessId] = useState<string | null>(null)
  const [rightPanel, setRightPanel] = useState<'files' | 'stats' | null>(null)

  // New Project modal
  const [showNewProject, setShowNewProject] = useState(false)
  const [newProjectPath, setNewProjectPath] = useState('')
  const [canBrowse, setCanBrowse] = useState(false)
  useEffect(() => { canPickFolder().then(setCanBrowse).catch(() => setCanBrowse(false)) }, [])

  const [creatingSession, setCreatingSession] = useState(false)

  useEffect(() => {
    listProjects()
      .then(ps => {
        setProjects(ps)
        if (ps.length > 0) selectProject(ps[0])
      })
      .catch(console.error)
  }, [])

  // Poll session names every 5 s — picks up amplifier's auto-naming hook
  useEffect(() => {
    if (!activeProject) return
    const id = setInterval(async () => {
      const ss = await listSessions(activeProject.id).catch(() => null)
      if (!ss) return
      setSessions(ss)
      setActiveSession(prev => {
        if (!prev) return prev
        const updated = ss.find(s => s.id === prev.id)
        return updated && updated.name !== prev.name ? updated : prev
      })
    }, 5_000)
    return () => clearInterval(id)
  }, [activeProject?.id])

  async function selectProject(p: Project) {
    setActiveProject(p)
    setActiveSession(null)
    setActiveProcessId(null)
    const ss = await listSessions(p.id).catch(() => [] as Session[])
    setSessions(ss)
    if (ss.length > 0) selectSession(p, ss[0])
  }

  async function selectSession(p: Project, s: Session) {
    setActiveSession(s)
    try {
      const { processId: pid } = await spawnTerminal(p.id, s.id)
      setActiveProcessId(pid)
      // Register processId as live — TerminalPanel keeps it in DOM with visibility:hidden
      setLiveProcessIds(prev => {
        if (prev.has(pid)) return prev
        const next = new Set(prev)
        next.add(pid)
        return next
      })
    } catch (e) {
      console.error('spawnTerminal:', e)
    }
  }

  async function handleDeleteProject(id: string) {
    try {
      await deleteProject(id)
      setProjects(ps => ps.filter(p => p.id !== id))
      if (activeProject?.id === id) {
        setActiveProject(null)
        setActiveSession(null)
        setActiveProcessId(null)
        setLiveProcessIds(new Set())
        setRightPanel(null)
      }
    } catch (e) { console.error('deleteProject:', e) }
  }

  async function handleDeleteSession(projectId: string, sessionId: string) {
    try {
      await deleteSession(projectId, sessionId)
      setSessions(ss => ss.filter(s => s.id !== sessionId))
      if (activeSession?.id === sessionId) {
        setActiveSession(null)
        setActiveProcessId(null)
        // Clean up the terminal buffer from the registry
        const reg = (window as any).__terminalRegistry
        if (reg) {
          // The processId for this session is in liveProcessIds — find and clean it up
          reg.buffers.delete(activeProcessId)
          reg.terminals.delete(activeProcessId)
        }
        setRightPanel(null)
      }
    } catch (e) { console.error('deleteSession:', e) }
  }

  async function handleBrowse() {
    try {
      const result = await pickFolder()
      if (result.path) setNewProjectPath(result.path)
    } catch (e) {
      console.error('browse:', e)
    }
  }

  async function handleCreateProject() {
    if (!newProjectPath) return
    const name = newProjectPath.split('/').filter(Boolean).pop() || newProjectPath
    try {
      const p = await createProject(name, newProjectPath)
      setProjects(ps => [...ps, p])
      setShowNewProject(false)
      setNewProjectPath('')
      // Auto-create first session — name derived from git branch on the backend
      await createSession(p.id, '').catch(() => {})
      selectProject(p)
    } catch (e) {
      console.error('createProject:', e)
    }
  }

  async function handleCreateSession() {
    if (!activeProject || creatingSession) return
    setCreatingSession(true)
    try {
      const s = await createSession(activeProject.id, '')
      setSessions(ss => [...ss, s])
      selectSession(activeProject, s)
    } catch (e) {
      console.error('createSession:', e)
    } finally {
      setCreatingSession(false)
    }
  }

  return (
    <div className="flex h-full bg-[#0d1117]">
      {/* Left sidebar */}
      <div className="w-56 border-r border-[#30363d] flex flex-col shrink-0">
        {/* Project list */}
        <div className="flex items-center justify-between px-3 py-2 border-b border-[#30363d]">
          <span className="text-[#8b949e] text-[10px] uppercase tracking-wider">Projects</span>
          <button
            onClick={() => setShowNewProject(true)}
            className="text-[#58a6ff] text-xs hover:text-[#e6edf3]"
            aria-label="New project"
          >+</button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {projects.map(p => (
            <div
              key={p.id}
              className={[
                'group flex items-center border-b border-[#21262d] transition-colors',
                activeProject?.id === p.id ? 'bg-[#21262d]' : 'hover:bg-[#161b22]',
              ].join(' ')}
            >
              <button
                onClick={() => selectProject(p)}
                className="flex-1 text-left px-3 py-2 min-w-0"
              >
                <div className={`text-xs truncate ${activeProject?.id === p.id ? 'text-[#e6edf3]' : 'text-[#8b949e]'}`}>
                  {p.name}
                </div>
              </button>
              <button
                onClick={() => handleDeleteProject(p.id)}
                className="opacity-0 group-hover:opacity-100 px-2 py-2 text-[#484f58] hover:text-[#f85149] text-xs shrink-0"
                title="Delete project"
              >×</button>
            </div>
          ))}
        </div>

        {/* Session list for active project */}
        {activeProject && (
          <>
            <div className="flex items-center justify-between px-3 py-2 border-t border-b border-[#30363d]">
              <span className="text-[#8b949e] text-[10px] uppercase tracking-wider">Sessions</span>
              <button
                onClick={handleCreateSession}
                disabled={creatingSession}
                className="text-[#58a6ff] text-xs hover:text-[#e6edf3] disabled:opacity-40"
                aria-label="New session"
              >{creatingSession ? '…' : '+'}</button>
            </div>
            {sessions.length === 0 && (
              <div className="px-3 py-2 text-[10px] text-[#484f58]">No sessions yet</div>
            )}
            {sessions.map(s => (
              <div
                key={s.id}
                className={[
                  'group flex items-center border-b border-[#21262d] transition-colors',
                  activeSession?.id === s.id ? 'bg-[#21262d]' : 'hover:bg-[#161b22]',
                ].join(' ')}
              >
                <button
                  onClick={() => selectSession(activeProject, s)}
                  className="flex-1 text-left px-3 py-1.5 min-w-0"
                >
                  <div className={`text-[11px] truncate ${activeSession?.id === s.id ? 'text-[#e6edf3]' : 'text-[#8b949e]'}`}>
                    {s.name}
                  </div>
                  <div className="text-[10px] text-[#484f58]">{s.status}</div>
                </button>
                <button
                  onClick={() => handleDeleteSession(activeProject.id, s.id)}
                  className="opacity-0 group-hover:opacity-100 px-2 py-1.5 text-[#484f58] hover:text-[#f85149] text-xs shrink-0"
                  title="Close session"
                >×</button>
              </div>
            ))}
          </>
        )}
      </div>

      {/* Main area — resizable split between terminal and right panel */}
      <PanelGroup direction="horizontal" className="flex-1 overflow-hidden">
        <Panel minSize={20} className="flex flex-col overflow-hidden">
          {/* Session header — only shown when a session is active */}
          {activeProject && activeSession && (
            <div className="flex items-center gap-2 px-3 py-1.5 bg-[#161b22] border-b border-[#30363d] shrink-0">
              <span className="text-xs text-[#e6edf3] font-medium">{activeProject.name}</span>
              <span className="text-xs text-[#8b949e]">/ {activeSession.name}</span>
              <div className="ml-auto flex gap-1">
                {(['files', 'stats'] as const).map(panel => (
                  <button
                    key={panel}
                    onClick={() => setRightPanel(rightPanel === panel ? null : panel)}
                    className={[
                      'text-[10px] px-2 py-0.5 rounded capitalize',
                      rightPanel === panel
                        ? 'bg-[#388bfd]/20 text-[#58a6ff]'
                        : 'bg-[#21262d] text-[#8b949e] hover:text-[#e6edf3]',
                    ].join(' ')}
                  >
                    {panel}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Content area — terminal container is ALWAYS in DOM so instances persist */}
          <div className="flex-1 overflow-hidden relative">
            {/* Empty state overlay — covers the (invisible) terminal when no session */}
            {(!activeProject || !activeSession) && (
              <div className="absolute inset-0 flex items-center justify-center z-10 bg-[#0d1117]">
                {activeProject ? (
                  <div className="text-center text-[#8b949e]">
                    <div className="text-sm font-medium text-[#e6edf3] mb-1">{activeProject.name}</div>
                    <div className="text-xs mb-3 text-[#484f58]">{activeProject.path}</div>
                    <button
                      onClick={handleCreateSession}
                      disabled={creatingSession}
                      className="text-xs px-3 py-1.5 bg-[#21262d] border border-[#30363d] rounded text-[#e6edf3] hover:bg-[#30363d] disabled:opacity-40"
                    >
                      {creatingSession ? 'Starting…' : '+ New Session'}
                    </button>
                  </div>
                ) : (
                  <div className="text-center text-[#8b949e]">
                    <div className="text-sm mb-2">Select or create a project</div>
                    <button
                      onClick={() => setShowNewProject(true)}
                      className="text-xs px-3 py-1.5 bg-[#21262d] border border-[#30363d] rounded text-[#e6edf3] hover:bg-[#30363d]"
                    >
                      + New Project
                    </button>
                  </div>
                )}
              </div>
            )}

            {/* One TerminalPanel per live processId — grove pattern: visibility:hidden keeps them alive */}
            {Array.from(liveProcessIds).map(pid => (
              <div
                key={pid}
                style={{
                  position: 'absolute',
                  inset: 0,
                  visibility: pid === activeProcessId ? 'visible' : 'hidden',
                  pointerEvents: pid === activeProcessId ? 'auto' : 'none',
                }}
              >
                <TerminalPanel processId={pid} />
              </div>
            ))}
          </div>
        </Panel>

        {rightPanel && activeProject && activeSession && (
          <>
            <PanelResizeHandle className="w-1 bg-[#30363d] hover:bg-[#58a6ff] transition-colors cursor-col-resize" />
            <Panel defaultSize={50} minSize={20} className="flex flex-col overflow-hidden">
              {rightPanel === 'files' && (
                <FileViewer projectId={activeProject.id} sessionId={activeSession.id} />
              )}
              {rightPanel === 'stats' && (
                <SessionStatsPanel project={activeProject} session={activeSession} />
              )}
            </Panel>
          </>
        )}
      </PanelGroup>

      {/* New Project modal */}
      {showNewProject && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-5 w-80">
            <h3 className="text-sm font-semibold text-[#e6edf3] mb-4">Open Project</h3>
            <div className="flex gap-2 mb-4">
              <input
                className="flex-1 px-3 py-1.5 text-sm bg-[#0d1117] border border-[#30363d] rounded text-[#e6edf3] placeholder:text-[#8b949e] focus:outline-none focus:border-[#58a6ff]"
                placeholder="/absolute/path/to/codebase"
                value={newProjectPath}
                autoFocus
                onChange={e => setNewProjectPath(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleCreateProject()}
              />
              {canBrowse && (
                <button
                  onClick={handleBrowse}
                  type="button"
                  className="px-3 py-1.5 text-xs bg-[#21262d] border border-[#30363d] rounded text-[#8b949e] hover:text-[#e6edf3] hover:bg-[#30363d] shrink-0"
                >
                  Browse…
                </button>
              )}
            </div>
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setShowNewProject(false)}
                className="px-3 py-1.5 text-xs text-[#8b949e] hover:text-[#e6edf3]"
              >Cancel</button>
              <button
                onClick={handleCreateProject}
                className="px-3 py-1.5 text-xs bg-[#238636] hover:bg-[#2ea043] text-white rounded"
              >Create</button>
            </div>
          </div>
        </div>
      )}


    </div>
  )
}
