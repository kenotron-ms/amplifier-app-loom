import { useState } from 'react'
import ProjectsView from './views/projects'
import JobsView from './views/jobs'
import MirrorView from './views/mirror'
import FeedbackModal from './components/FeedbackModal'

type Tab = 'projects' | 'jobs' | 'mirror'

const TABS: { id: Tab; label: string }[] = [
  { id: 'projects', label: 'Projects' },
  { id: 'jobs',     label: 'Jobs' },
  { id: 'mirror',   label: 'Mirror' },
]

export default function App() {
  const [active, setActive]           = useState<Tab>('projects')
  const [showFeedback, setShowFeedback] = useState(false)

  return (
    <div className="flex flex-col h-full bg-[#0d1117]">
      {/* Top nav */}
      <nav className="flex items-center bg-[#161b22] border-b border-[#30363d] px-3 h-9 shrink-0">
        <span className="text-[#8b949e] text-xs font-semibold mr-4">loom</span>
        <div className="flex h-full" role="tablist">
          {TABS.map(tab => (
            <button
              key={tab.id}
              role="tab"
              aria-selected={active === tab.id}
              onClick={() => setActive(tab.id)}
              className={[
                'px-3 h-full text-xs border-b-2 transition-colors',
                active === tab.id
                  ? 'border-[#58a6ff] text-[#e6edf3]'
                  : 'border-transparent text-[#8b949e] hover:text-[#e6edf3]',
              ].join(' ')}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Feedback button — right-aligned */}
        <button
          onClick={() => setShowFeedback(true)}
          className="ml-auto text-[10px] px-2 py-0.5 rounded bg-[#21262d] text-[#8b949e] hover:text-[#e6edf3] hover:bg-[#30363d] transition-colors"
          title="Send feedback or report a bug"
        >
          Feedback
        </button>
      </nav>

      {/* Mode content */}
      <div className="flex-1 overflow-hidden">
        {active === 'projects' && <ProjectsView />}
        {active === 'jobs'     && <JobsView />}
        {active === 'mirror'   && <MirrorView />}
      </div>

      {showFeedback && <FeedbackModal onClose={() => setShowFeedback(false)} />}
    </div>
  )
}
