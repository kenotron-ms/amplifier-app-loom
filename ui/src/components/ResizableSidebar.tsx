import { useEffect, useRef, useState } from 'react'

interface Props {
  defaultWidth?: number
  minWidth?: number
  maxWidth?: number
  children: React.ReactNode
}

/**
 * Wraps a sidebar with a drag-to-resize handle on its right edge.
 * The handle is a 5px strip that turns amber on hover.
 * Children are responsible for their own border-right / background styling.
 */
export default function ResizableSidebar({
  defaultWidth = 220,
  minWidth = 120,
  maxWidth = 600,
  children,
}: Props) {
  const [width, setWidth] = useState(defaultWidth)
  const dragging = useRef(false)
  const startX  = useRef(0)
  const startW  = useRef(0)

  const onHandleMouseDown = (e: React.MouseEvent) => {
    dragging.current = true
    startX.current   = e.clientX
    startW.current   = width
    e.preventDefault()
  }

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!dragging.current) return
      const next = startW.current + (e.clientX - startX.current)
      setWidth(Math.max(minWidth, Math.min(maxWidth, next)))
    }
    const onUp = () => { dragging.current = false }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup',   onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup',   onUp)
    }
  }, [minWidth, maxWidth])

  return (
    <div style={{ width, flexShrink: 0, position: 'relative', height: '100%' }}>
      {children}
      {/* Drag handle — 5px strip on the right edge, turns amber on hover */}
      <div
        onMouseDown={onHandleMouseDown}
        style={{
          position: 'absolute',
          right: 0,
          top: 0,
          bottom: 0,
          width: 5,
          cursor: 'col-resize',
          zIndex: 20,
          transition: 'background 0.12s',
        }}
        onMouseEnter={e => (e.currentTarget.style.background = 'rgba(245,158,11,0.35)')}
        onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
      />
    </div>
  )
}
