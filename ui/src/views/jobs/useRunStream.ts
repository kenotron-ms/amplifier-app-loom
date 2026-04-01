import { useEffect, useRef, useState } from 'react'

export function useRunStream(runId: string | null) {
  const [lines, setLines] = useState<string[]>([])
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!runId) {
      setLines([])
      return
    }
    setLines([])
    const es = new EventSource(`/api/runs/${runId}/stream`)
    esRef.current = es

    es.onmessage = (e) => {
      setLines(prev => [...prev, e.data as string])
    }
    es.onerror = () => es.close()

    return () => {
      es.close()
      esRef.current = null
    }
  }, [runId])

  return lines
}
