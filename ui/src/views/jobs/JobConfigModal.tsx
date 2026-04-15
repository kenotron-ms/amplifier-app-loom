import { CSSProperties, useEffect, useState } from 'react'
import { Job, getJob, updateJob } from '../../api/jobs'

interface Props {
  job: Job
  onClose: () => void
  onSaved: (updated: Job) => void
}

// ── Style tokens ──────────────────────────────────────────────────────────────

const field: CSSProperties = {
  width: '100%',
  fontSize: 12,
  padding: '5px 8px',
  background: 'var(--bg-terminal)',
  border: '1px solid var(--border)',
  borderRadius: 3,
  color: 'var(--text-primary)',
  outline: 'none',
  boxSizing: 'border-box',
}

const monoField: CSSProperties = {
  ...field,
  fontFamily: 'var(--font-mono)',
  fontSize: 11.5,
  resize: 'vertical',
  lineHeight: 1.55,
}

const sel: CSSProperties = { ...field, cursor: 'pointer' }

const lbl: CSSProperties = {
  display: 'block',
  fontSize: 11,
  fontWeight: 500,
  color: 'var(--text-muted)',
  marginBottom: 3,
}

const sectionHead: CSSProperties = {
  fontSize: 10,
  fontWeight: 600,
  color: 'var(--text-very-muted)',
  textTransform: 'uppercase',
  letterSpacing: '0.08em',
  paddingBottom: 6,
  borderBottom: '1px solid var(--border)',
  marginBottom: 8,
}

const row: CSSProperties = { display: 'flex', flexDirection: 'column', gap: 3 }

// ── Key-value helpers for amplifier context ───────────────────────────────────

type KV = { key: string; value: string }

function kvFrom(rec?: Record<string, string>): KV[] {
  if (!rec) return []
  return Object.entries(rec).map(([key, value]) => ({ key, value }))
}

