import { useEffect, useRef, useState } from 'react'
import { ChatMessage, sendChat, getChatHistory, clearChatHistory } from '../../api/chat'

interface Props {
  onResponse?: () => void
}

export default function ChatView({ onResponse }: Props = {}) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput]       = useState('')
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState<string | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    getChatHistory()
      .then(h => setMessages(h ?? []))
      .catch(console.error)
  }, [])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const handleSend = async () => {
    const msg = input.trim()
    if (!msg || loading) return
    setInput('')
    setLoading(true)
    setError(null)

    const userMsg: ChatMessage = {
      id: `temp-${Date.now()}`,
      role: 'user',
      content: msg,
      createdAt: new Date().toISOString(),
    }
    setMessages(prev => [...prev, userMsg])

    try {
      const res = await sendChat(msg)
      const assistantMsg: ChatMessage = {
        id: `temp-${Date.now()}-a`,
        role: 'assistant',
        content: res.text,
        createdAt: new Date().toISOString(),
      }
      setMessages(prev => [...prev, assistantMsg])
      onResponse?.()
    } catch (e: unknown) {
      const err = e as { message?: string; error?: string }
      if (err?.error === 'no_api_key') {
        setError('AI assistant not configured. Add your API key in Settings.')
      } else {
        // Backend always returns {error: "..."} — prefer that over the never-set .message field.
        setError(err?.error ?? err?.message ?? 'Something went wrong.')
      }
      setMessages(prev => prev.filter(m => m.id !== userMsg.id))
    } finally {
      setLoading(false)
    }
  }

  const handleClear = async () => {
    await clearChatHistory().catch(console.error)
    setMessages([])
  }

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      background: 'var(--bg-right)',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '0 16px',
        height: 32,
        background: 'var(--bg-pane-title)',
        borderBottom: '1px solid var(--border)',
        flexShrink: 0,
      }}>
        <span style={{
          fontSize: 10, fontWeight: 600,
          textTransform: 'uppercase', letterSpacing: '0.08em',
          color: 'var(--text-very-muted)',
        }}>AI Assistant</span>
        <button
          onClick={handleClear}
          style={{
            fontSize: 10, color: 'var(--text-very-muted)',
            background: 'none', border: 'none', cursor: 'pointer',
          }}
          onMouseEnter={e => (e.currentTarget as HTMLElement).style.color = 'var(--red)'}
          onMouseLeave={e => (e.currentTarget as HTMLElement).style.color = 'var(--text-very-muted)'}
        >Clear history</button>
      </div>

      {/* Messages */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '16px', display: 'flex', flexDirection: 'column', gap: 10 }} className="canvas-scroll">
        {messages.length === 0 && (
          <div style={{ textAlign: 'center', marginTop: 32 }}>
            <div style={{ fontSize: 13, color: 'var(--text-muted)', marginBottom: 8 }}>
              Describe what you want — I'll create or manage jobs for you.
            </div>
            <div style={{ fontSize: 11, color: 'var(--text-very-muted)' }}>
              e.g. "Create a daily digest job" or "Run the backup job now"
            </div>
          </div>
        )}
        {messages.map(msg => (
          <div
            key={msg.id}
            style={{
              display: 'flex',
              justifyContent: msg.role === 'user' ? 'flex-end' : 'flex-start',
            }}
          >
            <div style={{
              maxWidth: '78%',
              padding: '8px 12px',
              borderRadius: msg.role === 'user' ? '12px 12px 2px 12px' : '12px 12px 12px 2px',
              fontSize: 13,
              lineHeight: 1.5,
              background: msg.role === 'user'
                ? 'var(--amber)'
                : 'var(--bg-pane-title)',
              color: msg.role === 'user'
                ? '#1C1A16'
                : 'var(--text-primary)',
              border: msg.role === 'user'
                ? 'none'
                : '1px solid var(--border)',
            }}>
              <pre style={{ whiteSpace: 'pre-wrap', fontFamily: 'inherit', margin: 0 }}>{msg.content}</pre>
            </div>
          </div>
        ))}
        {loading && (
          <div style={{ display: 'flex', justifyContent: 'flex-start' }}>
            <div style={{
              padding: '8px 12px',
              background: 'var(--bg-pane-title)',
              border: '1px solid var(--border)',
              borderRadius: '12px 12px 12px 2px',
              fontSize: 12,
              color: 'var(--text-very-muted)',
            }}>thinking…</div>
          </div>
        )}
        {error && (
          <div style={{
            fontSize: 11, color: 'var(--red)',
            background: 'rgba(229,57,53,0.06)',
            border: '1px solid rgba(229,57,53,0.20)',
            borderRadius: 4, padding: '8px 12px',
          }}>{error}</div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div style={{
        padding: '10px 14px',
        borderTop: '1px solid var(--border)',
        flexShrink: 0,
        display: 'flex', gap: 8,
      }}>
        <input
          style={{
            flex: 1,
            padding: '7px 10px',
            fontSize: 13,
            background: 'var(--bg-input)',
            border: '1px solid var(--border)',
            borderRadius: 3,
            color: 'var(--text-primary)',
            outline: 'none',
            fontFamily: 'var(--font-ui)',
          }}
          placeholder="Describe what you want…"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && !e.shiftKey && handleSend()}
          disabled={loading}
          autoFocus
          onFocus={e => (e.currentTarget as HTMLElement).style.borderColor = 'var(--amber)'}
          onBlur={e => (e.currentTarget as HTMLElement).style.borderColor = 'var(--border)'}
        />
        <button
          onClick={handleSend}
          disabled={loading || !input.trim()}
          style={{
            padding: '7px 14px',
            fontSize: 12,
            background: 'var(--bg-modal)',
            border: '1px solid var(--border-dark)',
            borderRadius: 3,
            color: 'var(--text-primary)',
            cursor: 'pointer',
            opacity: (loading || !input.trim()) ? 0.4 : 1,
          }}
        >Send →</button>
      </div>
    </div>
  )
}
