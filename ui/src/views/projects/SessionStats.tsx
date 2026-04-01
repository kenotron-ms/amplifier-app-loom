import { useEffect, useState } from 'react'
import { Session, SessionStats, getSessionStats } from '../../api/projects'

interface Props {
  project: { id: string; name: string; path: string }
  session: Session
}

function StatCard({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div className="bg-[#161b22] border border-[#30363d] rounded p-3">
      <div className="text-[10px] text-[#8b949e] uppercase tracking-wider mb-1">{label}</div>
      <div className="text-lg font-semibold text-[#e6edf3]">{value}</div>
      {sub && <div className="text-[10px] text-[#484f58] mt-0.5">{sub}</div>}
    </div>
  )
}

export default function SessionStatsPanel({ project, session }: Props) {
  const [stats, setStats] = useState<SessionStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    getSessionStats(project.id, session.id)
      .then(setStats)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [project.id, session.id])

  const created = new Date(session.createdAt * 1000)
  const age = Math.floor((Date.now() - created.getTime()) / 1000)
  const ageStr = age < 60 ? `${age}s ago`
    : age < 3600 ? `${Math.floor(age / 60)}m ago`
    : age < 86400 ? `${Math.floor(age / 3600)}h ago`
    : `${Math.floor(age / 86400)}d ago`

  return (
    <div className="flex flex-col h-full bg-[#0d1117] overflow-y-auto">
      {/* Session header */}
      <div className="px-4 py-3 border-b border-[#30363d] shrink-0">
        <div className="text-xs font-semibold text-[#e6edf3] mb-0.5">{session.name}</div>
        <div className="text-[10px] text-[#484f58] font-mono truncate">{session.worktreePath}</div>
      </div>

      <div className="p-4 space-y-4">
        {/* Session metadata */}
        <div>
          <div className="text-[10px] text-[#8b949e] uppercase tracking-wider mb-2">Session</div>
          <div className="grid grid-cols-2 gap-2">
            <StatCard label="Status" value={session.status} />
            <StatCard label="Created" value={ageStr} sub={created.toLocaleTimeString()} />
          </div>
        </div>

        {/* Amplifier stats */}
        <div>
          <div className="text-[10px] text-[#8b949e] uppercase tracking-wider mb-2">Amplifier</div>
          {loading && (
            <div className="text-[10px] text-[#484f58]">Loading stats…</div>
          )}
          {error && (
            <div className="text-[10px] text-[#f85149]">{error}</div>
          )}
          {stats && (
            <div className="grid grid-cols-2 gap-2">
              <StatCard
                label="Tokens"
                value={stats.tokens > 0 ? stats.tokens.toLocaleString() : '—'}
                sub={stats.tokens > 0 ? 'total' : 'run amplifier to track'}
              />
              <StatCard
                label="Tool calls"
                value={stats.tools > 0 ? stats.tools : '—'}
                sub={stats.turns ? `${stats.turns} turns` : undefined}
              />
              {stats.model && (
                <div className="col-span-2">
                  <StatCard label="Model" value={stats.model} />
                </div>
              )}
            </div>
          )}
        </div>

        {/* Project info */}
        <div>
          <div className="text-[10px] text-[#8b949e] uppercase tracking-wider mb-2">Project</div>
          <div className="bg-[#161b22] border border-[#30363d] rounded p-3 space-y-1.5">
            <div className="flex justify-between text-[11px]">
              <span className="text-[#8b949e]">Name</span>
              <span className="text-[#e6edf3] truncate ml-4">{project.name}</span>
            </div>
            <div className="flex justify-between text-[11px]">
              <span className="text-[#8b949e] shrink-0">Path</span>
              <span className="text-[#e6edf3] font-mono text-[10px] truncate ml-4">{project.path}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
