import { useEffect, useState } from 'react'
import { Connector, listConnectors } from '../../api/mirror'
import ConnectorList from './ConnectorList'
import EntityBrowser from './EntityBrowser'

export default function MirrorView() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)

  useEffect(() => {
    listConnectors().then(cs => {
      setConnectors(cs)
      if (cs.length > 0) setSelectedId(cs[0].id)
    }).catch(console.error)
  }, [])

  const selected = connectors.find(c => c.id === selectedId) ?? null

  return (
    <div className="flex h-full">
      <ConnectorList
        connectors={connectors}
        selectedId={selectedId}
        onSelect={setSelectedId}
      />
      <div className="flex-1 overflow-hidden">
        {selected
          ? <EntityBrowser connector={selected} />
          : <div className="p-8 text-[#8b949e] text-sm">Select a connector to browse entities</div>
        }
      </div>
    </div>
  )
}
