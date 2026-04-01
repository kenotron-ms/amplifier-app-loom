import { useEffect, useMemo, useState } from 'react'
import Convert from 'ansi-to-html'
import { Job, JobRun, listJobRuns, triggerJob } from '../../api/jobs'
import { useRunStream } from './useRunStream'

// newline:false — <pre> with whitespace-pre-wrap already renders \n as line breaks
const ansiConvert = new Convert({ escapeXML: true, newline: false })

interface Props {
  job: Job
}

export default function RunDetail({ job }: Props) {
  const [runs, setRuns] = useState<JobRun[]>([])
  const [activeRunId, setActiveRunId] = useState<string | null>(null)
  const logOutput = useRunStream(activeRunId)
  const logHtml = useMemo(() => ansiConvert.toHtml(logOutput), [logOutput])

  const refreshRuns = async (jobId: string) => {
    try {
      const rs = await listJobRuns(jobId)
      const safe = rs ?? []
      setRuns(safe)
      if (safe.length > 0) setActiveRunId(safe[0].id)
    } catch (e) {
      console.error('listJobRuns:', e)
    }
  }

  useEffect(() => {
    refreshRuns(job.id)
  }, [job.id])

  const handleTrigger = async () => {
    try {
      await triggerJob(job.id)
      // Poll for the new run after a short delay — API returns {status:"triggered"}, not a run
      setTimeout(() => refreshRuns(job.id), 800)
    } catch (e) {
      console.error('triggerJob:', e)
    }
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

      {/* Log output — ANSI colour codes converted to HTML spans */}
      <div className="flex-1 overflow-y-auto bg-[#0d1117] p-4">
        {logOutput
          ? <pre
              className="font-mono text-[11px] text-[#e6edf3] whitespace-pre-wrap leading-relaxed m-0"
              dangerouslySetInnerHTML={{ __html: logHtml }}
            />
          : <span className="font-mono text-[11px] text-[#8b949e]">No output yet — click ▶ Run Now to trigger a run.</span>
        }
      </div>
    </div>
  )
}
