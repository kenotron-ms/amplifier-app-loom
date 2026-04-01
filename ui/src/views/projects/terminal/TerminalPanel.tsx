/**
 * Combines XTermTerminal (display) + useTerminalSocket (WS connection).
 * Rendered once per processId, kept alive with visibility:hidden when inactive.
 */
import { useCallback } from 'react'
import { XTermTerminal } from './XTermTerminal'
import { useTerminalSocket } from './useTerminalSocket'

interface Props {
  processId: string
}

export function TerminalPanel({ processId }: Props) {
  // Write incoming WS data into the terminal via the global registry
  const handleData = useCallback((data: string) => {
    const reg = (window as any).__terminalRegistry
    reg?.terminals?.get(processId)?.write(data)
  }, [processId])

  // write() sends keyboard input from the terminal back to the PTY
  const { write } = useTerminalSocket({ processId, onData: handleData })

  return <XTermTerminal processId={processId} onData={write} />
}
