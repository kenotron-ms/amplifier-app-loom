import { useEffect, useState } from 'react'
import { type Project, listProjects, listAmplifierSessions } from '../../api/projects'
import ProjectCard from './ProjectCard'

interface Props {
  onSelectProject: (id: string) => void
}

export default function ProjectsGrid({ onSelectProject }: Props) {
  const [projects, setProjects] = useState<Project[]>([])
  const [sessionCounts, setSessionCounts] = useState<Record<string, number>>({})
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    listProjects()
      .then(async (ps) => {
        setProjects(ps)
        const counts: Record<string, number> = {}
        await Promise.all(
          ps.map(async (p) => {
            try {
              const sessions = await listAmplifierSessions(p.id)
              counts[p.id] = sessions.length
            } catch {
              counts[p.id] = 0
            }
          }),
        )
        setSessionCounts(counts)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const groups = new Map<string, Project[]>()
  for (const p of projects) {
    const ws = p.workspace || 'Default'
    if (!groups.has(ws)) groups.set(ws, [])
    groups.get(ws)!.push(p)
  }

  if (loading) {
    return <div style={{ background: '#12141a', height: '100%', padding: 24, color: '#6b7280' }}>Loading projects...</div>
  }

  return (
    <div style={{ background: '#12141a', height: '100%', overflowY: 'auto', padding: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 24 }}>
        <button style={{
          fontSize: 12, fontWeight: 500, padding: '6px 16px',
          color: '#9ca3af', background: 'transparent',
          border: '1px solid #252832', borderRadius: 6, cursor: 'pointer',
        }}>
          Add Project
        </button>
      </div>

      {Array.from(groups.entries()).map(([workspace, wsProjects]) => (
        <div key={workspace} style={{ marginBottom: 32 }}>
          <div style={{
            fontSize: 11, fontWeight: 600, textTransform: 'uppercase',
            letterSpacing: '0.08em', color: '#6b7280', marginBottom: 12,
          }}>
            {workspace}
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16 }}>
            {wsProjects.map(p => (
              <ProjectCard
                key={p.id}
                project={p}
                sessionCount={sessionCounts[p.id] ?? 0}
                onSelect={onSelectProject}
              />
            ))}
          </div>
        </div>
      ))}

      {projects.length === 0 && (
        <div style={{ color: '#6b7280', textAlign: 'center', paddingTop: 64, fontSize: 13 }}>
          No projects found. Add a project to get started.
        </div>
      )}
    </div>
  )
}
