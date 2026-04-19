import { useEffect, useState } from 'react'
import { Transcript, listTranscripts, getTranscriptContent, transcriptAudioURL, deleteTranscript } from '../../api/transcripts'
import { marked } from 'marked'

function appLabel(app: string): string {
  const map: Record<string, string> = { teams: 'Teams', zoom: 'Zoom', meet: 'Google Meet' }
  return map[app.toLowerCase()] ?? app.charAt(0).toUpperCase() + app.slice(1)
}

export default function TranscriptsView() {
  const [transcripts, setTranscripts] = useState<Transcript[]>([])
  const [selected, setSelected] = useState<Transcript | null>(null)
  const [html, setHtml] = useState<string>('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    listTranscripts().then(setTranscripts).catch(console.error)
  }, [])

  const handleSelect = async (t: Transcript) => {
    setSelected(t)
    setLoading(true)
    setHtml('')
    try {
      const text = await getTranscriptContent(t.id)
      setHtml(String(await marked(text)))
    } catch {
      setHtml('<p style="color:var(--text-muted)">Failed to load transcript.</p>')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (t: Transcript, e: React.MouseEvent) => {
    e.stopPropagation()
    if (!confirm(`Delete transcript for ${appLabel(t.app)} on ${t.date}?`)) return
    await deleteTranscript(t.id).catch(console.error)
    setTranscripts(prev => prev.filter(x => x.id !== t.id))
    if (selected?.id === t.id) { setSelected(null); setHtml('') }
  }

  return (
    <div style={{ display: 'flex', height: '100%', background: 'var(--bg-page)' }}>
      {/* Left list */}
      <div style={{ width: 260, borderRight: '1px solid var(--border)', overflowY: 'auto', flexShrink: 0 }}>
        <div style={{ padding: '14px 16px 8px', fontSize: 11, fontWeight: 600, letterSpacing: '0.08em', color: 'var(--text-muted)', textTransform: 'uppercase' }}>
          Transcripts
        </div>
        {transcripts.length === 0 && (
          <div style={{ padding: '20px 16px', color: 'var(--text-muted)', fontSize: 13, lineHeight: 1.5 }}>
            No transcripts yet.<br />Enable Meeting Transcription in the tray to get started.
          </div>
        )}
        {transcripts.map(t => (
          <div
            key={t.id}
            onClick={() => handleSelect(t)}
            style={{
              padding: '10px 16px',
              cursor: 'pointer',
              borderBottom: '1px solid var(--border)',
              background: selected?.id === t.id ? 'var(--bg-selected, rgba(99,102,241,0.08))' : 'transparent',
              display: 'flex', alignItems: 'center', gap: 8,
            }}
          >
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 500, fontSize: 13, marginBottom: 2 }}>{appLabel(t.app)}</div>
              <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                {t.date}  ·  {t.time}
                {t.has_audio && <span style={{ marginLeft: 4 }}>🎵</span>}
              </div>
            </div>
            <button
              onClick={e => handleDelete(t, e)}
              title="Delete"
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', fontSize: 14, padding: '2px 6px', borderRadius: 4, lineHeight: 1 }}
            >
              ×
            </button>
          </div>
        ))}
      </div>

      {/* Right detail */}
      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        {!selected ? (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)', fontSize: 14 }}>
            Select a transcript to view
          </div>
        ) : (
          <>
            {selected.has_audio && (
              <div style={{ padding: '12px 24px', borderBottom: '1px solid var(--border)', background: 'var(--bg-card, var(--bg-page))' }}>
                <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 6, fontWeight: 500 }}>RECORDING</div>
                <audio controls src={transcriptAudioURL(selected.id)} style={{ width: '100%', height: 32 }} />
              </div>
            )}
            <div style={{ flex: 1, overflowY: 'auto', padding: '24px 32px' }}>
              {loading
                ? <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>Loading…</div>
                : <div
                    dangerouslySetInnerHTML={{ __html: html }}
                    style={{ maxWidth: 700, lineHeight: 1.75, fontSize: 14, color: 'var(--text-primary, inherit)' }}
                  />
              }
            </div>
          </>
        )}
      </div>
    </div>
  )
}
