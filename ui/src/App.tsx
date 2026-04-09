import { useState } from 'react'
import ProjectsView from './views/projects'
import JobsView from './views/jobs'
import MirrorView from './views/mirror'
import BundlesView from './views/bundles'
import FeedbackModal from './components/FeedbackModal'
import { useSingleInstance } from './hooks/useSingleInstance'

type Tab = 'projects' | 'jobs' | 'mirror' | 'bundles'

const TABS: { id: Tab; label: string }[] = [
  { id: 'projects', label: 'Projects' },
  { id: 'jobs',     label: 'Jobs' },
  { id: 'mirror',   label: 'Mirror' },
  { id: 'bundles',  label: 'Bundles' },
]

// Loom woven-grid logo mark
function LoomLogo() {
  return (
    <svg width="22" height="22" viewBox="0 0 36 36" xmlns="http://www.w3.org/2000/svg" style={{ display: 'block', flexShrink: 0 }}>
      <rect width="36" height="36" rx="7" fill="#1C1A16"/>
      {/* Horizontal stripes (teal) */}
      <rect x="4" y="4"  width="28" height="6" fill="#5A7E85"/>
      <rect x="4" y="15" width="28" height="6" fill="#5A7E85"/>
      <rect x="4" y="26" width="28" height="6" fill="#5A7E85"/>
      {/* Vertical stripes (mustard) on top */}
      <rect x="4"  y="4" width="6" height="28" fill="#D09D59"/>
      <rect x="15" y="4" width="6" height="28" fill="#D09D59"/>
      <rect x="26" y="4" width="6" height="28" fill="#D09D59"/>
      {/* Teal back on top at corners — weave pattern */}
      <rect x="4"  y="4"  width="6" height="6" fill="#5A7E85"/>
      <rect x="26" y="4"  width="6" height="6" fill="#5A7E85"/>
      <rect x="4"  y="26" width="6" height="6" fill="#5A7E85"/>
      <rect x="26" y="26" width="6" height="6" fill="#5A7E85"/>
    </svg>
  )
}

// Header icon buttons (SVG inline — feather-style)

function BellIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
      <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
    </svg>
  )
}

function SettingsIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
    </svg>
  )
}

function DuplicateTabScreen() {
  return (
    <div
      style={{
        position: 'fixed', inset: 0,
        display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center',
        background: 'var(--bg-page)',
        gap: 12,
      }}
    >
      <LoomLogo />
      <p style={{ fontSize: 14, fontWeight: 500, color: 'var(--text-primary)', margin: 0 }}>
        Loom is already open
      </p>
      <p style={{ fontSize: 12, color: 'var(--text-muted)', margin: 0, textAlign: 'center', maxWidth: 260 }}>
        Another dashboard tab is already running.<br />
        Switch to it or close this tab.
      </p>
      <button
        onClick={() => window.close()}
        style={{
          marginTop: 4,
          padding: '5px 14px',
          fontSize: 12,
          fontWeight: 500,
          color: 'var(--text-primary)',
          background: 'var(--bg-input)',
          border: '1px solid var(--border)',
          borderRadius: 5,
          cursor: 'pointer',
        }}
      >
        Close this tab
      </button>
    </div>
  )
}

export default function App() {
  const isDuplicate                     = useSingleInstance()
  const [active, setActive]             = useState<Tab>('projects')
  const [showFeedback, setShowFeedback] = useState(false)

  if (isDuplicate) return <DuplicateTabScreen />

  return (
    <div className="flex flex-col h-full" style={{ background: 'var(--bg-page)' }}>

      {/* ── App Header ──────────────────────────────────────────────── */}
      <header
        className="flex items-center shrink-0 px-3 gap-0"
        style={{
          height: 38,
          background: 'var(--bg-header)',
          borderBottom: '1px solid var(--border)',
        }}
      >
        {/* Logo + brand */}
        <div className="flex items-center gap-1.5 mr-4">
          <LoomLogo />
          <span style={{ fontSize: 13, fontWeight: 500, color: 'var(--text-primary)', letterSpacing: '-0.01em' }}>
            Loom
          </span>
        </div>

        {/* Section tabs */}
        <div className="flex h-full" role="tablist">
          {TABS.map(tab => (
            <button
              key={tab.id}
              role="tab"
              aria-selected={active === tab.id}
              onClick={() => setActive(tab.id)}
              style={{
                height: '100%',
                padding: '0 12px',
                fontSize: 12,
                fontWeight: 500,
                color: active === tab.id ? 'var(--text-primary)' : 'var(--text-muted)',
                background: 'transparent',
                borderBottom: active === tab.id ? '2px solid var(--amber)' : '2px solid transparent',
                borderTop: 'none', borderLeft: 'none', borderRight: 'none',
                cursor: 'pointer',
                transition: 'color 0.12s ease',
              }}
              onMouseEnter={e => {
                if (active !== tab.id) (e.currentTarget as HTMLElement).style.color = 'var(--text-primary)'
              }}
              onMouseLeave={e => {
                if (active !== tab.id) (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)'
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Right-side icon buttons */}
        <div className="flex items-center gap-0.5 ml-auto">
          <button
            onClick={() => setShowFeedback(true)}
            style={{
              width: 26, height: 26,
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              color: 'var(--text-very-muted)',
              background: 'transparent', border: 'none', borderRadius: 3,
              cursor: 'pointer', transition: 'color 0.12s ease',
            }}
            title="Send feedback"
            onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-primary)'}
            onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'}
          >
            <BellIcon />
          </button>
          <button
            style={{
              width: 26, height: 26,
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              color: 'var(--text-very-muted)',
              background: 'transparent', border: 'none', borderRadius: 3,
              cursor: 'pointer', transition: 'color 0.12s ease',
            }}
            title="Settings"
            onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-primary)'}
            onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'}
          >
            <SettingsIcon />
          </button>
        </div>
      </header>

      {/* ── Section content ─────────────────────────────────────────── */}
      <div className="flex-1 overflow-hidden">
        {active === 'projects' && <ProjectsView />}
        {active === 'jobs'     && <JobsView />}
        {active === 'mirror'   && <MirrorView />}
        {active === 'bundles'  && <BundlesView />}
      </div>

      {showFeedback && <FeedbackModal onClose={() => setShowFeedback(false)} />}
    </div>
  )
}
