import { Connector } from '../../api/mirror'

interface Props {
  connectors: Connector[]
  selectedId: string | null
  onSelect: (id: string) => void
}

export default function ConnectorList({ connectors, selectedId, onSelect }: Props) {
  const healthDot = (health: string) => {
    const color = health === 'live' ? '#3fb950' : health === 'error' ? '#f85149' : '#8b949e'
    return (
      <span
        style={{ width: 6, height: 6, borderRadius: '50%', background: color, display: 'inline-block', flexShrink: 0 }}
      />
    )
  }

  return (
    <div className="flex flex-col h-full bg-[#0d1117] border-r border-[#30363d] w-52 shrink-0">
      <div className="px-3 py-2 border-b border-[#30363d]">
        <span className="text-[#8b949e] text-xs uppercase tracking-wider">Connectors</span>
      </div>
      <div className="flex-1 overflow-y-auto">
        {connectors.map(c => (
          <button
            key={c.id}
            onClick={() => onSelect(c.id)}
            className={[
              'w-full text-left px-3 py-2 border-b border-[#21262d] hover:bg-[#161b22] transition-colors',
              selectedId === c.id ? 'bg-[#21262d]' : '',
            ].join(' ')}
          >
            <div className="flex items-center justify-between gap-2">
              <span className={`text-xs truncate ${selectedId === c.id ? 'text-[#e6edf3]' : 'text-[#8b949e]'}`}>
                {c.name}
              </span>
              {healthDot(c.health)}
            </div>
            <div className="text-[10px] text-[#8b949e] mt-0.5">{c.type}</div>
          </button>
        ))}
        {connectors.length === 0 && (
          <div className="px-3 py-4 text-xs text-[#8b949e]">No connectors configured</div>
        )}
      </div>
    </div>
  )
}
