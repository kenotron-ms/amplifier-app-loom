/**
 * Ported from amplifier-grove: apps/web/src/components/Terminal/XTermTerminal.tsx
 *
 * Key pattern: window.__terminalRegistry holds a rolling output buffer per processId.
 * On remount (session switch), the buffer is replayed into a fresh xterm instance so
 * scroll history survives without keeping xterm alive forever.
 */
import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

interface TerminalRegistry {
  terminals: Map<string, {
    write: (data: string) => void
    scrollToBottom: () => void
    ready: boolean
    cols?: number
    rows?: number
  }>
  buffers: Map<string, string>
}

const BUFFER_MAX_BYTES = 256 * 1024   // 256 KB rolling window
const SCROLLBACK_LINES  = 10_000

// Global registry — one instance for the lifetime of the page
if (!(window as any).__terminalRegistry) {
  ;(window as any).__terminalRegistry = {
    terminals: new Map(),
    buffers:   new Map(),
  } as TerminalRegistry
}

interface Props {
  processId: string
  onData:    (data: string) => void
}

export function XTermTerminal({ processId, onData }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const onDataRef    = useRef(onData)

  useEffect(() => { onDataRef.current = onData }, [onData])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    let initRafId:       number | null = null
    let fitRafId:        number | null = null
    let resizeFitRafId:  number | null = null
    let dimObserver:     ResizeObserver | null = null
    let term:            Terminal | null = null
    let fit:             FitAddon | null = null
    let initialized = false

    const init = () => {
      if (initialized) return
      initialized = true

      dimObserver?.disconnect()
      dimObserver = null

      term = new Terminal({
        cursorBlink: true,
        fontSize: 13,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        theme: {
          background: '#0d1117',
          foreground: '#e6edf3',
          cursor:     '#58a6ff',
          selectionBackground: '#264f78',
          black: '#0d1117', brightBlack: '#484f58',
          red:   '#f85149', brightRed:   '#ff7b72',
          green: '#3fb950', brightGreen: '#56d364',
          yellow:'#d29922', brightYellow:'#e3b341',
          blue:  '#388bfd', brightBlue:  '#79c0ff',
          magenta:'#bc8cff',brightMagenta:'#d2a8ff',
          cyan:  '#39c5cf', brightCyan:  '#56d4dd',
          white: '#b1bac4', brightWhite: '#f0f6fc',
        },
        scrollback: SCROLLBACK_LINES,
      })

      fit = new FitAddon()
      term.loadAddon(fit)
      term.open(el)

      // Sync dimensions eagerly before first fit rAF
      try {
        const d = fit.proposeDimensions()
        if (d && d.cols > 0 && d.rows > 0) term.resize(d.cols, d.rows)
      } catch { /* RenderService not ready yet — rAF handles it */ }

      // On resize: update registry dims AND send resize envelope over the WebSocket.
      // The backend intercepts {"type":"resize",...} and calls creackpty.Setsize
      // so the PTY grid matches the xterm viewport — no raw JSON leaks to the process.
      term.onResize(({ cols, rows }) => {
        const reg = (window as any).__terminalRegistry as TerminalRegistry
        const entry = reg.terminals.get(processId)
        if (entry) { entry.cols = cols; entry.rows = rows }
        onDataRef.current(JSON.stringify({ type: 'resize', cols, rows }))
      })

      // Replay buffer — find last full clear/alt-screen so we skip stale intermediate
      // clears and land on the most recent screen state
      const buf = ((window as any).__terminalRegistry as TerminalRegistry).buffers.get(processId)
      if (buf) {
        const FULL_CLEAR = '\x1b[2J'
        const ALT_ENTER  = '\x1b[?1049h'
        const lastClear  = Math.max(buf.lastIndexOf(FULL_CLEAR), buf.lastIndexOf(ALT_ENTER))
        term.write(buf.slice(lastClear > 0 ? lastClear : 0))
      }

      fitRafId = requestAnimationFrame(() => {
        fitRafId = null
        if (!term || !fit) return
        try { fit.fit(); term.scrollToBottom() } catch { /* ignore */ }
      })

      term.onData((d) => onDataRef.current(d))

      // rAF-coalesced resize — at most one fit() per paint frame
      const handleResize = () => {
        if (resizeFitRafId !== null) return
        resizeFitRafId = requestAnimationFrame(() => {
          resizeFitRafId = null
          try { fit?.fit() } catch { /* ignore */ }
        })
      }
      window.addEventListener('resize', handleResize)

      const containerObserver = new ResizeObserver(() => handleResize())
      containerObserver.observe(el)

      const reg = (window as any).__terminalRegistry as TerminalRegistry
      reg.terminals.set(processId, {
        write: (data: string) => {
          term!.write(data)
          // Append to rolling buffer so future remounts can replay history
          const prev = reg.buffers.get(processId) ?? ''
          const next = prev + data
          reg.buffers.set(
            processId,
            next.length > BUFFER_MAX_BYTES ? next.slice(next.length - BUFFER_MAX_BYTES) : next
          )
        },
        scrollToBottom: () => term!.scrollToBottom(),
        ready: true,
        cols: term!.cols,
        rows: term!.rows,
      })

      return () => {
        window.removeEventListener('resize', handleResize)
        containerObserver.disconnect()
      }
    }

    let resizeCleanup: (() => void) | null = null

    if (el.offsetWidth > 0 && el.offsetHeight > 0) {
      initRafId = requestAnimationFrame(() => {
        initRafId = null
        resizeCleanup = init() ?? null
      })
    } else {
      dimObserver = new ResizeObserver((entries) => {
        const e = entries[0]
        if (e?.contentRect.width > 0 && e.contentRect.height > 0) {
          resizeCleanup = init() ?? null
        }
      })
      dimObserver.observe(el)
    }

    return () => {
      if (initRafId     !== null) cancelAnimationFrame(initRafId)
      if (fitRafId      !== null) cancelAnimationFrame(fitRafId)
      if (resizeFitRafId !== null) cancelAnimationFrame(resizeFitRafId)
      dimObserver?.disconnect()
      resizeCleanup?.()

      // Remove terminal entry — keep the buffer so the next mount can replay history
      ;((window as any).__terminalRegistry as TerminalRegistry).terminals.delete(processId)

      term?.dispose()
    }
  }, [processId]) // only re-run when processId changes

  return (
    <div
      ref={containerRef}
      style={{
        position: 'relative',
        width: '100%',
        height: '100%',
        padding: '8px',
        boxSizing: 'border-box',
        overflow: 'hidden',
      }}
    />
  )
}
