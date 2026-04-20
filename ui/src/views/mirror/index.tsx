import { useEffect, useState } from 'react'
import { Connector, listConnectors } from '../../api/mirror'
import ConnectorList from './ConnectorList'
import EntityBrowser from './EntityBrowser'
import ResizableSidebar from '../../components/ResizableSidebar'

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
    <div style={{ display: 'flex', height: '100%', background: 'var(--bg-page)' }}>
      <ResizableSidebar defaultWidth={200}>
        <ConnectorList
          connectors={connectors}
          selectedId={selectedId}
          onSelect={setSelectedId}
        />
      </ResizableSidebar>
      <div style={{ flex: 1, overflow: 'hidden' }}>
        {selected
          ? <EntityBrowser connector={selected} />
          : (
            <div style={{
              padding: 32, fontSize: 12,
              color: 'var(--text-very-muted)',
            }}>Select a connector to browse entities</div>
          )
        }
      </div>
    </div>
  )
}