function kvTo(pairs: KV[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const { key, value } of pairs) if (key.trim()) out[key.trim()] = value
  return out
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function JobConfigModal({ job, onClose, onSaved }: Props) {
  const [loading, setLoading] = useState(true)
  const [saving, setSaving]   = useState(false)
  const [error, setError]     = useState<string | null>(null)

  // General
  const [name, setName]             = useState('')
  const [description, setDesc]      = useState('')
  const [enabled, setEnabled]       = useState(true)
  const [cwd, setCwd]               = useState('')
  const [jobTimeout, setJobTimeout] = useState('')
  const [maxRetries, setMaxRetries] = useState(0)

  // Trigger
  const [triggerType, setTriggerType]       = useState('loop')
  const [schedule, setSchedule]             = useState('')
  const [watchPath, setWatchPath]           = useState('')
  const [watchRecursive, setWatchRecursive] = useState(false)
  const [watchMode, setWatchMode]           = useState('notify')
  const [watchDebounce, setWatchDebounce]   = useState('')
  const [connectorId, setConnectorId]       = useState('')

  // Executor
  const [executor, setExecutor] = useState('shell')

  // Shell
  const [shellCommand, setShellCommand] = useState('')

  // Claude Code
  const [ccPrompt, setCcPrompt]                       = useState('')
  const [ccModel, setCcModel]                         = useState('')
  const [ccMaxTurns, setCcMaxTurns]                   = useState('')
  const [ccSteps, setCcSteps]                         = useState('')
  const [ccAllowedTools, setCcAllowedTools]           = useState('')
  const [ccAppendSystemPrompt, setCcAppendSP]         = useState('')

  // Amplifier
  const [ampPrompt, setAmpPrompt]         = useState('')
  const [ampRecipePath, setAmpRecipePath] = useState('')
  const [ampBundle, setAmpBundle]         = useState('')
  const [ampModel, setAmpModel]           = useState('')
  const [ampSteps, setAmpSteps]           = useState('')
  const [ampContext, setAmpContext]       = useState<KV[]>([])

  // ── Load ────────────────────────────────────────────────────────────────────

  useEffect(() => {
    setLoading(true)
    getJob(job.id)
      .then(full => {
        setName(full.name ?? '')
        setDesc(full.description ?? '')
        setEnabled(full.enabled ?? true)
        setCwd(full.cwd ?? '')
        setJobTimeout(full.timeout ?? '')
        setMaxRetries(full.maxRetries ?? 0)

        setTriggerType(full.trigger?.type ?? 'loop')
        setSchedule(full.trigger?.schedule ?? '')

        if (full.watch) {
          setWatchPath(full.watch.path ?? '')
          setWatchRecursive(full.watch.recursive ?? false)
          setWatchMode(full.watch.mode ?? 'notify')
          setWatchDebounce(full.watch.debounce ?? '')
        }
        if (full.connector) setConnectorId(full.connector.connectorId ?? '')

        setExecutor(full.executor ?? 'shell')

        if (full.shell)       setShellCommand(full.shell.command ?? '')
        if (full.claudeCode) {
          setCcPrompt(full.claudeCode.prompt ?? '')
          setCcModel(full.claudeCode.model ?? '')
          setCcMaxTurns(full.claudeCode.maxTurns != null ? String(full.claudeCode.maxTurns) : '')
          setCcSteps((full.claudeCode.steps ?? []).join('\n'))
          setCcAllowedTools((full.claudeCode.allowedTools ?? []).join('\n'))
          setCcAppendSP(full.claudeCode.appendSystemPrompt ?? '')
        }
        if (full.amplifier) {
          setAmpPrompt(full.amplifier.prompt ?? '')
          setAmpRecipePath(full.amplifier.recipePath ?? '')
          setAmpBundle(full.amplifier.bundle ?? '')
          setAmpModel(full.amplifier.model ?? '')
          setAmpSteps((full.amplifier.steps ?? []).join('\n'))
          setAmpContext(kvFrom(full.amplifier.context))
        }
        setLoading(false)
      })
      .catch(e => { setError(String(e)); setLoading(false) })
  }, [job.id])

  // ── Save ────────────────────────────────────────────────────────────────────

  const handleSave = async () => {
    setError(null)
    const lines = (s: string) => s.split('\n').map(l => l.trim()).filter(Boolean)

    const updates: Partial<Job> = {
      name,
      description,
      enabled,
      cwd,
      timeout: jobTimeout,
      maxRetries: Number(maxRetries),
      trigger: { type: triggerType, schedule },
      executor,
    }

    if (triggerType === 'watch') {
      updates.watch = {
        path: watchPath,
        recursive: watchRecursive,
        mode: watchMode,
        ...(watchDebounce && { debounce: watchDebounce }),
      }
    }
    if (triggerType === 'connector') updates.connector = { connectorId }

    if (executor === 'shell') {
      updates.shell = { command: shellCommand }
    } else if (executor === 'claude-code') {
      updates.claudeCode = {
        prompt: ccPrompt,
        ...(ccModel      && { model: ccModel }),
        ...(ccMaxTurns   && { maxTurns: Number(ccMaxTurns) }),
        ...(ccSteps      && { steps: lines(ccSteps) }),
        ...(ccAllowedTools && { allowedTools: lines(ccAllowedTools) }),
        ...(ccAppendSystemPrompt && { appendSystemPrompt: ccAppendSystemPrompt }),
      }
    } else if (executor === 'amplifier') {
      const ctx = kvTo(ampContext)
      updates.amplifier = {
        ...(ampPrompt     && { prompt: ampPrompt }),
        ...(ampRecipePath && { recipePath: ampRecipePath }),
        ...(ampBundle     && { bundle: ampBundle }),
        ...(ampModel      && { model: ampModel }),
        ...(ampSteps      && { steps: lines(ampSteps) }),
        ...(Object.keys(ctx).length && { context: ctx }),
      }
    }

    setSaving(true)
    try {
      const updated = await updateJob(job.id, updates)
      onSaved(updated)
    } catch (e) {
      setError(String(e))
      setSaving(false)
    }
  }

  useEffect(() => {
    const fn = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', fn)
    return () => window.removeEventListener('keydown', fn)
  }, [onClose])

  // ── Render ───────────────────────────────────────────────────────────────────

  return (
    <>
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.55)', zIndex: 1000 }}
      />

      {/* Modal */}
      <div style={{
        position: 'fixed', top: '50%', left: '50%',
        transform: 'translate(-50%, -50%)',
        width: 740, maxHeight: '90vh',
        display: 'flex', flexDirection: 'column',
        background: 'var(--bg-modal)',
        border: '1px solid var(--border-dark)',
        borderRadius: 6, zIndex: 1001,
        boxShadow: '0 24px 64px rgba(0,0,0,0.55)',
      }}>

        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center',
          padding: '0 16px', height: 40, flexShrink: 0,
          borderBottom: '1px solid var(--border)',
        }}>
          <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--text-primary)' }}>
            Edit — {job.name}
          </span>
          <button onClick={onClose} style={{
            marginLeft: 'auto', fontSize: 18, lineHeight: 1,
            background: 'none', border: 'none',
            color: 'var(--text-muted)', cursor: 'pointer', padding: '0 2px',
          }}>×</button>
        </div>

        {/* Scrollable form body */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '16px 20px', display: 'flex', flexDirection: 'column', gap: 22 }}>
          {loading ? (
            <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>Loading…</span>
          ) : (
            <>

              {/* ── General ──────────────────────────────────── */}
              <section style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div style={sectionHead}>General</div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 120px', gap: 10 }}>
                  <div style={row}>
                    <label style={lbl}>Name</label>
                    <input style={field} value={name} onChange={e => setName(e.target.value)} />
                  </div>
                  <div style={row}>
                    <label style={lbl}>Enabled</label>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 7, paddingTop: 6, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={enabled}
                        onChange={e => setEnabled(e.target.checked)}
                        style={{ cursor: 'pointer', accentColor: 'var(--amber)' }}
                      />
                      <span style={{ fontSize: 12, color: enabled ? 'var(--text-primary)' : 'var(--text-muted)' }}>
                        {enabled ? 'Active' : 'Paused'}
                      </span>
                    </label>
                  </div>
                </div>

                <div style={row}>
                  <label style={lbl}>Description <span style={{ fontWeight: 400, opacity: 0.6 }}>(optional)</span></label>
                  <input style={field} value={description} onChange={e => setDesc(e.target.value)} placeholder="What does this job do?" />
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 90px 90px', gap: 10 }}>
                  <div style={row}>
                    <label style={lbl}>Working Directory</label>
                    <input style={field} value={cwd} onChange={e => setCwd(e.target.value)} placeholder="/path/to/dir or leave blank" />
                  </div>
                  <div style={row}>
                    <label style={lbl}>Timeout</label>
                    <input style={field} value={jobTimeout} onChange={e => setJobTimeout(e.target.value)} placeholder="30s" />
                  </div>
                  <div style={row}>
                    <label style={lbl}>Max Retries</label>
                    <input style={field} type="number" min={0} value={maxRetries} onChange={e => setMaxRetries(Number(e.target.value))} />
                  </div>
                </div>
              </section>

              {/* ── Trigger ──────────────────────────────────── */}
              <section style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div style={sectionHead}>Trigger</div>

                <div style={{ display: 'grid', gridTemplateColumns: '180px 1fr', gap: 10, alignItems: 'start' }}>
                  <div style={row}>
                    <label style={lbl}>Type</label>
                    <select style={sel} value={triggerType} onChange={e => setTriggerType(e.target.value)}>
                      <option value="loop">Loop (interval)</option>
                      <option value="cron">Cron</option>
                      <option value="once">Once</option>
                      <option value="watch">File Watch</option>
                      <option value="connector">Connector</option>
                    </select>
                  </div>

                  {(triggerType === 'loop' || triggerType === 'cron') && (
                    <div style={row}>
                      <label style={lbl}>
                        {triggerType === 'loop' ? 'Interval' : 'Cron Expression'}
                      </label>
                      <input
                        style={field}
                        value={schedule}
                        onChange={e => setSchedule(e.target.value)}
                        placeholder={triggerType === 'loop' ? '30s · 5m · 1h' : '0 * * * *'}
                      />
                    </div>
                  )}

                  {triggerType === 'once' && (
                    <div style={{ paddingTop: 20 }}>
                      <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                        Runs once, then auto-disables.
                      </span>
                    </div>
                  )}

                  {triggerType === 'connector' && (
                    <div style={row}>
                      <label style={lbl}>Connector ID</label>
                      <input style={field} value={connectorId} onChange={e => setConnectorId(e.target.value)} placeholder="connector-id" />
                    </div>
                  )}
                </div>

                {triggerType === 'watch' && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 2 }}>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 110px 100px', gap: 10 }}>
                      <div style={row}>
                        <label style={lbl}>Watch Path</label>
                        <input style={field} value={watchPath} onChange={e => setWatchPath(e.target.value)} placeholder="/path/to/watch" />
                      </div>
                      <div style={row}>
                        <label style={lbl}>Mode</label>
                        <select style={sel} value={watchMode} onChange={e => setWatchMode(e.target.value)}>
                          <option value="notify">notify</option>
                          <option value="poll">poll</option>
                        </select>
                      </div>
                      <div style={row}>
                        <label style={lbl}>Debounce</label>
                        <input style={field} value={watchDebounce} onChange={e => setWatchDebounce(e.target.value)} placeholder="500ms" />
                      </div>
                    </div>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 7, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={watchRecursive}
                        onChange={e => setWatchRecursive(e.target.checked)}
                        style={{ cursor: 'pointer', accentColor: 'var(--amber)' }}
                      />
                      <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Recursive (watch subdirectories)</span>
                    </label>
                  </div>
                )}
              </section>

              {/* ── Executor ─────────────────────────────────── */}
              <section style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div style={sectionHead}>Executor</div>

                <div style={row}>
                  <label style={lbl}>Type</label>
                  <select style={{ ...sel, maxWidth: 200 }} value={executor} onChange={e => setExecutor(e.target.value)}>
                    <option value="shell">Shell</option>
                    <option value="claude-code">Claude Code</option>
                    <option value="amplifier">Amplifier</option>
                  </select>
                </div>

                {/* Shell ---------------------------------------------------- */}
                {executor === 'shell' && (
                  <div style={row}>
                    <label style={lbl}>Command</label>
                    <textarea
                      style={{ ...monoField, minHeight: 80 }}
                      value={shellCommand}
                      onChange={e => setShellCommand(e.target.value)}
                      placeholder="echo 'hello world'"
                      spellCheck={false}
                    />
                  </div>
                )}

                {/* Claude Code ---------------------------------------------- */}
                {executor === 'claude-code' && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                    <div style={row}>
                      <label style={lbl}>Prompt</label>
                      <textarea
                        style={{ ...field, resize: 'vertical', minHeight: 88, lineHeight: 1.55 }}
                        value={ccPrompt}
                        onChange={e => setCcPrompt(e.target.value)}
                        placeholder="Describe what Claude should do…"
                      />
                    </div>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 90px', gap: 10 }}>
                      <div style={row}>
                        <label style={lbl}>Model <span style={{ fontWeight: 400, opacity: 0.6 }}>(optional)</span></label>
                        <input style={field} value={ccModel} onChange={e => setCcModel(e.target.value)} placeholder="claude-opus-4-5" />
                      </div>
                      <div style={row}>
                        <label style={lbl}>Max Turns</label>
                        <input style={field} type="number" min={1} value={ccMaxTurns} onChange={e => setCcMaxTurns(e.target.value)} placeholder="∞" />
                      </div>
                    </div>
                    <div style={row}>
                      <label style={lbl}>Steps <span style={{ fontWeight: 400, opacity: 0.6 }}>(one per line, optional)</span></label>
                      <textarea
                        style={{ ...monoField, minHeight: 60 }}
                        value={ccSteps}
                        onChange={e => setCcSteps(e.target.value)}
                        placeholder={"Run tests\nCheck lint\nReport results"}
                        spellCheck={false}
                      />
                    </div>
                    <div style={row}>
                      <label style={lbl}>Allowed Tools <span style={{ fontWeight: 400, opacity: 0.6 }}>(one per line, optional)</span></label>
                      <textarea
                        style={{ ...monoField, minHeight: 52 }}
                        value={ccAllowedTools}
                        onChange={e => setCcAllowedTools(e.target.value)}
                        placeholder={"Bash\nRead\nWrite"}
                        spellCheck={false}
                      />
                    </div>
                    <div style={row}>
                      <label style={lbl}>Append System Prompt <span style={{ fontWeight: 400, opacity: 0.6 }}>(optional)</span></label>
                      <textarea
                        style={{ ...field, resize: 'vertical', minHeight: 52, lineHeight: 1.55 }}
                        value={ccAppendSystemPrompt}
                        onChange={e => setCcAppendSP(e.target.value)}
                        placeholder="Additional system-level instructions…"
                      />
                    </div>
                  </div>
                )}

                {/* Amplifier ------------------------------------------------ */}
                {executor === 'amplifier' && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                    <div style={row}>
                      <label style={lbl}>Prompt <span style={{ fontWeight: 400, opacity: 0.6 }}>(or use Recipe Path)</span></label>
                      <textarea
                        style={{ ...field, resize: 'vertical', minHeight: 88, lineHeight: 1.55 }}
                        value={ampPrompt}
                        onChange={e => setAmpPrompt(e.target.value)}
                        placeholder="Describe what Amplifier should do…"
                      />
                    </div>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                      <div style={row}>
                        <label style={lbl}>Recipe Path <span style={{ fontWeight: 400, opacity: 0.6 }}>(optional)</span></label>
                        <input style={field} value={ampRecipePath} onChange={e => setAmpRecipePath(e.target.value)} placeholder="@recipes:my-recipe.yaml" />
                      </div>
                      <div style={row}>
                        <label style={lbl}>Bundle <span style={{ fontWeight: 400, opacity: 0.6 }}>(optional)</span></label>
                        <input style={field} value={ampBundle} onChange={e => setAmpBundle(e.target.value)} placeholder="my-bundle" />
                      </div>
                    </div>
                    <div style={row}>
                      <label style={lbl}>Model <span style={{ fontWeight: 400, opacity: 0.6 }}>(optional)</span></label>
                      <input style={{ ...field, maxWidth: 260 }} value={ampModel} onChange={e => setAmpModel(e.target.value)} placeholder="claude-opus-4-5" />
                    </div>
                    <div style={row}>
                      <label style={lbl}>Steps <span style={{ fontWeight: 400, opacity: 0.6 }}>(one per line, optional)</span></label>
                      <textarea
                        style={{ ...monoField, minHeight: 60 }}
                        value={ampSteps}
                        onChange={e => setAmpSteps(e.target.value)}
                        placeholder={"Run analysis\nGenerate report\nNotify team"}
                        spellCheck={false}
                      />
                    </div>
                    <div style={row}>
                      <label style={lbl}>Context Variables <span style={{ fontWeight: 400, opacity: 0.6 }}>(optional)</span></label>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
                        {ampContext.map((kv, i) => (
                          <div key={i} style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                            <input
                              style={{ ...field, width: 150, flexShrink: 0 }}
                              value={kv.key}
                              onChange={e => setAmpContext(p => p.map((x, j) => j === i ? { ...x, key: e.target.value } : x))}
                              placeholder="key"
                            />
                            <span style={{ color: 'var(--text-very-muted)', fontSize: 13 }}>=</span>
                            <input
                              style={{ ...field, flex: 1 }}
                              value={kv.value}
                              onChange={e => setAmpContext(p => p.map((x, j) => j === i ? { ...x, value: e.target.value } : x))}
                              placeholder="value"
                            />
                            <button
                              onClick={() => setAmpContext(p => p.filter((_, j) => j !== i))}
                              style={{
                                fontSize: 16, lineHeight: 1, background: 'none', border: 'none',
                                color: 'var(--text-muted)', cursor: 'pointer', padding: '0 3px', flexShrink: 0,
                              }}
                              title="Remove"
                            >×</button>
                          </div>
                        ))}
                        <button
                          onClick={() => setAmpContext(p => [...p, { key: '', value: '' }])}
                          style={{
                            alignSelf: 'flex-start', fontSize: 11, padding: '4px 10px',
                            background: 'transparent', border: '1px dashed var(--border-dark)',
                            borderRadius: 3, color: 'var(--text-muted)', cursor: 'pointer',
                            marginTop: 2,
                          }}
                        >+ Add variable</button>
                      </div>
                    </div>
                  </div>
                )}
              </section>

            </>
          )}
        </div>

        {/* Error banner */}
        {error && (
          <div style={{
            fontSize: 11, color: 'var(--red)', margin: '0 16px 4px',
            padding: '6px 10px', background: 'rgba(239,68,68,0.08)',
            borderRadius: 4, border: '1px solid rgba(239,68,68,0.2)', flexShrink: 0,
          }}>{error}</div>
        )}

        {/* Footer */}
        <div style={{
          display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 8,
          padding: '10px 16px', flexShrink: 0,
          borderTop: '1px solid var(--border)',
        }}>
          <button onClick={onClose} style={{
            fontSize: 11, padding: '5px 14px',
            background: 'transparent', border: '1px solid var(--border-dark)',
            borderRadius: 3, color: 'var(--text-muted)', cursor: 'pointer',
          }}>Cancel</button>
          <button
            onClick={handleSave}
            disabled={saving || loading}
            style={{
              fontSize: 11, padding: '5px 16px',
              background: saving ? 'var(--bg-pane-title)' : 'var(--amber)',
              border: 'none', borderRadius: 3,
              color: saving ? 'var(--text-muted)' : '#000',
              cursor: saving ? 'not-allowed' : 'pointer',
              fontWeight: 600,
            }}
          >{saving ? 'Saving…' : 'Save'}</button>
        </div>

      </div>
    </>
  )
}
