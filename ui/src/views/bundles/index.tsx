import { useEffect, useState } from 'react'
import {
  RegistryEntry, AppBundle,
  fetchRegistry, listBundles, addBundle, removeBundle, toggleBundle,
} from '../../api/bundles'

// ── helpers ───────────────────────────────────────────────────────────────────

const TYPE_COLORS: Record<string, string> = {
  bundle: 'bg-[#388bfd]/20 text-[#F59E0B]',
  agent:  'bg-[#7C3F2A]/20 text-[#7C3F2A]',
  tool:   'bg-[#3fb950]/20 text-[#56d364]',
  module: 'bg-[#C4784A]/20 text-[#C4784A]',
}

function Stars({ rating }: { rating: number | null }) {
  const r       = rating ?? 0
  const full    = Math.floor(r)
  const hasHalf = r - full >= 0.4
  return (
    <span className="flex items-center gap-0.5 text-[var(--amber)] text-[10px]">
      {Array.from({ length: 5 }, (_, i) => {
        if (i < full)    return <span key={i}>★</span>
        if (i === full && hasHalf) return <span key={i} className="opacity-60">★</span>
        return <span key={i} className="text-[var(--border-dark)]">★</span>
      })}
      <span className="text-[var(--text-muted)] ml-0.5">{r.toFixed(1)}</span>
    </span>
  )
}

const CATEGORIES = ['all', 'dev', 'infra', 'knowledge', 'integration', 'research', 'ui']
const TYPES      = ['all', 'bundle', 'agent', 'tool', 'module']

// ── BundleCard ────────────────────────────────────────────────────────────────

function BundleCard({
  entry, installed, busy, onAdd, onRemove,
}: {
  entry: RegistryEntry
  installed: boolean
  busy: boolean
  onAdd: () => void
  onRemove: () => void
}) {
  return (
    <div className="flex flex-col bg-[var(--bg-sidebar)] border border-[var(--border)] rounded-lg p-3 hover:border-[var(--border-dark)] transition-colors">
      {/* Header */}
      <div className="flex items-start justify-between gap-2 mb-1.5">
        <div className="min-w-0">
          <span className="text-xs font-semibold text-[var(--text-primary)] block truncate">{entry.name}</span>
          <span className="text-[9px] text-[var(--text-very-muted)]">{entry.namespace}</span>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {entry.featured && (
            <span className="text-[8px] px-1 py-0.5 rounded bg-[var(--amber-subtle)] text-[var(--amber)]">featured</span>
          )}
          <span className={`text-[9px] px-1.5 py-0.5 rounded capitalize ${TYPE_COLORS[entry.type] ?? 'bg-[var(--bg-sidebar-active)] text-[var(--text-muted)]'}`}>
            {entry.type}
          </span>
        </div>
      </div>

      {/* Description */}
      <p className="text-[10px] text-[var(--text-muted)] leading-relaxed mb-2 flex-1 line-clamp-2">
        {entry.description}
      </p>

      {/* LLM verdict */}
      {entry.llmVerdict && (
        <p className="text-[9px] text-[var(--text-very-muted)] italic mb-2 line-clamp-2">
          "{entry.llmVerdict}"
        </p>
      )}

      {/* Rating + tags */}
      <div className="flex items-center justify-between gap-2 mb-2">
        <Stars rating={entry.rating} />
        <div className="flex gap-1 flex-wrap justify-end">
          {entry.tags.slice(0, 3).map(t => (
            <span key={t} className="text-[8px] px-1 py-0.5 rounded bg-[var(--bg-sidebar-active)] text-[var(--text-very-muted)]">
              {t}
            </span>
          ))}
        </div>
      </div>

      {/* Action */}
      <div className="flex items-center justify-between gap-2">
        <a
          href={entry.repo}
          target="_blank"
          rel="noopener noreferrer"
          className="text-[10px] text-[var(--amber)] hover:underline truncate font-mono"
          title={entry.repo}
        >
          {entry.repo.replace('https://github.com/', '')}
        </a>
        {installed ? (
          <button
            onClick={onRemove}
            disabled={busy}
            className="text-[10px] px-2 py-0.5 rounded bg-[#E53935]/10 text-[#E53935] hover:bg-[#E53935]/20 disabled:opacity-40 shrink-0"
          >
            Remove
          </button>
        ) : (
          <button
            onClick={onAdd}
            disabled={busy}
            className="text-[10px] px-2 py-0.5 rounded bg-[#4CAF74] hover:bg-[#43A047] text-white disabled:opacity-40 shrink-0"
          >
            {busy ? 'Adding…' : 'Add'}
          </button>
        )}
      </div>
    </div>
  )
}

