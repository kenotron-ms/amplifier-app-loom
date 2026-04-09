import { useEffect, useState } from 'react'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import {
  Project, Session,
  listProjects, createProject, deleteProject,
  listSessions, createSession, deleteSession, spawnTerminal,
} from '../../api/projects'
import FileViewer from './FileViewer'
import SessionStatsPanel from './SessionStats'
import { TerminalPanel } from './terminal/TerminalPanel'
import DirectoryBrowserModal from '../../components/DirectoryBrowserModal'

// ── Status dot ──────────────────────────────────────────────────────────────

function SessionDot({ status }: { status: string }) {
  const isRunning = status === 'running' || status === 'active'
  const isDone = status === 'done' || status === 'completed'

  if (isDone) {
    return (
      <span style={{
        width: 14, height: 14, borderRadius: '50%',
        background: 'var(--green)',
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        flexShrink: 0,
        fontSize: 8, fontWeight: 700, color: '#fff',
      }}>✓</span>
    )
  }
  return (
    <span style={{
      width: 6, height: 6, borderRadius: '50%',
      background: isRunning ? 'var(--amber)' : 'var(--text-very-muted)',
      display: 'inline-block', flexShrink: 0,
    }} />
  )
}

// ── Pane title strip ─────────────────────────────────────────────────────────

function PaneTitle({ children }: { children: React.ReactNode }) {
  return (
    <div style={{
      height: 28,
      display: 'flex', alignItems: 'center',
      padding: '0 14px',
      background: 'var(--bg-pane-title)',
      borderBottom: '1px solid var(--border)',
      fontSize: 10,
      fontFamily: 'var(--font-ui)',
      fontWeight: 600,
      textTransform: 'uppercase',
      letterSpacing: '0.08em',
      color: 'var(--text-very-muted)',
      flexShrink: 0,
    }}>
      {children}
    </div>
  )
}

// ── App preview panel ────────────────────────────────────────────────────────

function AppPreviewPanel() {
  const [url, setUrl] = useState('http://localhost:3000')
  const [inputUrl, setInputUrl] = useState('http://localhost:3000')

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Address bar */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        padding: '4px 8px',
        background: 'var(--bg-pane-title)',
        borderBottom: '1px solid var(--border)',
        flexShrink: 0,
      }}>
        <input
          value={inputUrl}
          onChange={e => setInputUrl(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && setUrl(inputUrl)}
          style={{
            flex: 1,
            fontFamily: 'var(--font-mono)',
            fontSize: 11,
            color: 'var(--text-muted)',
            background: 'var(--bg-input)',
            border: '1px solid var(--border)',
            borderRadius: 3,
            padding: '2px 8px',
            outline: 'none',
          }}
        />
        <button
          onClick={() => setUrl(inputUrl)}
          style={{
            fontSize: 10,
            padding: '2px 8px',
            background: 'var(--bg-pane-title)',
            border: '1px solid var(--border-dark)',
            borderRadius: 3,
            cursor: 'pointer',
            color: 'var(--text-muted)',
          }}
        >Go</button>
      </div>
      <iframe
        src={url}
        style={{ flex: 1, width: '100%', border: 'none' }}
        title="App preview"
      />
    </div>
  )
}

// ── Main workspace ────────────────────────────────────────────────────────────

