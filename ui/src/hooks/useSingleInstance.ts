import { useEffect, useState } from 'react'

const CHANNEL_NAME = 'loom-dashboard'
const PONG_WAIT_MS  = 200   // how long to wait for an existing tab to reply

/**
 * Detects whether another Loom dashboard tab is already open.
 *
 * Protocol:
 *   - On mount: broadcast { type: 'ping' }
 *   - Existing tabs hear the ping, respond { type: 'pong' }, and call window.focus()
 *   - If we receive a pong within PONG_WAIT_MS we know we're a duplicate → return true
 *   - If no pong arrives in time, we are the primary tab → respond to future pings
 *
 * Returns true when the current tab is a duplicate (another tab is already open).
 */
export function useSingleInstance(): boolean {
  const [isDuplicate, setIsDuplicate] = useState(false)

  useEffect(() => {
    if (typeof BroadcastChannel === 'undefined') return  // SSR / old browser safety

    const ch = new BroadcastChannel(CHANNEL_NAME)
    let settled = false

    const timeoutId = setTimeout(() => {
      // No pong arrived — we are the primary tab.
      // From now on, respond to pings from any new tabs.
      settled = true
      ch.onmessage = (e: MessageEvent) => {
        if (e.data?.type === 'ping') {
          ch.postMessage({ type: 'pong' })
          window.focus()
        }
      }
    }, PONG_WAIT_MS)

    // During the initial window, listen for a pong back to our ping.
    ch.onmessage = (e: MessageEvent) => {
      if (settled) return
      if (e.data?.type === 'pong') {
        settled = true
        clearTimeout(timeoutId)
        setIsDuplicate(true)
      }
    }

    // Announce ourselves — existing tabs will pong back.
    ch.postMessage({ type: 'ping' })

    return () => {
      clearTimeout(timeoutId)
      ch.close()
    }
  }, [])

  return isDuplicate
}
