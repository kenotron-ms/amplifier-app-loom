import { Job } from '../../api/jobs'

interface Props {
  jobs: Job[]
  selectedId: string | null
  onSelect: (id: string) => void
  onNew: () => void
}

export default function JobList({ jobs, selectedId, onSelect, onNew }: Props) {
  const statusDot = (job: Job) => {
    const color = job.lastRunStatus === 'running' ? '#3fb950' : '#8b949e'
    return (
      <span
        style={{ width: 6, height: 6, borderRadius: '50%', background: color, display: 'inline-block', flexShrink: 0 }}
      />
    )
  }

  return (
    <div className="flex flex-col h-full bg-[#0d1117] border-r border-[#30363d] w-52 shrink-0">
      <div className="flex items-center justify-between px-3 py-2 border-b border-[#30363d]">
        <span className="text-[#8b949e] text-xs uppercase tracking-wider">Jobs</span>
        <button onClick={onNew} className="text-xs text-[#58a6ff] hover:text-[#e6edf3]">
          + New
        </button>
      </div>
      <div className="flex-1 overflow-y-auto">
        {jobs.map(job => (
          <button
            key={job.id}
            onClick={() => onSelect(job.id)}
            className={[
              'w-full text-left px-3 py-2 border-b border-[#21262d] hover:bg-[#161b22] transition-colors',
              selectedId === job.id ? 'bg-[#21262d]' : '',
            ].join(' ')}
          >
            <div className="flex items-center justify-between gap-2">
              <span
                className={`text-xs truncate ${selectedId === job.id ? 'text-[#e6edf3]' : 'text-[#8b949e]'}`}
              >
                {job.name}
              </span>
              {statusDot(job)}
            </div>
            <div className="text-[10px] text-[#8b949e] mt-0.5">
              {job.trigger.type} {job.trigger.schedule && `· ${job.trigger.schedule}`}
            </div>
          </button>
        ))}
        {jobs.length === 0 && (
          <div className="px-3 py-4 text-xs text-[#8b949e]">No jobs yet</div>
        )}
      </div>
    </div>
  )
}
