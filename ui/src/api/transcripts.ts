export interface Transcript {
  id: string
  date: string
  time: string
  app: string
  has_audio: boolean
  created_at: string
}

const BASE = '/api'

export async function listTranscripts(): Promise<Transcript[]> {
  const r = await fetch(`${BASE}/transcripts`)
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}

export async function getTranscriptContent(id: string): Promise<string> {
  const r = await fetch(`${BASE}/transcripts/${encodeURIComponent(id)}/content`)
  if (!r.ok) throw new Error(await r.text())
  return r.text()
}

export function transcriptAudioURL(id: string): string {
  return `${BASE}/transcripts/${encodeURIComponent(id)}/audio`
}

export async function deleteTranscript(id: string): Promise<void> {
  const r = await fetch(`${BASE}/transcripts/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!r.ok) throw new Error(await r.text())
}
