export interface Connector {
  id: string
  name: string
  type: string
  health: 'live' | 'idle' | 'error'
  lastSyncAt?: string
  entityCount?: number
}

export interface Entity {
  address: string
  type: string
  data: Record<string, unknown>
  updatedAt: string
}

export async function listConnectors(): Promise<Connector[]> {
  const res = await fetch('/api/mirror/connectors')
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function listEntities(connectorId: string): Promise<Entity[]> {
  const res = await fetch(`/api/mirror/entities?connectorId=${connectorId}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
