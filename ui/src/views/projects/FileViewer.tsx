import { useEffect, useState } from 'react'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import { FileEntry, listFiles, readFileContent } from '../../api/projects'

const IMAGE_EXT = /\.(png|jpe?g|gif|svg|webp|ico|avif)$/i
const isImage = (name: string) => IMAGE_EXT.test(name)

const EXT_LANG: Record<string, string> = {
  ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript',
  go: 'go', py: 'python', sh: 'bash', bash: 'bash', zsh: 'bash',
  json: 'json', yaml: 'yaml', yml: 'yaml', toml: 'toml',
  md: 'markdown', html: 'html', css: 'css', sql: 'sql',
  rs: 'rust', java: 'java', c: 'c', cpp: 'cpp', cs: 'csharp',
  rb: 'ruby', php: 'php', swift: 'swift', kt: 'kotlin',
  tf: 'hcl', Dockerfile: 'dockerfile',
}

function langFor(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  return EXT_LANG[ext] ?? 'plaintext'
}

function highlight(code: string, lang: string): string {
  try {
    if (lang !== 'plaintext') return hljs.highlight(code, { language: lang }).value
  } catch {}
  return hljs.highlightAuto(code).value
}

interface Props {
  projectId: string
  sessionId: string
}

export default function FileViewer({ projectId, sessionId }: Props) {
  const [entries, setEntries] = useState<FileEntry[]>([])
  const [path, setPath] = useState('')
  const [selected, setSelected] = useState<string | null>(null)
  const [content, setContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [contentLoading, setContentLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Load directory listing
  useEffect(() => {
    setLoading(true)
    setSelected(null)
    setContent(null)
    listFiles(projectId, sessionId, path)
      .then(setEntries)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [projectId, sessionId, path])

  const openFile = async (name: string) => {
    const fullPath = path ? `${path}/${name}` : name
    setSelected(fullPath)
    if (isImage(name)) {
      setContent('__image__')
      return
    }
    setContentLoading(true)
    setContent(null)
    setError(null)
    try {
      const text = await readFileContent(projectId, sessionId, fullPath)
      setContent(text)
    } catch (e: unknown) {
      setError((e as Error).message)
    } finally {
      setContentLoading(false)
    }
  }

  const breadcrumbs = path.split('/').filter(Boolean)

  return (
    <div className="flex h-full bg-[#0d1117]">
      {/* File tree */}
      <div className="w-52 shrink-0 flex flex-col border-r border-[#30363d] overflow-hidden">
        {/* Breadcrumb nav */}
        <div className="px-2 py-1.5 border-b border-[#21262d] flex items-center gap-1 text-[10px] text-[#8b949e] overflow-x-auto whitespace-nowrap">
          <button
            onClick={() => setPath('')}
            className="hover:text-[#e6edf3] shrink-0"
          >/</button>
          {breadcrumbs.map((seg, i) => (
            <span key={i} className="flex items-center gap-1 shrink-0">
              <span>/</span>
              <button
                onClick={() => setPath(breadcrumbs.slice(0, i + 1).join('/'))}
                className="hover:text-[#e6edf3]"
              >{seg}</button>
            </span>
          ))}
        </div>
        {/* Entries */}
        <div className="flex-1 overflow-y-auto">
          {loading && <div className="px-3 py-2 text-[10px] text-[#484f58]">Loading…</div>}
          {path && (
            <button
              onClick={() => setPath(breadcrumbs.slice(0, -1).join('/'))}
              className="w-full text-left px-3 py-1 text-[11px] text-[#8b949e] hover:bg-[#161b22] border-b border-[#21262d]"
            >↑ ..</button>
          )}
          {entries.map(e => (
            <button
              key={e.name}
              onClick={() => e.isDir ? setPath(path ? `${path}/${e.name}` : e.name) : openFile(e.name)}
              className={[
                'w-full text-left px-3 py-1 text-[11px] border-b border-[#21262d] hover:bg-[#161b22] transition-colors',
                selected === (path ? `${path}/${e.name}` : e.name) ? 'bg-[#21262d]' : '',
              ].join(' ')}
            >
              <span className={e.isDir ? 'text-[#58a6ff]' : 'text-[#e6edf3]'}>
                {e.isDir ? '📁' : '📄'} {e.name}
              </span>
              {!e.isDir && e.size > 0 && (
                <span className="ml-1 text-[#484f58] text-[9px]">
                  {e.size < 1024 ? `${e.size}b` : `${(e.size / 1024).toFixed(1)}k`}
                </span>
              )}
            </button>
          ))}
        </div>
      </div>

      {/* Content pane */}
      <div className="flex-1 overflow-auto">
        {!selected && (
          <div className="flex items-center justify-center h-full text-[#484f58] text-sm">
            Select a file to view
          </div>
        )}
        {selected && contentLoading && (
          <div className="flex items-center justify-center h-full text-[#484f58] text-sm">
            Loading…
          </div>
        )}
        {selected && error && (
          <div className="p-4 text-[#f85149] text-xs font-mono">{error}</div>
        )}
        {selected && content === '__image__' && (
          <div className="p-4 flex items-start justify-center">
            <img
              src={`/api/projects/${projectId}/sessions/${sessionId}/files/${selected}`}
              alt={selected}
              className="max-w-full rounded border border-[#30363d]"
            />
          </div>
        )}
        {selected && content !== null && content !== '__image__' && (
          <>
            <div className="flex items-center gap-2 px-3 py-1.5 bg-[#161b22] border-b border-[#30363d] sticky top-0">
              <span className="text-[10px] text-[#8b949e] font-mono truncate">{selected}</span>
              <span className="ml-auto text-[9px] text-[#484f58]">{langFor(selected.split('/').pop() ?? '')}</span>
            </div>
            <pre className="m-0 p-0 overflow-auto">
              <code
                className="block p-4 text-[12px] font-mono leading-relaxed"
                dangerouslySetInnerHTML={{
                  __html: highlight(content, langFor(selected.split('/').pop() ?? '')),
                }}
              />
            </pre>
          </>
        )}
      </div>
    </div>
  )
}
