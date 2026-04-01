import { useState } from 'react'
import { submitFeedback } from '../api/feedback'

interface Props {
  onClose: () => void
}

type State = 'idle' | 'submitting' | 'done' | 'error'

export default function FeedbackModal({ onClose }: Props) {
  const [title, setTitle]     = useState('')
  const [body, setBody]       = useState('')
  const [state, setState]     = useState<State>('idle')
  const [issueUrl, setIssueUrl] = useState('')
  const [errorMsg, setErrorMsg] = useState('')

  const canSubmit = title.trim().length > 0 && state === 'idle'

  async function handleSubmit() {
    if (!canSubmit) return
    setState('submitting')
    try {
      const result = await submitFeedback({ title: title.trim(), body: body.trim() })
      setIssueUrl(result.url)
      setState('done')
    } catch (e: unknown) {
      setErrorMsg((e as Error).message)
      setState('error')
    }
  }

  return (
    <div
      className="fixed inset-0 bg-black/60 flex items-center justify-center z-50"
      onClick={e => { if (e.target === e.currentTarget) onClose() }}
    >
      <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-5 w-96 shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-sm font-semibold text-[#e6edf3]">Send Feedback</h3>
          <button
            onClick={onClose}
            className="text-[#484f58] hover:text-[#8b949e] text-lg leading-none"
            aria-label="Close"
          >×</button>
        </div>

        {state === 'done' ? (
          /* Success state */
          <div className="text-center py-4">
            <div className="text-2xl mb-2">✓</div>
            <p className="text-sm text-[#e6edf3] mb-1">Issue filed!</p>
            <a
              href={issueUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-[#58a6ff] hover:underline break-all"
            >
              {issueUrl}
            </a>
            <div className="mt-4">
              <button
                onClick={onClose}
                className="px-4 py-1.5 text-xs bg-[#21262d] border border-[#30363d] rounded text-[#e6edf3] hover:bg-[#30363d]"
              >
                Close
              </button>
            </div>
          </div>
        ) : (
          /* Form state */
          <>
            <div className="mb-3">
              <label className="block text-[11px] text-[#8b949e] mb-1">
                Title <span className="text-[#f85149]">*</span>
              </label>
              <input
                autoFocus
                value={title}
                onChange={e => setTitle(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSubmit()}
                disabled={state === 'submitting'}
                placeholder="Short description of the issue or idea"
                className="w-full px-3 py-1.5 text-xs bg-[#0d1117] border border-[#30363d] rounded text-[#e6edf3] placeholder:text-[#484f58] focus:outline-none focus:border-[#58a6ff] disabled:opacity-50"
              />
            </div>

            <div className="mb-4">
              <label className="block text-[11px] text-[#8b949e] mb-1">Details</label>
              <textarea
                value={body}
                onChange={e => setBody(e.target.value)}
                disabled={state === 'submitting'}
                rows={5}
                placeholder="Steps to reproduce, expected vs actual behavior, etc."
                className="w-full px-3 py-1.5 text-xs bg-[#0d1117] border border-[#30363d] rounded text-[#e6edf3] placeholder:text-[#484f58] focus:outline-none focus:border-[#58a6ff] resize-none disabled:opacity-50"
              />
            </div>

            {state === 'error' && (
              <p className="text-[11px] text-[#f85149] mb-3 font-mono">{errorMsg}</p>
            )}

            <div className="flex gap-2 justify-end">
              <button
                onClick={onClose}
                disabled={state === 'submitting'}
                className="px-3 py-1.5 text-xs text-[#8b949e] hover:text-[#e6edf3] disabled:opacity-40"
              >
                Cancel
              </button>
              <button
                onClick={handleSubmit}
                disabled={!canSubmit}
                className="px-3 py-1.5 text-xs bg-[#238636] hover:bg-[#2ea043] text-white rounded disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {state === 'submitting' ? 'Filing…' : 'Submit Issue'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
