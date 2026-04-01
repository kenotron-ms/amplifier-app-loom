export interface Job {
  id: string
  name: string
  description: string
  enabled: boolean
  trigger: { type: string; schedule: string }
  executor: string
  lastRunAt?: string
  lastRunStatus?: string
}

export interface JobRun {
  id: string
  jobId: string
  status: 'running' | 'succeeded' | 'failed' | 'cancelled'
  startedAt: string
  finishedAt?: string
  output?: string
}

export async function listJobs(): Promise<Job[]> {
  const res = await fetch('/api/jobs')
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function listJobRuns(jobId: string, limit = 20): Promise<JobRun[]> {
  const res = await fetch(`/api/jobs/${jobId}/runs?limit=${limit}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function triggerJob(jobId: string): Promise<JobRun> {
  const res = await fetch(`/api/jobs/${jobId}/trigger`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
