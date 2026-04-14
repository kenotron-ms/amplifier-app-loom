import { useEffect, useState } from 'react'

const TERMINALS = ['Ghostty', 'Terminal.app', 'iTerm2', 'Warp']

interface Props { onClose: () => void }

export default function SettingsModal({ onClose }: Props) {
  const [preferredTerminal, setPreferredTerminal] = useState('Ghostty')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  // Load current setting from server
  useEffect(() => {
    fetch('/api/settings')
      .then(r => r.json())
      .then(data => {
        if (data.preferredTerminal) setPreferredTerminal(data.preferredTerminal)
      })
      .catch(console.error)
  }, [])

  async function handleTerminalChange(value: string) {
    setPreferredTerminal(value)
    setSaving(true)
    setSaved(false)
    try {
      await fetch('/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ preferredTerminal: value }),
      })
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (err) {
      console.error('Failed to save terminal preference:', err)
    } finally {
      setSaving(false)
    }
  }

  const labelStyle: React.CSSProperties = {
    display: 'block',
    fontSize: 10,
    fontWeight: 600,
    textTransform: 'uppercase',
    letterSpacing: '0.08em',
    color: 'var(--text-very-muted)',
    marginBottom: 6,
  }

  const selectStyle: React.CSSProperties = {
    width: '100%',
    padding: '7px 10px',
    fontSize: 13,
    background: 'var(--bg-input)',
    border: '1px solid var(--border)',
    borderRadius: 4,
    color: 'var(--text-primary)',
    outline: 'none',
    fontFamily: 'var(--font-ui)',
    cursor: 'pointer',
  }

  return (
    <div
      style={{
        position: 'fixed', inset: 0,
        background: 'rgba(0,0,0,0.5)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 50,
      }}
      onClick={e => { if (e.target === e.currentTarget) onClose() }}
    >
      <div style={{
        background: 'var(--bg-modal)',
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: 24,
        width: 380,
        boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
      }}>
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          marginBottom: 16,
        }}>
          <h3 style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Settings
          </h3>
          <button
            onClick={onClose}
            style={{
              fontSize: 18, color: 'var(--text-very-muted)',
              background: 'none', border: 'none', cursor: 'pointer', lineHeight: 1,
            }}
            onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-muted)'}
            onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'}
            aria-label="Close"
          >×</button>
        </div>

        <div style={{ borderTop: '1px solid var(--border)', marginBottom: 20 }} />

        {/* Preferred Terminal */}
        <div style={{ marginBottom: 20 }}>
          <label style={labelStyle}>Preferred Terminal</label>
          <select
            value={preferredTerminal}
            onChange={e => handleTerminalChange(e.target.value)}
            disabled={saving}
            style={selectStyle}
          >
            {TERMINALS.map(t => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          {saved && (
            <div style={{ fontSize: 11, color: 'var(--green)', marginTop: 6 }}>✓ Saved</div>
          )}
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button
            onClick={onClose}
            style={{
              padding: '7px 16px', fontSize: 13,
              background: 'var(--bg-pane-title)',
              border: '1px solid var(--border)',
              borderRadius: 4, cursor: 'pointer',
              color: 'var(--text-primary)',
            }}
          >Close</button>
        </div>
      </div>
    </div>
  )
}
