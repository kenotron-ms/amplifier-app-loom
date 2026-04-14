import { useEffect, useState } from 'react'
import { type AmplifierSession, listAmplifierSessions, openTerminal } from '../../api/projects'

interface Props {
  projectId: string
}

function formatTimestamp(ts: string): string {
  const date = new Date(ts)
  return date.toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit',
  })
}

export default function SessionsList({ projectId }: Props) {
  const [sessions, setSessions] = useState<AmplifierSession[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    listAmplifierSessions(projectId)
      .then(setSessions)
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [projectId])

  async function handleNewSession() {
    try { await openTerminal(projectId, 'new') }
    catch (err) { console.error('Failed to open terminal:', err) }
  }

  async function handleOpenSession(sessionId: string) {
    try { await openTerminal(projectId, 'resume', sessionId) }
    catch (err) { console.error('Failed to resume session:', err) }
  }

  if (loading) return <div style={{ padding: 16, color: 'var(--text-very-muted)' }}>Loading sessions...</div>

  const sorted = [...sessions].sort((a, b) =>
    (b.isActive ? 1 : 0) - (a.isActive ? 1 : 0)
  )

  return (
    <div style={{ padding: 16 }}>
      <button
        onClick={handleNewSession}
        style={{
          fontSize: 13, fontWeight: 500, padding: '8px 20px', marginBottom: 16,
          color: '#14b8a6', background: 'transparent',
          border: '1px solid #14b8a6', borderRadius: 6, cursor: 'pointer',
        }}
        onMouseEnter={e => (e.currentTarget.style.background = 'rgba(20,184,166,0.08)')}
        onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
      >
        New Session
      </button>

      {sorted.length === 0 ? (
        <div style={{ color: 'var(--text-very-muted)', fontSize: 13, paddingTop: 16 }}>
          No Amplifier sessions found for this project.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          {sorted.map(s => (
            <div key={s.id} style={{
              display: 'flex', alignItems: 'center',
              padding: '10px 12px', borderBottom: '1px solid var(--border)', gap: 12,
            }}>
              <span style={{
                flex: 1, fontSize: 13, color: 'var(--text-primary)', fontWeight: 500,
                overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
              }}>
                {s.name || s.id}
              </span>
              {s.isActive && (
                <span style={{
                  fontSize: 10, fontWeight: 600,
                  padding: '1px 6px', borderRadius: 9999,
                  background: 'rgba(76,175,116,0.15)',
                  color: '#4caf74',
                  flexShrink: 0,
                }}>
                  Active
                </span>
              )}
              <span style={{ fontSize: 12, color: 'var(--text-very-muted)', flexShrink: 0 }}>
                {formatTimestamp(s.lastActiveAt || s.createdAt)}
              </span>
              <button
                onClick={() => handleOpenSession(s.id)}
                style={{
                  fontSize: 12, fontWeight: 500, padding: '4px 12px',
                  color: '#14b8a6', background: 'transparent',
                  border: '1px solid var(--border)', borderRadius: 4,
                  cursor: 'pointer', flexShrink: 0,
                }}
                onMouseEnter={e => (e.currentTarget.style.borderColor = '#14b8a6')}
                onMouseLeave={e => (e.currentTarget.style.borderColor = 'var(--border)')}
              >
                Open
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
