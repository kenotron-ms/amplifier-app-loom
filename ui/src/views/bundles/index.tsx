import { useEffect, useState } from 'react'
import ResizableSidebar from '../../components/ResizableSidebar'
import {
  RegistryEntry, IndexStatus, AmplifierBundle,
  fetchRegistry, fetchLocalRegistry, addBundle,
  getIndexStatus, triggerScan, addScanJob, removeScanJob,
  fetchAmplifierBundles, enableBundleApp, disableBundleApp, removeAmplifierBundle,
  setActiveBundle, clearActiveBundle,
} from '../../api/bundles'

// ── helpers ───────────────────────────────────────────────────────────────────

const TYPE_COLORS: Record<string, string> = {
  bundle:   'bg-[#388bfd]/20 text-[#F59E0B]',
  agent:    'bg-[#7C3F2A]/20 text-[#7C3F2A]',
  tool:     'bg-[#3fb950]/20 text-[#56d364]',
  module:   'bg-[#C4784A]/20 text-[#C4784A]',
  behavior: 'bg-[#5c2d91]/20 text-[#b392f0]',
  recipe:   'bg-[#0d419d]/20 text-[#79c0ff]',
  package:  'bg-[#264f78]/20 text-[#79c0ff]',
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
          {entry.private && (
            <span className="text-[8px] px-1 py-0.5 rounded bg-[#30363d] text-[#8b949e]" title={entry.localPath}>🔒 private</span>
          )}
          {!entry.private && entry.featured && (
            <span className="text-[8px] px-1 py-0.5 rounded bg-[var(--amber-subtle)] text-[var(--amber)]">featured</span>
          )}
          {installed && (
            <span className="text-[8px] px-1 py-0.5 rounded bg-[#14b8a6]/20 text-[#14b8a6]">installed</span>
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
          {(entry.tags ?? []).slice(0, 3).map(t => (
            <span key={t} className="text-[8px] px-1 py-0.5 rounded bg-[var(--bg-sidebar-active)] text-[var(--text-very-muted)]">
              {t}
            </span>
          ))}
        </div>
      </div>

      {/* Action */}
      <div className="flex items-center justify-between gap-2">
        <a
          href={entry.repo ?? '#'}
          target="_blank"
          rel="noopener noreferrer"
          className="text-[10px] text-[var(--amber)] hover:underline truncate font-mono"
          title={entry.repo ?? ''}
        >
          {(entry.repo ?? '').replace('https://github.com/', '')}
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
  const [localRegistry, setLocalRegistry] = useState<RegistryEntry[]>([])
  const [loading,   setLoading]   = useState(true)
  const [search,    setSearch]    = useState('')
  const [category,  setCategory]  = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [busy, setBusy]           = useState<Record<string, boolean>>({})
  const [toast, setToast]         = useState('')
  const [error, setError]         = useState<string | null>(null)
  const [indexStatus, setIndexStatus] = useState<IndexStatus | null>(null)
  const [scanBusy, setScanBusy]   = useState(false)
  const [watchBusy, setWatchBusy] = useState(false)
  const [ampBundles, setAmpBundles] = useState<AmplifierBundle[]>([])
  const [installedFilter, setInstalledFilter] = useState(false)
  const [sidebarBusy, setSidebarBusy] = useState<Record<string, boolean>>({})

  const refreshAmpBundles = () =>
    fetchAmplifierBundles().then(setAmpBundles).catch(() => {})

  useEffect(() => {
    Promise.all([fetchRegistry(), fetchLocalRegistry(), getIndexStatus(), fetchAmplifierBundles()])
      .then(([reg, local, status, amp]) => {
        setRegistry(reg)
        setLocalRegistry(local)
        setIndexStatus(status)
        setAmpBundles(amp)
      })
      .catch((e: unknown) => {
        console.error(e)
        setError((e as Error).message ?? 'Failed to load registry')
      })
      .finally(() => setLoading(false))
  }, [])

  // Poll index status while a scan is running
  useEffect(() => {
    if (!indexStatus?.scanning) return
    const id = setInterval(async () => {
      try {
        const s = await getIndexStatus()
        setIndexStatus(s)
        if (!s.scanning) {
          clearInterval(id)
          fetchLocalRegistry().then(setLocalRegistry).catch(() => {})
        }
      } catch { /* ignore */ }
    }, 2000)
    return () => clearInterval(id)
  }, [indexStatus?.scanning])

  // Match registry entries to ampBundles by URI (spec extracted from entry.install)
  const addedUris = new Set(ampBundles.map(b => b.uri.trim()))
  const ampByUri  = new Map(ampBundles.map(b => [b.uri.trim(), b]))
  const specFromEntry = (e: RegistryEntry) => e.install.replace(/^amplifier bundle add\s+/, '').trim()
  const isInstalled   = (e: RegistryEntry) => addedUris.has(specFromEntry(e))


  const showToast = (msg: string) => {
    setToast(msg)
    setTimeout(() => setToast(''), 3000)
  }

  async function handleAdd(entry: RegistryEntry) {
    setBusy(b => ({ ...b, [entry.id]: true }))
    try {
      await addBundle(entry)
      showToast(`✓ Added: ${entry.name}`)
      await refreshAmpBundles()
    } catch (e: unknown) {
      showToast(`✗ ${(e as Error).message}`)
    } finally {
      setBusy(b => ({ ...b, [entry.id]: false }))
    }
  }

  async function handleRemove(id: string, entry?: RegistryEntry) {
    setBusy(b => ({ ...b, [id]: true }))
    try {
      if (entry) {
        const spec = entry.install.replace(/^amplifier bundle add\s+/, '').trim()
        const amp  = ampByUri.get(spec)
        if (amp) {
          await removeAmplifierBundle(amp.name)
        } else {
          await removeAmplifierBundle(id)
        }
      } else {
        await removeAmplifierBundle(id)
      }
      showToast('Bundle removed')
      await refreshAmpBundles()
    } catch (e: unknown) {
      showToast(`✗ ${(e as Error).message}`)
    } finally {
      setBusy(b => ({ ...b, [id]: false }))
    }
  }


  async function handleScan() {
    setScanBusy(true)
    try {
      await triggerScan()
      const s = await getIndexStatus()
      setIndexStatus(s)
      showToast('Scan started')
    } catch (e: unknown) {
      showToast(`✗ ${(e as Error).message}`)
    } finally {
      setScanBusy(false)
    }
  }

  const withSidebarBusy = (name: string, fn: () => Promise<void>) => async () => {
    setSidebarBusy(s => ({ ...s, [name]: true }))
    try { await fn() } catch (e: unknown) { showToast(`✗ ${(e as Error).message}`) }
    finally { setSidebarBusy(s => ({ ...s, [name]: false })) }
  }

  async function handleAppToggle(b: AmplifierBundle) {
    await withSidebarBusy(b.name, async () => {
      if (b.appEnabled) {
        await disableBundleApp(b.appSpec)
      } else {
        await enableBundleApp(b.appSpec)
      }
      await refreshAmpBundles()
    })()
  }

  async function handleRemoveAmp(name: string) {
    await withSidebarBusy(name, async () => {
      await removeAmplifierBundle(name)
      await refreshAmpBundles()
      showToast('Bundle removed')
    })()
  }

  async function handleSetActive(name: string) {
    await withSidebarBusy(name, async () => {
      await setActiveBundle(name)
      await refreshAmpBundles()
    })()
  }

  async function handleClearActive() {
    const active = ampBundles.find(b => b.active)
    if (!active) return
    await withSidebarBusy(active.name, async () => {
      await clearActiveBundle()
      await refreshAmpBundles()
    })()
  }

  async function handleWatchToggle() {
    setWatchBusy(true)
    try {
      if (indexStatus?.watchJobId) {
        await removeScanJob()
        showToast('Scan schedule removed')
      } else {
        await addScanJob('2h')
        showToast('Scan scheduled every 2h')
      }
      const s = await getIndexStatus()
      setIndexStatus(s)
    } catch (e: unknown) {
      showToast(`✗ ${(e as Error).message}`)
    } finally {
      setWatchBusy(false)
    }
  }

  // Filter registry
  const q = search.toLowerCase()
  const matchEntry = (e: RegistryEntry) => {
    if (typeFilter !== 'all' && e.type !== typeFilter) return false
    if (installedFilter && !isInstalled(e)) return false
    if (q) {
      return e.name.toLowerCase().includes(q) ||
        (e.description ?? '').toLowerCase().includes(q) ||
        (e.namespace ?? '').toLowerCase().includes(q) ||
        (e.tags ?? []).some(t => t.toLowerCase().includes(q))
    }
    return true
  }
  const filtered = registry.filter(e => {
    if (category !== 'all' && e.category !== category) return false
    return matchEntry(e)
  })
  const localFiltered = localRegistry.filter(matchEntry)

  const featured      = filtered.filter(e => e.featured)
  const rest          = filtered.filter(e => !e.featured)
  const privateEntries = localFiltered

  if (error) {
    return (
      <div className="flex h-full items-center justify-center bg-[var(--bg-page)]">
        <div className="text-center space-y-2">
          <p className="text-xs text-[var(--text-muted)]">Failed to load registry</p>
          <p className="text-[10px] text-[var(--text-very-muted)] font-mono">{error}</p>
          <button
            onClick={() => { setError(null); setLoading(true); Promise.all([fetchRegistry(), fetchAmplifierBundles()]).then(([reg, amp]) => { setRegistry(reg); setAmpBundles(amp) }).catch((e: unknown) => { setError((e as Error).message ?? 'Failed to load registry') }).finally(() => setLoading(false)) }}
            className="text-[10px] px-3 py-1 rounded bg-[var(--bg-sidebar-active)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex h-full bg-[var(--bg-page)] overflow-hidden">

      {/* ── Left sidebar: bundles from ~/.amplifier/settings.yaml ─────────────── */}
      <ResizableSidebar defaultWidth={240}>
      <div className="flex flex-col border-r border-[var(--border)] overflow-hidden h-full">
        <div className="flex items-center justify-between px-3 py-2 border-b border-[var(--border)]">
          <span className="text-[var(--text-muted)] text-[10px] uppercase tracking-wider">Installed</span>
          <div className="flex items-center gap-2">
            {ampBundles.some(b => b.active) && (
              <button
                onClick={handleClearActive}
                className="text-[9px] text-[var(--text-very-muted)] hover:text-[#E53935] transition-colors"
                title="Clear active bundle (falls back to foundation)"
              >
                clear active
              </button>
            )}
            <span className="text-[10px] text-[var(--text-very-muted)]">{ampBundles.length}</span>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          {ampBundles.length === 0 && !loading && (
            <div className="px-3 py-4 text-[10px] text-[var(--text-very-muted)] text-center">
              No bundles in settings.yaml yet.
            </div>
          )}
          {ampBundles.map(b => {
            const busy = !!sidebarBusy[b.name]
            return (
            <div
              key={b.name}
              className={`flex items-center gap-2 px-3 py-2 border-b border-[var(--border)] group transition-opacity ${
                busy ? 'opacity-60' : !b.downloaded ? 'opacity-50' : ''
              }`}
            >
              {/* Toggle — LEFT, always visible */}
              <button
                onClick={() => handleAppToggle(b)}
                disabled={busy}
                title={b.appEnabled ? 'Disable as app overlay' : 'Enable as app overlay'}
                className={`w-7 h-4 rounded-full transition-colors shrink-0 disabled:cursor-wait ${
                  b.appEnabled ? 'bg-[#4CAF74]' : 'bg-[var(--bg-sidebar-active)]'
                }`}
              >
                <div className={`w-3 h-3 rounded-full bg-white mx-auto transition-transform ${
                  b.appEnabled ? 'translate-x-1.5' : '-translate-x-1.5'
                }`} />
              </button>

              {/* Name + URI */}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-1 mb-0.5">
                  <span className="text-[11px] text-[var(--text-primary)] truncate">{b.name}</span>
                  {b.version && (
                    <span className="text-[9px] text-[var(--text-very-muted)] shrink-0">v{b.version}</span>
                  )}
                </div>
                <div className="flex items-center text-[9px] text-[var(--text-very-muted)] min-w-0">
                  <span className="truncate" title={b.uri}>
                    {b.uri.replace(/^(git\+https?:\/\/github\.com\/)|(file:\/\/)/, '').replace(/@[^@]*$/, '')}
                  </span>
                  {b.active && (
                    <span className="shrink-0 ml-1.5 text-[#14b8a6]/70">· active</span>
                  )}
                </div>
              </div>

              {/* Circle action buttons — RIGHT, show on hover (or while busy) */}
              <div className={`flex items-center gap-1 shrink-0 transition-opacity ${
                busy ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
              }`}>
                {!b.active && (
                  <button
                    onClick={() => handleSetActive(b.name)}
                    disabled={busy}
                    title="Set as active bundle"
                    className="w-5 h-5 rounded-full border border-[#14b8a6]/60 flex items-center justify-center pl-px text-[#14b8a6] hover:bg-[#14b8a6]/20 disabled:cursor-wait transition-colors"
                  >
                    {busy
                      ? <span className="text-[9px] animate-spin inline-block">⟳</span>
                      : <svg width="7" height="8" viewBox="0 0 7 8" fill="currentColor"><polygon points="0,0 7,4 0,8"/></svg>
                    }
                  </button>
                )}
                <button
                  onClick={() => handleRemoveAmp(b.name)}
                  disabled={busy}
                  title="Remove bundle"
                  className="w-5 h-5 rounded-full border border-[#E53935]/60 flex items-center justify-center text-[#E53935] hover:bg-[#E53935]/20 disabled:cursor-wait transition-colors"
                >
                  <svg width="8" height="8" viewBox="0 0 8 8" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                    <line x1="1.5" y1="1.5" x2="6.5" y2="6.5"/>
                    <line x1="6.5" y1="1.5" x2="1.5" y2="6.5"/>
                  </svg>
                </button>
              </div>
            </div>
          )})}

        </div>
      </div>
      </ResizableSidebar>

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

        {/* Index scan toolbar */}
        <div className="flex items-center gap-3 px-4 py-2 border-b border-[var(--border)] shrink-0 bg-[var(--bg-page)]">
          {/* Status */}
          <div className="flex items-center gap-2 flex-1 min-w-0">
            {indexStatus ? (
              <>
                <span className={`text-[9px] px-1.5 py-0.5 rounded ${
                  indexStatus.scanning
                    ? 'bg-[var(--amber-subtle)] text-[var(--amber)]'
                    : 'bg-[var(--bg-sidebar-active)] text-[var(--text-very-muted)]'
                }`}>
                  {indexStatus.scanning ? 'scanning…' : indexStatus.repoCount > 0 ? `${indexStatus.repoCount} repos` : 'not scanned'}
                </span>
                {indexStatus.lastScan && !indexStatus.scanning && (
                  <span className="text-[10px] text-[var(--text-very-muted)] truncate">
                    last: {new Date(indexStatus.lastScan).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}
                  </span>
                )}
              </>
            ) : (
              <span className="text-[10px] text-[var(--text-very-muted)]">GitHub index</span>
            )}
          </div>

          {/* Scan now */}
          <button
            onClick={handleScan}
            disabled={scanBusy || indexStatus?.scanning}
            className="text-[10px] px-2.5 py-1 rounded border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:border-[var(--border-dark)] disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {scanBusy || indexStatus?.scanning ? '⟳ scanning…' : '⟳ Scan now'}
          </button>

          {/* Watch toggle */}
          <button
            onClick={handleWatchToggle}
            disabled={watchBusy}
            className={`text-[10px] px-2.5 py-1 rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${
              indexStatus?.watchJobId
                ? 'border-[var(--amber)] text-[var(--amber)] hover:border-[#C4784A] hover:text-[#C4784A]'
                : 'border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:border-[var(--border-dark)]'
            }`}
            title={indexStatus?.watchJobId ? 'Remove scheduled scan' : 'Schedule scan every 2h'}
          >
            {indexStatus?.watchJobId ? '⏱ watching' : '+ Add scan job'}
          </button>
        </div>

        {/* Search + filters */}
        <div className="px-4 py-3 border-b border-[var(--border)] shrink-0 space-y-2">
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search registry…"
            className="w-full px-3 py-1.5 text-xs bg-[var(--bg-input)] border border-[var(--border)] rounded text-[var(--text-primary)] placeholder:text-[var(--text-very-muted)] focus:outline-none focus:border-[var(--amber)]"
          />
          <div className="flex gap-4 flex-wrap">
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
            {/* Installed filter */}
            <button
              onClick={() => setInstalledFilter(f => !f)}
              className={`text-[9px] px-2 py-0.5 rounded ${
                installedFilter
                  ? 'bg-[#14b8a6]/20 text-[#14b8a6]'
                  : 'text-[var(--text-very-muted)] hover:text-[var(--text-muted)]'
              }`}
            >
              installed
            </button>
          </div>
        </div>

        {/* Card grid */}
        <div className="flex-1 overflow-y-auto px-4 py-3 space-y-4">
          {loading && (
            <div className="flex items-center justify-center h-32 text-[var(--text-very-muted)] text-xs">
              Loading registry…
            </div>
          )}

          {/* Private / local */}
          {privateEntries.length > 0 && (
            <section>
              <h3 className="text-[10px] text-[#8b949e] uppercase tracking-wider mb-2">🔒 Private ({privateEntries.length})</h3>
              <div className="grid grid-cols-2 xl:grid-cols-3 gap-3">
                {privateEntries.map(e => (
                  <BundleCard
                    key={e.id}
                    entry={e}
                    installed={isInstalled(e)}
                    busy={!!busy[e.id]}
                    onAdd={() => handleAdd(e)}
                    onRemove={() => handleRemove(e.id, e)}
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
                    installed={isInstalled(e)}
                    busy={!!busy[e.id]}
                    onAdd={() => handleAdd(e)}
                    onRemove={() => handleRemove(e.id, e)}
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
                    installed={isInstalled(e)}
                    busy={!!busy[e.id]}
                    onAdd={() => handleAdd(e)}
                    onRemove={() => handleRemove(e.id, e)}
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
