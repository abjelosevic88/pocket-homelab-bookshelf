import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Candidate } from '../lib/types'
import { Segmented, useDebounce } from './ui'
import { RemoteCover } from './Cover'

const SOURCE_LABEL: Record<string, string> = { all: 'All', openlibrary: 'Open Library', google: 'Google Books' }

/** Search box + results from the public catalogues. */
export function BookSearch({ initial = '', onPick }: { initial?: string; onPick: (c: Candidate) => void }) {
  const { settings } = useApp()
  const sources = settings?.sources || ['openlibrary']
  const [q, setQ] = useState(initial)
  const [source, setSource] = useState<string>(() => (settings?.search_source && (settings.search_source === 'all' || sources.includes(settings.search_source)) ? settings.search_source : 'all'))
  const [results, setResults] = useState<Candidate[] | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
    const [busy, setBusy] = useState(false)
  const dq = useDebounce(q.trim(), 450)

  useEffect(() => {
    if (dq.length < 3) { setResults(null); return } // catalogues refuse shorter queries
    let live = true
    setBusy(true)
    api.get<{ results: Candidate[]; warnings: string[] | null }>(`${V1}/search?q=${encodeURIComponent(dq)}&source=${source}`)
      .then(r => { if (live) { setResults(r.results); setWarnings(r.warnings || []) } })
      .catch(() => { if (live) { setResults([]); setWarnings([]) } }) // a failed lookup reads as “no results”
      .finally(() => live && setBusy(false))
    return () => { live = false }
  }, [dq, source])

  return (
    <div className="book-search">
      <div className="search-bar">
        <span className="ico">⌕</span>
        <input value={q} onChange={e => setQ(e.target.value)} placeholder="Title, author or ISBN" autoFocus />
        {busy && <span className="spinner sm" />}
      </div>
      {sources.length > 1 && (
        <div className="row mt-sm"><Segmented size="sm" value={source} onChange={setSource} options={['all', ...sources].map(s => ({ value: s, label: SOURCE_LABEL[s] || s }))} /></div>
      )}
      {results && results.length === 0 && !busy && <p className="muted mt">No results. Try other words, the ISBN, or add it by hand.</p>}
      {results && results.length > 0 && warnings.length > 0 && <p className="muted small mt-sm">Some results may be missing: one catalogue did not answer.</p>}
      {results && results.length > 0 && (
        <ul className="results">
          {results.map(c => (
            <li key={c.source + c.source_id}>
              <button type="button" onClick={() => onPick(c)}>
                <RemoteCover url={c.thumb_url} title={c.title} size="xs" />
                <span className="info">
                  <span className="nm">{c.title}{c.subtitle && <span className="muted">: {c.subtitle}</span>}</span>
                  <span className="meta">{(c.authors || []).join(', ') || 'Unknown author'}{c.published && <> · {c.published}</>}{c.pages > 0 && <> · {c.pages} p.</>}</span>
                </span>
                <span className="tags">
                  {c.on_shelf ? <span className="badge cat">on shelf</span> : null}
                  <span className="badge">{c.source === 'google' ? 'Google' : 'Open Library'}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {!results && q.trim().length > 0 && q.trim().length < 3 && <p className="muted small mt">Keep typing…</p>}
      {!results && q.trim().length === 0 && <p className="muted small mt">Searches {sources.map(s => SOURCE_LABEL[s]).join(' and ')}.{!sources.includes('google') && ' Add a free Google Books API key under Settings → Metadata for a second catalogue.'}</p>}
    </div>
  )
}

/** Book fields filled from a search result (metadata only). */
export function fromCandidate(c: Candidate) {
  return {
    title: c.title, subtitle: c.subtitle, authors: (c.authors || []).join(', '), isbn13: c.isbn13, isbn10: c.isbn10, pages: c.pages,
    published: c.published, publisher: c.publisher, language: c.language, description: c.description,
    source: c.source, source_id: c.source_id, source_url: c.source_url,
  }
}

/** Fetches the full record (description, subjects) of a picked result. */
export async function withDetails(c: Candidate): Promise<Candidate> {
  if (c.description && c.subjects?.length) return c
  try {
    const d = await api.get<Candidate>(`${V1}/search/details?source=${c.source}&id=${encodeURIComponent(c.source_id)}`)
    return { ...c, description: c.description || d.description, subjects: c.subjects?.length ? c.subjects : d.subjects, cover_url: c.cover_url || d.cover_url }
  } catch { return c }
}
