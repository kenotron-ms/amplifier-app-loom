import { useEffect, useRef, useState } from 'react'

/**
 * Streams log output for a run via SSE.
 * The server sends JSON events: {"chunk":"..."} — we extract the chunk
 * and accumulate into a single string so it renders correctly in <pre>.
 */
export function useRunStream(runId: string | null): string {
  const [output, setOutput] = useState('')
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!runId) {
      setOutput('')
      return
    }
    setOutput('')
    const es = new EventSource(`/api/runs/${runId}/stream`)
    esRef.current = es

    es.onmessage = (e) => {
      let chunk = e.data as string
      try {
        const parsed = JSON.parse(chunk) as { chunk?: string }
        chunk = parsed.chunk ?? chunk
      } catch { /* not JSON — use raw text */ }
      setOutput(prev => prev + chunk)
    }

    es.addEventListener('done', () => es.close())
    es.onerror = () => es.close()

    return () => {
      es.close()
      esRef.current = null
    }
  }, [runId])

  return output
}
