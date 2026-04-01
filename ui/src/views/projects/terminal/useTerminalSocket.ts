/**
 * Ported from amplifier-grove: apps/web/src/components/Terminal/hooks/useTerminalSocket.ts
 * Stripped: auth tokens, API resize call (loom uses WebSocket directly)
 */
import { useCallback, useEffect, useRef } from 'react'

interface Props {
  processId: string | null
  onData: (data: string) => void
  onExit?: (code: number) => void
}

// Exponential backoff with jitter, capped at 30s (JupyterLab pattern)
function getReconnectDelay(attempt: number): number {
  const base = Math.min(30_000, 1_000 * Math.pow(2, attempt))
  return base + Math.random() * 1_000
}

function isTerminalReady(processId: string): boolean {
  const reg = (window as any).__terminalRegistry
  return reg?.terminals?.get(processId)?.ready === true
}

function getTerminalDims(processId: string): { cols: number; rows: number } | null {
  const reg = (window as any).__terminalRegistry
  const entry = reg?.terminals?.get(processId)
  if (!entry?.cols || !entry?.rows) return null
  return { cols: entry.cols, rows: entry.rows }
}

export function useTerminalSocket({ processId, onData, onExit }: Props) {
  const wsRef = useRef<WebSocket | null>(null)
  const onDataRef = useRef(onData)
  const onExitRef = useRef(onExit)
  const reconnectAttempts = useRef(0)
  const intentionalClose = useRef(false)

  useEffect(() => {
    onDataRef.current = onData
    onExitRef.current = onExit
  }, [onData, onExit])

  useEffect(() => {
    if (!processId) return

    intentionalClose.current = false
    reconnectAttempts.current = 0

    const connect = () => {
      if (intentionalClose.current) return
      if (wsRef.current) return

      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${proto}//${window.location.host}/api/terminal/${processId}`

      const ws = new WebSocket(url)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        reconnectAttempts.current = 0
        // Send initial terminal dimensions — server uses this to size the PTY
        const dims = getTerminalDims(processId)
        if (dims) {
          ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }))
        }
      }

      ws.onmessage = (e) => {
        const data = e.data instanceof ArrayBuffer
          ? new TextDecoder().decode(e.data)
          : e.data as string
        onDataRef.current(data)
      }

      ws.onclose = (e) => {
        wsRef.current = null
        // 4xxx codes encode process exit: code - 4000 = exit code
        if (e.code >= 4000 && e.code < 5000) {
          onExitRef.current?.(e.code - 4000)
          return
        }
        if (!intentionalClose.current) {
          const delay = getReconnectDelay(reconnectAttempts.current)
          reconnectAttempts.current += 1
          setTimeout(connect, delay)
        }
      }

      ws.onerror = () => {} // onclose fires after error, handles reconnect
    }

    // Gate connection: wait for XTermTerminal to register itself in the registry
    let pollId: ReturnType<typeof setTimeout> | null = null

    const waitForTerminal = () => {
      if (intentionalClose.current) return
      if (isTerminalReady(processId)) {
        connect()
      } else {
        pollId = setTimeout(waitForTerminal, 50)
      }
    }
    waitForTerminal()

    return () => {
      intentionalClose.current = true
      if (pollId !== null) clearTimeout(pollId)
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [processId])

  const write = useCallback((data: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(data)
    }
  }, [])

  return { write }
}
