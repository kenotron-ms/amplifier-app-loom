import { useEffect, useState } from 'react'
import { Job, JobRun, listJobRuns, triggerJob } from '../../api/jobs'
import { useRunStream } from './useRunStream'

interface Props {
  job: Job
}

export default function RunDetail({ job }: Props) {
  const [runs, setRuns] = useState<JobRun[]>([])
  const [activeRunId, setActiveRunId] = useState<string | null>(null)
  const logLines = useRunStream(activeRunId)

  useEffect(() => {
    listJobRuns(job.id).then(rs => {
      setRuns(rs)
      if (rs.length > 0 && !activeRunId) setActiveRunId(rs[0].id)
    })
  }, [job.id])

  const handleTrigger = async () => {
    const run = await triggerJob(job.id)
    setRuns(prev => [run, ...prev])
    setActiveRunId(run.id)
  }

  const statusBadge = (status: string) => {
    const styles: Record<string, string> = {
      running:   'bg-[#1a3a2a] text-[#3fb950]',
      succeeded: 'bg-[#1a3a2a] text-[#3fb950]',
      failed:    'bg-[#3a1a1a] text-[#f85149]',
      cancelled: 'bg-[#21262d] text-[#8b949e]',
    }
    return (
      <span className={`text-[10px] px-1.5 py-0.5 rounded ${styles[status] ?? styles.cancelled}`}>
        {status}
      </span>
    )
  }

  const activeRun = runs.find(r => r.id === activeRunId) ?? null

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2 bg-[#161b22] border-b border-[#30363d] shrink-0">
        <span className="text-sm font-semibold text-[#e6edf3]">{job.name}</span>
        {activeRun && statusBadge(activeRun.status)}
        <button
          onClick={handleTrigger}
          className="ml-auto text-xs px-3 py-1 bg-[#21262d] border border-[#30363d] rounded text-[#e6edf3] hover:bg-[#30363d]"
        >
          ▶ Run Now
        </button>
      </div>

      {/* Run history tabs */}
      {runs.length > 0 && (
        <div className="flex gap-1 px-4 py-1.5 bg-[#0d1117] border-b border-[#21262d] shrink-0 overflow-x-auto">
          {runs.slice(0, 10).map((run, i) => (
            <button
              key={run.id}
              onClick={() => setActiveRunId(run.id)}
              className={[
                'text-[10px] px-2 py-0.5 rounded shrink-0',
                activeRunId === run.id
                  ? 'bg-[#21262d] text-[#e6edf3]'
                  : 'text-[#8b949e] hover:text-[#e6edf3]',
              ].join(' ')}
            >
              #{runs.length - i}
            </button>
          ))}
        </div>
      )}

      {/* Log output */}
      <div className="flex-1 overflow-y-auto font-mono text-[11px] text-[#e6edf3] bg-[#0d1117] p-4 leading-relaxed">
        {logLines.length > 0
          ? logLines.map((line, i) => <div key={i}>{line}</div>)
          : <span className="text-[#8b949e]">No output yet</span>
        }
      </div>
    </div>
  )
}
