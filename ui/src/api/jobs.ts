export interface ShellConfig {
  command: string
}

export interface ClaudeCodeConfig {
  prompt: string
  steps?: string[]
  model?: string
  maxTurns?: number
  allowedTools?: string[]
  appendSystemPrompt?: string
}

export interface AmplifierConfig {
  prompt?: string
  steps?: string[]
  recipePath?: string
  bundle?: string
  model?: string
  context?: Record<string, string>
}

export interface WatchConfig {
  path: string
  recursive: boolean
  events?: string[]
  mode: string
  pollInterval?: string
  debounce?: string
}

export interface ConnectorConfig {
  connectorId: string
}

export interface Job {
  id: string
  name: string
  description: string
  enabled: boolean
  trigger: { type: string; schedule: string }
  executor: string
  cwd: string
  timeout: string
  maxRetries: number
  createdAt?: string
  updatedAt?: string
  lastRunAt?: string
  lastRunStatus?: string
  // Executor configs — only the one matching executor is set
  shell?: ShellConfig
  claudeCode?: ClaudeCodeConfig
  amplifier?: AmplifierConfig
  // Trigger-specific configs
  watch?: WatchConfig
  connector?: ConnectorConfig
  // Legacy (backward compat)
  command?: string
}

export interface JobRun {
  id: string
  jobId: string
  jobName: string
  status: 'pending' | 'running' | 'success' | 'failed' | 'timeout' | 'skipped' | 'cancelled'
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

export async function getJob(jobId: string): Promise<Job> {
  const res = await fetch(`/api/jobs/${jobId}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function updateJob(jobId: string, updates: Partial<Job>): Promise<Job> {
  const res = await fetch(`/api/jobs/${jobId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  })
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

export async function cancelRun(runId: string): Promise<void> {
  const res = await fetch(`/api/runs/${runId}/cancel`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}
