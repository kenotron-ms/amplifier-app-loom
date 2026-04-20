import { Connector } from '../../api/mirror'

interface Props {
  connectors: Connector[]
  selectedId: string | null
  onSelect: (id: string) => void
}

function HealthDot({ health }: { health: string }) {
  const color = health === 'live'
    ? 'var(--green)'
    : health === 'error'
    ? 'var(--red)'
    : 'var(--text-very-muted)'
  return (
    <span style={{
      width: 6, height: 6, borderRadius: '50%',
      background: color,
      display: 'inline-block', flexShrink: 0,
    }} />
  )
}

export default function ConnectorList({ connectors, selectedId, onSelect }: Props) {
  return (
    <div style={{
      width: '100%',
      display: 'flex',
      flexDirection: 'column',
      background: 'var(--bg-sidebar)',
      borderRight: '1px solid var(--border)',
      height: '100%',
    }}>
      {/* Header */}
      <div style={{
        padding: '0 12px',
        height: 32,
        display: 'flex', alignItems: 'center',
        borderBottom: '1px solid var(--border)',
        flexShrink: 0,
      }}>
        <span style={{
          fontSize: 10, fontWeight: 600,
          textTransform: 'uppercase', letterSpacing: '0.08em',
          color: 'var(--text-very-muted)',
        }}>Connectors</span>
      </div>

      {/* List */}
      <div style={{ flex: 1, overflowY: 'auto' }} className="canvas-scroll">
        {connectors.map(c => (
          <button
            key={c.id}
            onClick={() => onSelect(c.id)}
            style={{
              width: '100%', textAlign: 'left',
              padding: '7px 12px 7px 14px',
              display: 'flex', alignItems: 'center', gap: 8,
              background: selectedId === c.id ? 'var(--bg-sidebar-active)' : 'transparent',
              borderLeft: selectedId === c.id ? '2px solid var(--amber)' : '2px solid transparent',
              borderBottom: '1px solid var(--border)',
              cursor: 'pointer',
              transition: 'background 0.12s ease',
            }}
            onMouseEnter={e => {
              if (selectedId !== c.id)
                (e.currentTarget as HTMLElement).style.background = 'rgba(0,0,0,0.03)'
            }}
            onMouseLeave={e => {
              if (selectedId !== c.id)
                (e.currentTarget as HTMLElement).style.background = 'transparent'
            }}
          >
            <HealthDot health={c.health} />
            <div style={{ minWidth: 0, flex: 1 }}>
              <div style={{
                fontSize: 12,
                fontWeight: selectedId === c.id ? 500 : 400,
                color: selectedId === c.id ? 'var(--text-primary)' : 'var(--text-muted)',
                overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
              }}>{c.name}</div>
              <div style={{ fontSize: 10, color: 'var(--text-very-muted)', marginTop: 2 }}>
                {c.type}
              </div>
            </div>
          </button>
        ))}
        {connectors.length === 0 && (
          <div style={{ padding: '16px 14px', fontSize: 11, color: 'var(--text-very-muted)' }}>
            No connectors configured
          </div>
        )}
      </div>
    </div>
  )
}
