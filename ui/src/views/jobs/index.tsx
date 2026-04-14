import { useCallback, useEffect, useState } from 'react'
import { Job, deleteJob, listJobs } from '../../api/jobs'
import JobList from './JobList'
import RunDetail from './RunDetail'
import ChatView from '../chat'

export default function JobsView() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const loadJobs = useCallback(async () => {
    const js = await listJobs().catch(() => [] as Job[])
    setJobs(js)
  }, [])

  useEffect(() => { loadJobs() }, [loadJobs])

  const selectedJob = jobs.find(j => j.id === selectedId) ?? null

  const handleSelect = (id: string) => setSelectedId(id)

  // "+" New just clears selection — the right panel defaults to chat
  const handleNew = () => setSelectedId(null)

  const handleDelete = async (id: string) => {
    if (!window.confirm('Delete this job?')) return
    try {
      await deleteJob(id)
      setJobs(prev => prev.filter(j => j.id !== id))
      if (selectedId === id) setSelectedId(null)
    } catch (e) { console.error('deleteJob:', e) }
  }

  return (
    <div style={{ display: 'flex', height: '100%', background: 'var(--bg-page)' }}>
      <JobList
        jobs={jobs}
        selectedId={selectedId}
        onSelect={handleSelect}
        onNew={handleNew}
        onDelete={handleDelete}
      />
      <div className="flex-1 overflow-hidden">
        {selectedJob
          ? <RunDetail job={selectedJob} />
          : <ChatView onResponse={loadJobs} />
        }
      </div>
    </div>
  )
}
