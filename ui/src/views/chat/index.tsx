import { useEffect, useRef, useState } from 'react'
import { ChatMessage, sendChat, getChatHistory, clearChatHistory } from '../../api/chat'

export default function ChatView() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
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
    } catch (e: unknown) {
      const err = e as { message?: string; error?: string }
      if (err?.error === 'no_api_key') {
        setError('AI assistant not configured. Add your API key in Settings.')
      } else {
        setError(err?.message ?? 'Something went wrong.')
      }
      // remove the optimistic user message
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
    <div className="flex flex-col h-full bg-[#0d1117]">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 bg-[#161b22] border-b border-[#30363d] shrink-0">
        <span className="text-sm font-semibold text-[#e6edf3]">AI Assistant</span>
        <button
          onClick={handleClear}
          className="text-xs text-[#8b949e] hover:text-[#f85149]"
        >
          Clear history
        </button>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {messages.length === 0 && (
          <div className="text-sm text-[#8b949e] text-center mt-8">
            Ask me to create, list, or manage your jobs.
            <div className="text-xs mt-2 text-[#484f58]">e.g. "List my jobs" or "Create a daily digest job"</div>
          </div>
        )}
        {messages.map(msg => (
          <div
            key={msg.id}
            className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            <div
              className={[
                'max-w-[75%] rounded-lg px-3 py-2 text-sm',
                msg.role === 'user'
                  ? 'bg-[#1f6feb] text-white'
                  : 'bg-[#21262d] text-[#e6edf3]',
              ].join(' ')}
            >
              <pre className="whitespace-pre-wrap font-sans">{msg.content}</pre>
            </div>
          </div>
        ))}
        {loading && (
          <div className="flex justify-start">
            <div className="bg-[#21262d] text-[#8b949e] rounded-lg px-3 py-2 text-sm">
              thinking…
            </div>
          </div>
        )}
        {error && (
          <div className="text-xs text-[#f85149] bg-[#3a1a1a] rounded px-3 py-2">
            {error}
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="px-4 py-3 border-t border-[#30363d] shrink-0">
        <div className="flex gap-2">
          <input
            className="flex-1 px-3 py-1.5 text-sm bg-[#161b22] border border-[#30363d] rounded text-[#e6edf3] placeholder:text-[#484f58] focus:outline-none focus:border-[#58a6ff]"
            placeholder="Ask anything about your jobs…"
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && !e.shiftKey && handleSend()}
            disabled={loading}
          />
          <button
            onClick={handleSend}
            disabled={loading || !input.trim()}
            className="px-3 py-1.5 text-xs bg-[#238636] hover:bg-[#2ea043] disabled:opacity-40 text-white rounded"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  )
}
