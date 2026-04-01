export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  createdAt: string
}

export async function sendChat(message: string): Promise<{ text: string; actions?: string[] }> {
  const res = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: 'request failed' }))
    throw body
  }
  return res.json()
}

export async function getChatHistory(): Promise<ChatMessage[]> {
  const res = await fetch('/api/chat/history')
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function clearChatHistory(): Promise<void> {
  const res = await fetch('/api/chat/history', { method: 'DELETE' })
  if (!res.ok) throw new Error(await res.text())
}