export default function WorkspaceApp() {
  const [projects, setProjects]       = useState<Project[]>([])
  const [sessions, setSessions]       = useState<Session[]>([])
  const [activeProject, setActiveProject] = useState<Project | null>(null)
  const [activeSession, setActiveSession] = useState<Session | null>(null)
  // Grove pattern: keep ALL seen processIds in DOM, show active via visibility:hidden
  const [liveProcessIds, setLiveProcessIds] = useState<Set<string>>(new Set())
  const [activeProcessId, setActiveProcessId] = useState<string | null>(null)
  const [rightPanel, setRightPanel]   = useState<'files' | 'app' | 'analysis'>('files')
  const [rightHidden, setRightHidden] = useState(false)

  // New Project modal
  const [showNewProject, setShowNewProject] = useState(false)
  const [newProjectPath, setNewProjectPath] = useState('')
  const [showDirBrowser, setShowDirBrowser] = useState(false)
  const [creatingSession, setCreatingSession] = useState(false)

  useEffect(() => {
    listProjects()
      .then(ps => {
        setProjects(ps)
        if (ps.length > 0) selectProject(ps[0])
      })
      .catch(console.error)
  }, [])

  // Poll session names every 5s — picks up amplifier's auto-naming hook.
  // `cancelled` flag discards in-flight responses that arrive after a project
  // switch, preventing stale data from overwriting the current project's sessions.
  useEffect(() => {
    if (!activeProject) return
    const projectId = activeProject.id
    let cancelled = false

    const id = setInterval(async () => {
      const ss = await listSessions(projectId).catch(() => null)
      if (!ss || cancelled) return
      setSessions(ss)
      setActiveSession(prev => {
        if (!prev) return prev
        const updated = ss.find(s => s.id === prev.id)
        // Always sync the full session object, not just on name change — keeps
        // status, amplifierSessionId, and other fields up to date too.
        return updated ?? prev
      })
    }, 5_000)

    return () => {
      cancelled = true
      clearInterval(id)
    }
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
      }
    } catch (e) { console.error('deleteSession:', e) }
  }

  async function handleCreateProject() {
    if (!newProjectPath) return
    const name = newProjectPath.split('/').filter(Boolean).pop() || newProjectPath
    try {
      const p = await createProject(name, newProjectPath)
      setProjects(ps => [...ps, p])
      setShowNewProject(false)
      setNewProjectPath('')
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
    <div style={{ height: '100%', background: 'var(--bg-page)', position: 'relative' }}>
    <PanelGroup direction="horizontal" style={{ height: '100%', overflow: 'hidden' }}>

      {/* ── Left sidebar (resizable) ──────────────────────────────────────── */}
      <Panel defaultSize={16} minSize={10} maxSize={30} style={{
        display: 'flex',
        flexDirection: 'column',
        background: 'var(--bg-sidebar)',
        overflow: 'hidden',
      }}>
        {/* Projects section header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '0 12px',
          height: 32,
          borderBottom: '1px solid var(--border)',
          flexShrink: 0,
        }}>
          <span style={{
            fontSize: 10, fontWeight: 600,
            textTransform: 'uppercase', letterSpacing: '0.08em',
            color: 'var(--text-very-muted)',
          }}>Projects</span>
          <button
            onClick={() => setShowNewProject(true)}
            style={{
              fontSize: 14, lineHeight: 1,
              color: 'var(--text-muted)',
              background: 'none', border: 'none', cursor: 'pointer',
              padding: '0 2px',
            }}
            onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--amber)'}
            onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)'}
            aria-label="New project"
            title="New project"
          >+</button>
        </div>

        {/* Project list */}
        <div style={{ flex: 1, overflowY: 'auto' }} className="canvas-scroll">
          {projects.length === 0 && (
            <div style={{
              padding: '16px 12px',
              fontSize: 11, color: 'var(--text-very-muted)', textAlign: 'center',
            }}>No projects yet</div>
          )}
          {projects.map(p => (
            <div
              key={p.id}
              className="group"
              style={{
                display: 'flex', alignItems: 'center',
                borderBottom: '1px solid var(--border)',
                transition: 'background 0.12s ease',
                background: activeProject?.id === p.id ? 'var(--bg-sidebar-active)' : 'transparent',
              }}
              onMouseEnter={e => {
                if (activeProject?.id !== p.id)
                  (e.currentTarget as HTMLElement).style.background = 'rgba(0,0,0,0.03)'
              }}
              onMouseLeave={e => {
                if (activeProject?.id !== p.id)
                  (e.currentTarget as HTMLElement).style.background = 'transparent'
              }}
            >
              <button
                onClick={() => selectProject(p)}
                style={{
                  flex: 1, textAlign: 'left',
                  padding: '7px 12px 7px 14px',
                  background: 'none', border: 'none', cursor: 'pointer',
                  display: 'flex', alignItems: 'center', gap: 8,
                  borderLeft: activeProject?.id === p.id ? '2px solid var(--amber)' : '2px solid transparent',
                  minWidth: 0,
                }}
              >
                <span style={{
                  fontSize: 12, fontWeight: activeProject?.id === p.id ? 500 : 400,
                  color: activeProject?.id === p.id ? 'var(--text-primary)' : 'var(--text-muted)',
                  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                }}>{p.name}</span>
              </button>
              <button
                onClick={() => handleDeleteProject(p.id)}
                style={{
                  padding: '7px 8px',
                  fontSize: 13, color: 'var(--text-very-muted)',
                  background: 'none', border: 'none', cursor: 'pointer',
                  opacity: 0, flexShrink: 0,
                }}
                className="delete-btn"
                title="Remove project"
                onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--red)'}
                onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'}
              >×</button>
            </div>
          ))}

          {/* Sessions for active project */}
          {activeProject && (
            <div style={{ borderTop: '1px solid var(--border)' }}>
              {/* Sessions section header */}
              <div style={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                padding: '0 12px',
                height: 32,
                borderBottom: sessions.length > 0 ? '1px solid var(--border)' : 'none',
                flexShrink: 0,
              }}>
                <span style={{
                  fontSize: 10, fontWeight: 600,
                  textTransform: 'uppercase', letterSpacing: '0.08em',
                  color: 'var(--text-very-muted)',
                }}>Sessions</span>
                <button
                  onClick={handleCreateSession}
                  disabled={creatingSession}
                  style={{
                    fontSize: 14, lineHeight: 1,
                    color: creatingSession ? 'var(--text-very-muted)' : 'var(--text-muted)',
                    background: 'none', border: 'none', cursor: creatingSession ? 'default' : 'pointer',
                    padding: '0 2px',
                  }}
                  onMouseEnter={e => { if (!creatingSession) (e.currentTarget as HTMLElement).style.color = 'var(--amber)' }}
                  onMouseLeave={e => { if (!creatingSession) (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)' }}
                  aria-label="New session"
                  title="New session"
                >{creatingSession ? '…' : '+'}</button>
              </div>
            </div>
          )}
          {activeProject && sessions.length > 0 && (
            <div>
              {sessions.map(s => (
                <div
                  key={s.id}
                  style={{
                    display: 'flex', alignItems: 'center',
                    borderBottom: '1px solid var(--border)',
                    transition: 'background 0.12s ease',
                    background: activeSession?.id === s.id ? 'var(--bg-sidebar-active)' : 'transparent',
                  }}
                  onMouseEnter={e => {
                    if (activeSession?.id !== s.id)
                      (e.currentTarget as HTMLElement).style.background = 'rgba(0,0,0,0.03)'
                  }}
                  onMouseLeave={e => {
                    if (activeSession?.id !== s.id)
                      (e.currentTarget as HTMLElement).style.background = 'transparent'
                  }}
                >
                  <button
                    onClick={() => selectSession(activeProject, s)}
                    style={{
                      flex: 1, textAlign: 'left',
                      padding: '6px 12px 6px 14px',
                      background: 'none', border: 'none', cursor: 'pointer',
                      display: 'flex', alignItems: 'center', gap: 8,
                      borderLeft: activeSession?.id === s.id ? '2px solid var(--amber)' : '2px solid transparent',
                      minWidth: 0,
                    }}
                  >
                    <SessionDot status={s.status ?? 'active'} />
                    <div style={{ minWidth: 0 }}>
                      <div style={{
                        fontSize: 11.5,
                        fontWeight: activeSession?.id === s.id ? 500 : 400,
                        color: activeSession?.id === s.id ? 'var(--text-primary)' : 'var(--text-muted)',
                        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                      }}>{s.name || 'Session'}</div>
                      {s.status && s.status !== 'active' && (
                        <div style={{ fontSize: 10, color: 'var(--text-very-muted)', marginTop: 1 }}>
                          {s.status}
                        </div>
                      )}
                    </div>
                  </button>
                  <button
                    onClick={() => handleDeleteSession(activeProject.id, s.id)}
                    style={{
                      padding: '6px 8px',
                      fontSize: 13, color: 'var(--text-very-muted)',
                      background: 'none', border: 'none', cursor: 'pointer',
                      opacity: 0.35, flexShrink: 0,
                    }}
                    title="Close session"
                    onMouseEnter={e => {
                      ;(e.currentTarget as HTMLElement).style.color = 'var(--red)'
                      ;(e.currentTarget as HTMLElement).style.opacity = '1'
                    }}
                    onMouseLeave={e => {
                      ;(e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'
                      ;(e.currentTarget as HTMLElement).style.opacity = '0.35'
                    }}
                  >×</button>
                </div>
              ))}
            </div>
          )}


        </div>
      </Panel>

      <PanelResizeHandle style={{ width: 4, background: 'var(--border)', cursor: 'col-resize', flexShrink: 0 }} />

      {/* ── Main area: terminal + right panel ─────────────────────────── */}
      <Panel style={{ overflow: 'hidden' }}>
      <PanelGroup direction="horizontal" style={{ height: '100%' }}>
        <Panel minSize={20} style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>

          {/* Pane title strip */}
          {activeProject && activeSession ? (
            <div style={{
              height: 28,
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '0 14px',
              background: 'var(--bg-pane-title)',
              borderBottom: '1px solid var(--border)',
              flexShrink: 0,
            }}>
              <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <span style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-muted)' }}>
                  {activeProject.name}
                </span>
                <span style={{ fontSize: 10, color: 'var(--text-very-muted)' }}>·</span>
                <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                  {activeSession.name || 'Session'}
                </span>
              </span>
              <button
                onClick={() => setRightHidden(h => !h)}
                style={{
                  fontSize: 10, color: 'var(--text-very-muted)',
                  background: 'none', border: 'none', cursor: 'pointer',
                  letterSpacing: '0.04em',
                }}
                title={rightHidden ? 'Show panel' : 'Hide panel'}
                onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)'}
                onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'}
              >
                {rightHidden ? '⊞' : '⊟'}
              </button>
            </div>
          ) : (
            <PaneTitle>
              {activeProject ? activeProject.name : 'Loom'}
            </PaneTitle>
          )}

          {/* Terminal / empty state */}
          <div style={{ flex: 1, overflow: 'hidden', position: 'relative', background: 'var(--bg-terminal)' }}>
            {/* Empty state */}
            {(!activeProject || !activeSession) && (
              <div style={{
                position: 'absolute', inset: 0,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                zIndex: 10,
                background: 'var(--bg-page)',
              }}>
                {activeProject ? (
                  <div style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: 22, fontWeight: 700, color: 'var(--text-primary)', marginBottom: 6, fontStyle: 'italic', letterSpacing: '-0.02em' }}>
                      {activeProject.name}
                    </div>
                    <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 20 }}>
                      {activeProject.path}
                    </div>
                    <button
                      onClick={handleCreateSession}
                      disabled={creatingSession}
                      style={{
                        fontSize: 13, padding: '7px 16px',
                        background: 'var(--bg-modal)',
                        border: '1px solid var(--border-dark)',
                        borderRadius: 4, cursor: 'pointer',
                        color: 'var(--text-primary)',
                        opacity: creatingSession ? 0.5 : 1,
                      }}
                    >
                      {creatingSession ? 'Starting…' : 'Start session →'}
                    </button>
                  </div>
                ) : (
                  <div style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', marginBottom: 10, fontStyle: 'italic', letterSpacing: '-0.02em' }}>
                      Welcome to Loom
                    </div>
                    <div style={{ fontSize: 14, color: 'var(--text-muted)', marginBottom: 24, maxWidth: 340, lineHeight: 1.5 }}>
                      Amplifier is a powerful engine with no cockpit. Loom is the cockpit.
                    </div>
                    <button
                      onClick={() => setShowNewProject(true)}
                      style={{
                        fontSize: 13, padding: '8px 18px',
                        background: 'var(--bg-modal)',
                        border: '1px solid var(--border-dark)',
                        borderRadius: 4, cursor: 'pointer',
                        color: 'var(--text-primary)',
                      }}
                    >
                      Create your first project →
                    </button>
                  </div>
                )}
              </div>
            )}

            {/* Grove pattern: all live terminals in DOM */}
            {Array.from(liveProcessIds).map(pid => (
              <div
                key={pid}
                style={{
                  position: 'absolute', inset: 0,
                  visibility: pid === activeProcessId ? 'visible' : 'hidden',
                  pointerEvents: pid === activeProcessId ? 'auto' : 'none',
                }}
              >
                <TerminalPanel processId={pid} />
              </div>
            ))}
          </div>
        </Panel>

        {/* ── Right panel ────────────────────────────────────────────── */}
        {activeProject && activeSession && !rightHidden && (
          <>
            <PanelResizeHandle style={{
              width: 1,
              background: 'var(--border)',
              cursor: 'col-resize',
              flexShrink: 0,
            }} />
            <Panel defaultSize={36} minSize={18} style={{
              display: 'flex', flexDirection: 'column', overflow: 'hidden',
              background: 'var(--bg-right)',
            }}>
              {/* Tab bar */}
              <div style={{
                display: 'flex', alignItems: 'center',
                height: 30,
                background: 'var(--bg-right)',
                borderBottom: '1px solid var(--border)',
                flexShrink: 0,
                gap: 0,
              }}>
                {(['files', 'app', 'analysis'] as const).map(tab => (
                  <button
                    key={tab}
                    onClick={() => setRightPanel(tab)}
                    style={{
                      height: '100%',
                      padding: '0 14px',
                      fontSize: 11.5,
                      fontWeight: rightPanel === tab ? 500 : 400,
                      textTransform: 'uppercase',
                      letterSpacing: '0.06em',
                      color: rightPanel === tab ? 'var(--text-primary)' : 'var(--text-very-muted)',
                      background: 'transparent',
                      border: 'none',
                      borderBottom: rightPanel === tab ? '2px solid var(--amber)' : '2px solid transparent',
                      cursor: 'pointer',
                      transition: 'color 0.12s ease',
                    }}
                    onMouseEnter={e => {
                      if (rightPanel !== tab)
                        (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)'
                    }}
                    onMouseLeave={e => {
                      if (rightPanel !== tab)
                        (e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'
                    }}
                  >
                    {tab}
                  </button>
                ))}
              </div>

              {/* Panel content */}
              {rightPanel === 'files'    && <FileViewer projectId={activeProject.id} sessionId={activeSession.id} />}
              {rightPanel === 'app'      && <AppPreviewPanel />}
              {rightPanel === 'analysis' && <SessionStatsPanel project={activeProject} session={activeSession} />}
            </Panel>
          </>
        )}
      </PanelGroup>
      </Panel>
    </PanelGroup>

      {/* ── Directory browser modal ─────────────────────────────────── */}
      {showDirBrowser && (
        <DirectoryBrowserModal
          onSelect={p => { setNewProjectPath(p); setShowDirBrowser(false) }}
          onClose={() => setShowDirBrowser(false)}
        />
      )}

      {/* ── New Project modal ──────────────────────────────────────── */}
      {showNewProject && (
        <div style={{
          position: 'fixed', inset: 0,
          background: 'rgba(20,16,10,0.18)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          zIndex: 50,
        }}
          onClick={e => { if (e.target === e.currentTarget) setShowNewProject(false) }}
        >
          <div style={{
            background: 'var(--bg-modal)',
            border: '1px solid var(--border)',
            borderRadius: 6,
            padding: 24,
            width: 400,
            boxShadow: '0 8px 24px rgba(0,0,0,0.12)',
          }}>
            {/* Header */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
              <h3 style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                New Project
              </h3>
              <button
                onClick={() => setShowNewProject(false)}
                style={{
                  fontSize: 16, color: 'var(--text-very-muted)',
                  background: 'none', border: 'none', cursor: 'pointer', lineHeight: 1,
                }}
              >×</button>
            </div>

            <div style={{ borderTop: '1px solid var(--border)', marginBottom: 16 }} />

            {/* Folder path */}
            <label style={{
              display: 'block', fontSize: 10, fontWeight: 600,
              textTransform: 'uppercase', letterSpacing: '0.08em',
              color: 'var(--text-very-muted)', marginBottom: 6,
            }}>Folder</label>
            <div style={{ display: 'flex', gap: 6, marginBottom: 20 }}>
              <input
                style={{
                  flex: 1,
                  padding: '7px 10px',
                  fontSize: 13,
                  background: 'var(--bg-input)',
                  border: '1px solid var(--border)',
                  borderRadius: 3,
                  color: 'var(--text-muted)',
                  outline: 'none',
                }}
                placeholder="/absolute/path/to/project"
                value={newProjectPath}
                autoFocus
                onChange={e => setNewProjectPath(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleCreateProject()}
                onFocus={e => (e.currentTarget as HTMLElement).style.borderColor = 'var(--amber)'}
                onBlur={e => (e.currentTarget as HTMLElement).style.borderColor = 'var(--border)'}
              />
              <button
                onClick={() => setShowDirBrowser(true)}
                type="button"
                style={{
                  padding: '7px 12px',
                  fontSize: 12,
                  background: 'var(--bg-pane-title)',
                  border: '1px solid var(--border)',
                  borderRadius: 3,
                  color: 'var(--text-muted)',
                  cursor: 'pointer',
                  flexShrink: 0,
                }}
              >Browse…</button>
            </div>

            {/* Actions */}
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button
                onClick={() => setShowNewProject(false)}
                style={{
                  padding: '7px 14px', fontSize: 13,
                  color: 'var(--text-muted)',
                  background: 'none', border: 'none', cursor: 'pointer',
                }}
              >Cancel</button>
              <button
                onClick={handleCreateProject}
                style={{
                  padding: '7px 16px', fontSize: 13,
                  background: 'var(--bg-modal)',
                  border: '1px solid var(--border-dark)',
                  borderRadius: 4,
                  color: 'var(--text-primary)',
                  cursor: 'pointer',
                }}
              >Create project →</button>
            </div>
          </div>
        </div>
      )}

      {/* Hover reveal for delete buttons */}
      <style>{`
        .group:hover .delete-btn { opacity: 1 !important; }
        [data-panel-resize-handle-id]:hover { background: var(--amber) !important; }
      `}</style>
    </div>
  )
}
