import { useEffect, useState } from 'react'
import {
  type Project, type AmplifierSession,
  getProject, listAmplifierSessions,
} from '../../api/projects'
import SessionsList from './SessionsList'
import { ProjectSettingsPanel } from './ProjectSettingsPanel'
import FileViewer from './FileViewer'

interface Props {
  projectId: string
  onBack: () => void
}

type Tab = 'sessions' | 'settings' | 'files'
const TABS: { id: Tab; label: string }[] = [
  { id: 'sessions', label: 'Sessions' },
  { id: 'settings', label: 'Settings' },
  { id: 'files', label: 'Files' },
]

export default function ProjectDetail({ projectId, onBack }: Props) {
  const [project, setProject] = useState<Project | null>(null)
  const [activeTab, setActiveTab] = useState<Tab>('sessions')
  const [sessions, setSessions] = useState<AmplifierSession[]>([])

  useEffect(() => {
    getProject(projectId).then(setProject).catch(console.error)
    listAmplifierSessions(projectId).then(setSessions).catch(() => setSessions([]))
  }, [projectId])

  return (
    <div style={{ background: '#12141a', height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '16px 24px', borderBottom: '1px solid #252832', flexShrink: 0,
      }}>
        <button
          onClick={onBack}
          style={{ fontSize: 13, color: '#9ca3af', background: 'transparent', border: 'none', cursor: 'pointer', padding: '4px 8px' }}
          onMouseEnter={e => (e.currentTarget.style.color = '#ffffff')}
          onMouseLeave={e => (e.currentTarget.style.color = '#9ca3af')}
        >
          ← Back
        </button>
        <h1 style={{ fontSize: 18, fontWeight: 600, color: '#ffffff', margin: 0 }}>
          {project?.name ?? 'Loading...'}
        </h1>
      </div>

      <div style={{ display: 'flex', gap: 0, padding: '0 24px', borderBottom: '1px solid #252832', flexShrink: 0 }}>
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            style={{
              padding: '10px 16px', fontSize: 13, fontWeight: 500,
              color: activeTab === tab.id ? '#ffffff' : '#6b7280',
              background: 'transparent', border: 'none',
              borderBottom: activeTab === tab.id ? '2px solid #14b8a6' : '2px solid transparent',
              cursor: 'pointer',
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div style={{ flex: 1, overflowY: 'auto' }}>
        {activeTab === 'sessions' && <SessionsList projectId={projectId} />}
        {activeTab === 'settings' && <ProjectSettingsPanel projectId={projectId} />}
        {activeTab === 'files' && (
          sessions.length > 0
            ? <FileViewer projectId={projectId} sessionId={sessions[0].id} />
            : <div style={{ padding: 16, color: '#6b7280', fontSize: 13 }}>No sessions available for file browsing.</div>
        )}
      </div>
    </div>
  )
}
