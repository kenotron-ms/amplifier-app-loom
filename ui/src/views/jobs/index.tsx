import { useEffect, useState } from 'react'
import { Job, listJobs } from '../../api/jobs'
import JobList from './JobList'
import RunDetail from './RunDetail'

export default function JobsView() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)

  useEffect(() => {
    listJobs().then(js => {
      setJobs(js)
      if (js.length > 0) setSelectedId(js[0].id)
    }).catch(console.error)
  }, [])

  const selectedJob = jobs.find(j => j.id === selectedId) ?? null

  return (
    <div className="flex h-full">
      <JobList
        jobs={jobs}
        selectedId={selectedId}
        onSelect={setSelectedId}
        onNew={() => window.open('http://localhost:7700', '_self')}
      />
      <div className="flex-1 overflow-hidden">
        {selectedJob
          ? <RunDetail job={selectedJob} />
          : <div className="p-8 text-[#8b949e] text-sm">Select a job to view details</div>
        }
      </div>
    </div>
  )
}