// ── Main view ─────────────────────────────────────────────────────────────────

export default function BundlesView() {
  const [registry,  setRegistry]  = useState<RegistryEntry[]>([])
  const [installed, setInstalled] = useState<AppBundle[]>([])
  const [loading,   setLoading]   = useState(true)
  const [search,    setSearch]    = useState('')
  const [category,  setCategory]  = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [busy, setBusy]           = useState<Record<string, boolean>>({})
  const [toast, setToast]         = useState('')

  useEffect(() => {
    Promise.all([fetchRegistry(), listBundles()])
      .then(([reg, inst]) => { setRegistry(reg); setInstalled(inst) })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const installedIds = new Set(installed.map(b => b.id))

  const showToast = (msg: string) => {
    setToast(msg)
    setTimeout(() => setToast(''), 3000)
  }

  async function handleAdd(entry: RegistryEntry) {
    setBusy(b => ({ ...b, [entry.id]: true }))
    try {
      const bundle = await addBundle(entry)
      setInstalled(prev => [...prev, bundle])
      showToast(`✓ Added: ${entry.name}`)
    } catch (e: unknown) {
      showToast(`✗ ${(e as Error).message}`)
    } finally {
      setBusy(b => ({ ...b, [entry.id]: false }))
    }
  }

  async function handleRemove(id: string) {
    setBusy(b => ({ ...b, [id]: true }))
    try {
      await removeBundle(id)
      setInstalled(prev => prev.filter(b => b.id !== id))
      showToast('Bundle removed')
    } catch (e: unknown) {
      showToast(`✗ ${(e as Error).message}`)
    } finally {
      setBusy(b => ({ ...b, [id]: false }))
    }
  }

  async function handleToggle(id: string) {
    try {
      const updated = await toggleBundle(id)
      setInstalled(prev => prev.map(b => b.id === id ? updated : b))
    } catch (e: unknown) {
      showToast(`✗ ${(e as Error).message}`)
    }
  }

  // Filter registry
  const q = search.toLowerCase()
  const filtered = registry.filter(e => {
    if (category !== 'all' && e.category !== category) return false
    if (typeFilter !== 'all' && e.type !== typeFilter) return false
    if (q) {
      return e.name.toLowerCase().includes(q) ||
        e.description.toLowerCase().includes(q) ||
        e.namespace.toLowerCase().includes(q) ||
        e.tags.some(t => t.toLowerCase().includes(q))
    }
    return true
  })

  const featured = filtered.filter(e => e.featured && !installedIds.has(e.id))
  const rest     = filtered.filter(e => !e.featured && !installedIds.has(e.id))
  const installedEntries = filtered.filter(e => installedIds.has(e.id))

  return (
    <div className="flex h-full bg-[var(--bg-page)] overflow-hidden">

      {/* ── Left sidebar: installed bundles ────────────────────────────── */}
      <div className="w-60 shrink-0 flex flex-col border-r border-[var(--border)] overflow-hidden">
        <div className="flex items-center justify-between px-3 py-2 border-b border-[var(--border)]">
          <span className="text-[var(--text-muted)] text-[10px] uppercase tracking-wider">Installed</span>
          <span className="text-[10px] text-[var(--text-very-muted)]">{installed.length}</span>
        </div>

        {/* Install loom button */}
        <div className="px-3 py-2 border-b border-[var(--border)]">
          <button
            onClick={async () => {
              showToast('Running: amplifier bundle add …')
              try {
                const res = await fetch('/api/bundles', {
                  method: 'POST',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify({
                    id: 'amplifier-app-loom',
                    installSpec: 'git+https://github.com/kenotron-ms/amplifier-app-loom@main',
                    name: 'Loom',
                  }),
                })
                if (res.ok || res.status === 409) {
                  showToast('✓ Loom registered as app bundle')
                  const updated = await listBundles()
                  setInstalled(updated)
                }
              } catch { showToast('Run: loom bundle install') }
            }}
            className="w-full text-left text-[10px] px-2 py-1.5 rounded bg-[var(--bg-sidebar-active)] text-[var(--text-muted)] hover:bg-[var(--border-dark)] hover:text-[var(--text-primary)] transition-colors"
            title="Run: loom bundle install"
          >
            + Install loom as app bundle
          </button>
        </div>

        <div className="flex-1 overflow-y-auto">
          {installed.length === 0 && !loading && (
            <div className="px-3 py-4 text-[10px] text-[var(--text-very-muted)] text-center">
              No bundles installed yet.{'\n'}Browse the registry →
            </div>
          )}
          {installed.map(b => (
            <div key={b.id} className="flex items-center gap-2 px-3 py-2 border-b border-[var(--border)] group">
              <div className="flex-1 min-w-0">
                <div className="text-[11px] text-[var(--text-primary)] truncate">{b.name || b.id}</div>
                <div className="text-[9px] text-[var(--text-very-muted)] truncate">{b.installSpec}</div>
              </div>
              {/* Toggle */}
              <button
                onClick={() => handleToggle(b.id)}
                title={b.enabled ? 'Disable' : 'Enable'}
                className={`w-7 h-4 rounded-full transition-colors shrink-0 ${
                  b.enabled ? 'bg-[#4CAF74]' : 'bg-[var(--bg-sidebar-active)]'
                }`}
              >
                <div className={`w-3 h-3 rounded-full bg-white mx-auto transition-transform ${
                  b.enabled ? 'translate-x-1.5' : '-translate-x-1.5'
                }`} />
              </button>
              {/* Remove */}
              <button
                onClick={() => handleRemove(b.id)}
                className="opacity-0 group-hover:opacity-100 text-[var(--text-very-muted)] hover:text-[#E53935] text-xs shrink-0"
                title="Remove"
              >×</button>
            </div>
          ))}
        </div>
      </div>

      {/* ── Right: Registry browser ─────────────────────────────────────── */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Registry header */}
        <div className="flex items-center gap-3 px-4 py-2 border-b border-[var(--border)] bg-[var(--bg-sidebar)] shrink-0">
          <span className="text-[10px] text-[var(--text-very-muted)] uppercase tracking-wider">Amplifier Registry</span>
          <a
            href="https://kenotron-ms.github.io/amplifier-registry/"
            target="_blank"
            rel="noopener noreferrer"
            className="text-[11px] font-medium text-[#f0883e] hover:text-[#C4784A] hover:underline transition-colors"
          >
            🌐 Browse Site
          </a>
          <a
            href="https://github.com/kenotron-ms/amplifier-registry"
            target="_blank"
            rel="noopener noreferrer"
            className="text-[11px] font-medium text-[#F59E0B] hover:text-[#79c0ff] hover:underline transition-colors"
          >
            ⎇ GitHub Source
          </a>
        </div>

        {/* Search + filters */}
        <div className="px-4 py-3 border-b border-[var(--border)] shrink-0 space-y-2">
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search registry…"
            className="w-full px-3 py-1.5 text-xs bg-[var(--bg-input)] border border-[var(--border)] rounded text-[var(--text-primary)] placeholder:text-[var(--text-very-muted)] focus:outline-none focus:border-[var(--amber)]"
          />
          <div className="flex gap-4">
            {/* Category */}
            <div className="flex gap-1 flex-wrap">
              {CATEGORIES.map(c => (
                <button
                  key={c}
                  onClick={() => setCategory(c)}
                  className={`text-[9px] px-2 py-0.5 rounded capitalize ${
                    category === c
                      ? 'bg-[#388bfd]/20 text-[#F59E0B]'
                      : 'text-[var(--text-very-muted)] hover:text-[var(--text-muted)]'
                  }`}
                >
                  {c}
                </button>
              ))}
            </div>
            <div className="flex gap-1">
              {TYPES.map(t => (
                <button
                  key={t}
                  onClick={() => setTypeFilter(t)}
                  className={`text-[9px] px-2 py-0.5 rounded capitalize ${
                    typeFilter === t
                      ? `${TYPE_COLORS[t] ?? 'bg-[#388bfd]/20 text-[#F59E0B]'}`
                      : 'text-[var(--text-very-muted)] hover:text-[var(--text-muted)]'
                  }`}
                >
                  {t}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* Card grid */}
        <div className="flex-1 overflow-y-auto px-4 py-3 space-y-4">
          {loading && (
            <div className="flex items-center justify-center h-32 text-[var(--text-very-muted)] text-xs">
              Loading registry…
            </div>
          )}

          {/* Already installed — shown at top if matches filter */}
          {installedEntries.length > 0 && (
            <section>
              <h3 className="text-[10px] text-[var(--text-very-muted)] uppercase tracking-wider mb-2">Installed</h3>
              <div className="grid grid-cols-2 xl:grid-cols-3 gap-3">
                {installedEntries.map(e => (
                  <BundleCard
                    key={e.id}
                    entry={e}
                    installed={true}
                    busy={!!busy[e.id]}
                    onAdd={() => handleAdd(e)}
                    onRemove={() => handleRemove(e.id)}
                  />
                ))}
              </div>
            </section>
          )}

          {/* Featured */}
          {featured.length > 0 && (
            <section>
              <h3 className="text-[10px] text-[#F59E0B] uppercase tracking-wider mb-2">⭐ Featured</h3>
              <div className="grid grid-cols-2 xl:grid-cols-3 gap-3">
                {featured.map(e => (
                  <BundleCard
                    key={e.id}
                    entry={e}
                    installed={installedIds.has(e.id)}
                    busy={!!busy[e.id]}
                    onAdd={() => handleAdd(e)}
                    onRemove={() => handleRemove(e.id)}
                  />
                ))}
              </div>
            </section>
          )}

          {/* All others */}
          {rest.length > 0 && (
            <section>
              {featured.length > 0 && (
                <h3 className="text-[10px] text-[var(--text-very-muted)] uppercase tracking-wider mb-2">
                  Community ({rest.length})
                </h3>
              )}
              <div className="grid grid-cols-2 xl:grid-cols-3 gap-3">
                {rest.map(e => (
                  <BundleCard
                    key={e.id}
                    entry={e}
                    installed={installedIds.has(e.id)}
                    busy={!!busy[e.id]}
                    onAdd={() => handleAdd(e)}
                    onRemove={() => handleRemove(e.id)}
                  />
                ))}
              </div>
            </section>
          )}

          {!loading && filtered.length === 0 && (
            <div className="flex items-center justify-center h-32 text-[var(--text-very-muted)] text-xs">
              No bundles match your filters.
            </div>
          )}
        </div>
      </div>

      {/* Toast */}
      {toast && (
        <div className="fixed bottom-4 right-4 z-50 px-3 py-2 bg-[var(--bg-modal)] border border-[var(--border)] rounded text-xs text-[var(--text-primary)] shadow-lg">
          {toast}
        </div>
      )}
    </div>
  )
}
