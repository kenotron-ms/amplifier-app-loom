import { type Project, openTerminal } from '../../api/projects'

interface Props {
  project: Project
  sessionCount: number
  onSelect: (id: string) => void
}

function shortenPath(fullPath: string): string {
  return fullPath.replace(/^\/Users\/[^/]+/, '~')
}

export default function ProjectCard({ project, sessionCount, onSelect }: Props) {
  const hasActive = sessionCount > 0

  async function handleNewSession(e: React.MouseEvent) {
    e.stopPropagation()
    try {
      await openTerminal(project.id, 'new')
    } catch (err) {
      console.error('Failed to open terminal:', err)
    }
  }

  return (
    <div
      onClick={() => onSelect(project.id)}
      style={{
        background: '#1c1f27',
        border: '1px solid #252832',
        borderRadius: 8,
        boxShadow: '0 2px 8px rgba(0,0,0,0.4)',
        padding: 16,
        cursor: 'pointer',
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
        transition: 'border-color 0.15s ease',
      }}
      onMouseEnter={e => (e.currentTarget.style.borderColor = '#3a3f4b')}
      onMouseLeave={e => (e.currentTarget.style.borderColor = '#252832')}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{
          fontSize: 14, fontWeight: 600, color: '#ffffff',
          flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {project.name}
        </span>
        <span style={{
          width: 8, height: 8, borderRadius: '50%',
          background: hasActive ? '#4CAF74' : '#6b7280', flexShrink: 0,
        }} />
      </div>

      <div style={{
        fontFamily: 'monospace', fontSize: 12, color: '#4b5563',
        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
      }}>
        {shortenPath(project.path)}
      </div>

      <div>
        <span style={{
          display: 'inline-block', fontSize: 11, fontWeight: 500,
          padding: '2px 8px', borderRadius: 9999,
          background: hasActive ? 'rgba(76,175,116,0.15)' : 'rgba(107,114,128,0.15)',
          color: hasActive ? '#4CAF74' : '#6b7280',
        }}>
          {sessionCount} {sessionCount === 1 ? 'session' : 'sessions'}
        </span>
      </div>

      <button
        onClick={handleNewSession}
        style={{
          width: '100%', padding: '6px 0', marginTop: 4,
          fontSize: 12, fontWeight: 500, color: '#9ca3af',
          background: 'transparent', border: '1px solid #252832',
          borderRadius: 6, cursor: 'pointer', transition: 'all 0.15s ease',
        }}
        onMouseEnter={e => { e.currentTarget.style.borderColor = '#14b8a6'; e.currentTarget.style.color = '#14b8a6' }}
        onMouseLeave={e => { e.currentTarget.style.borderColor = '#252832'; e.currentTarget.style.color = '#9ca3af' }}
      >
        New Session
      </button>
    </div>
  )
}
