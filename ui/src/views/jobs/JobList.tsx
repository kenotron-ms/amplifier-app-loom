import { useState } from 'react'
import { Job } from '../../api/jobs'

interface Props {
  jobs: Job[]
  selectedId: string | null
  onSelect: (id: string) => void
  onNew: () => void
  onDelete: (id: string) => void
}

function StatusDot({ status }: { status: string }) {
  const isRunning = status === 'running'
  return (
    <span style={{
      width: 6, height: 6, borderRadius: '50%',
      background: isRunning ? 'var(--amber)' : 'var(--text-very-muted)',
      display: 'inline-block', flexShrink: 0,
    }} />
  )
}

export default function JobList({ jobs, selectedId, onSelect, onNew, onDelete }: Props) {
  const [hoveredId, setHoveredId] = useState<string | null>(null)

  return (
    <div style={{
      width: '100%', display: 'flex', flexDirection: 'column',
      background: 'var(--bg-sidebar)', borderRight: '1px solid var(--border)', height: '100%',
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '0 12px', height: 32,
        borderBottom: '1px solid var(--border)', flexShrink: 0,
      }}>
        <span style={{
          fontSize: 10, fontWeight: 600,
          textTransform: 'uppercase', letterSpacing: '0.08em',
          color: 'var(--text-very-muted)',
        }}>Jobs</span>
        <button
          onClick={onNew}
          style={{
            fontSize: 14, lineHeight: 1, color: 'var(--text-muted)',
            background: 'none', border: 'none', cursor: 'pointer', padding: '0 2px',
          }}
          onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--amber)'}
          onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)'}
          title="New job"
        >+</button>
      </div>

      <div style={{ flex: 1, overflowY: 'auto' }} className="canvas-scroll">
        {jobs.map(job => (
          <div
            key={job.id}
            style={{ position: 'relative' }}
            onMouseEnter={() => setHoveredId(job.id)}
            onMouseLeave={() => setHoveredId(null)}
          >
            <button
              onClick={() => onSelect(job.id)}
              style={{
                width: '100%', textAlign: 'left',
                padding: '7px 12px 7px 14px',
                display: 'flex', alignItems: 'flex-start', gap: 8,
                background: selectedId === job.id ? 'var(--bg-sidebar-active)' : 'transparent',
                borderLeft: selectedId === job.id ? '2px solid var(--amber)' : '2px solid transparent',
                borderBottom: '1px solid var(--border)',
                borderTop: 'none', borderRight: 'none',
                cursor: 'pointer', transition: 'background 0.12s ease',
              }}
            >
              <StatusDot status={job.lastRunStatus ?? 'idle'} />
              <div style={{ minWidth: 0, flex: 1 }}>
                <div style={{
                  fontSize: 12,
                  fontWeight: selectedId === job.id ? 500 : 400,
                  color: selectedId === job.id ? 'var(--text-primary)' : 'var(--text-muted)',
                  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                }}>{job.name}</div>
                <div style={{ fontSize: 10, color: 'var(--text-very-muted)', marginTop: 2 }}>
                  {job.trigger.type}
                  {job.trigger.schedule && ` · ${job.trigger.schedule}`}
                </div>
              </div>
            </button>

            {/* X delete button — visible only on hover */}
            <button
              onClick={(e) => { e.stopPropagation(); onDelete(job.id) }}
              style={{
                position: 'absolute', top: 6, right: 6,
                width: 18, height: 18,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                background: 'var(--bg-pane-title)',
                border: '1px solid var(--border)',
                borderRadius: 3,
                color: 'var(--text-muted)', fontSize: 11, lineHeight: 1,
                cursor: 'pointer',
                opacity: hoveredId === job.id ? 1 : 0,
                pointerEvents: hoveredId === job.id ? 'auto' : 'none',
                transition: 'opacity 0.12s ease',
                padding: 0,
              }}
              onMouseEnter={e => {
                (e.currentTarget as HTMLElement).style.color = 'var(--red)'
                ;(e.currentTarget as HTMLElement).style.borderColor = 'var(--red)'
              }}
              onMouseLeave={e => {
                (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)'
                ;(e.currentTarget as HTMLElement).style.borderColor = 'var(--border)'
              }}
              title="Remove job"
            >×</button>
          </div>
        ))}
        {jobs.length === 0 && (
          <div style={{ padding: '16px 14px', fontSize: 11, color: 'var(--text-very-muted)' }}>
            No jobs yet
          </div>
        )}
      </div>
    </div>
  )
}
