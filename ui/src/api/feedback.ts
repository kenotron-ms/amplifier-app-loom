export interface FeedbackPayload {
  title: string
  body: string
}

export interface FeedbackResult {
  url: string
}

export async function submitFeedback(payload: FeedbackPayload): Promise<FeedbackResult> {
  const res = await fetch('/api/feedback', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? 'Failed to submit feedback')
  }
  return res.json()
}
