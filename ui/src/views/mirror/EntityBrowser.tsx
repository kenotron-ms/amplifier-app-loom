import { useEffect, useState } from 'react'
import { Connector, Entity, listEntities } from '../../api/mirror'

interface Props {
  connector: Connector
}

export default function EntityBrowser({ connector }: Props) {
  const [entities, setEntities] = useState<Entity[]>([])
  const [selected, setSelected] = useState<Entity | null>(null)

  useEffect(() => {
    setEntities([])
    setSelected(null)
    listEntities(connector.id).then(setEntities).catch(console.error)
  }, [connector.id])

  return (
    <div className="flex flex-col h-full">
      <div className="px-4 py-2 bg-[#161b22] border-b border-[#30363d] shrink-0">
        <span className="text-sm font-semibold text-[#e6edf3]">{connector.name}</span>
        <span className="ml-2 text-xs text-[#8b949e]">
          {entities.length} entities
          {connector.lastSyncAt && ` · last sync ${new Date(connector.lastSyncAt).toLocaleTimeString()}`}
        </span>
      </div>
      <div className="flex flex-1 overflow-hidden">
        {/* Entity list */}
        <div className="w-72 border-r border-[#30363d] overflow-y-auto shrink-0">
          {entities.map(e => (
            <button
              key={e.address}
              onClick={() => setSelected(e)}
              className={[
                'w-full text-left px-3 py-2 border-b border-[#21262d] hover:bg-[#161b22] transition-colors',
                selected?.address === e.address ? 'bg-[#21262d]' : '',
              ].join(' ')}
            >
              <div className="text-xs text-[#e6edf3] truncate">{e.address}</div>
              <div className="text-[10px] text-[#8b949e]">{e.type}</div>
            </button>
          ))}
          {entities.length === 0 && (
            <div className="px-3 py-4 text-xs text-[#8b949e]">No entities</div>
          )}
        </div>
        {/* Entity detail */}
        <div className="flex-1 overflow-auto p-4">
          {selected ? (
            <pre className="text-xs text-[#e6edf3] whitespace-pre-wrap font-mono">
              {JSON.stringify(selected.data, null, 2)}
            </pre>
          ) : (
            <span className="text-[#8b949e] text-sm">Select an entity</span>
          )}
        </div>
      </div>
    </div>
  )
}
