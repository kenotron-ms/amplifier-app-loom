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
  jobName: string
  status: 'pending' | 'running' | 'success' | 'failed' | 'timeout' | 'skipped'
  startedAt: string
  endedAt?: string   // NOTE: was "finishedAt" — corrected to match Go backend
  exitCode: number
  output?: string
  attempt: number
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

export async function deleteJob(jobId: string): Promise<void> {
  const res = await fetch(`/api/jobs/${jobId}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await res.text())
}
