import { useEffect, useState } from 'react'
import { api, ApiError, V1 } from '../lib/api'
import type { Book } from '../lib/types'
import { Confirm, Spinner, useAsync, useToast } from './ui'
import { Icon } from './icons'

// The same book in Pocket Library (through Pocket Core): open it there, or
// link it by hand when the reading sync didn't match it, or unlink it.

interface Hit { id: number; title: string; authors: string; cover_url: string; series?: string; series_index?: number; language?: string; url: string }

/** The "Pocket Library" box on the book page. */
export function LibraryLinkBox({ book, onChange }: { book: Book; onChange: () => void }) {
  const { toast } = useToast()
  const linked = book.library_book_id > 0
  const { data: status } = useAsync(() => linked ? Promise.resolve({ core: true }) : api.get<{ core: boolean }>(`${V1}/library`), [linked])
  const [picking, setPicking] = useState(false)
  const [unlinking, setUnlinking] = useState(false)
  if (!linked && !status?.core) return null

  const unlink = async () => {
    setUnlinking(false)
    try {
      await api.del(`${V1}/library/links/${book.library_book_id}`)
      toast('Unlinked from Pocket Library', 'ok')
      onChange()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="get-it lib-link">
      <div className="get-it-head">
        <span className="get-it-title">Pocket Library</span>
      </div>
      {linked ? (
        <div className="get-it-row">
          <span className="get-it-kind"><Icon name="library" size={14} />In your Pocket Library</span>
          <span className="row">
            {book.library_url && <a className="btn sm" href={book.library_url} target="_blank" rel="noreferrer"><Icon name="external" size={13} />Open</a>}
            <button type="button" className="btn ghost sm" onClick={() => setUnlinking(true)} title="Unlink from Pocket Library">Unlink</button>
          </span>
        </div>
      ) : picking ? (
        <LibraryPicker book={book} onDone={() => { setPicking(false); onChange() }} onCancel={() => setPicking(false)} />
      ) : (
        <div className="get-it-row">
          <span className="get-it-kind"><Icon name="library" size={14} />Not linked</span>
          <button type="button" className="btn sm" onClick={() => setPicking(true)}><Icon name="link" size={13} />Link…</button>
        </div>
      )}
      {unlinking && <Confirm title="Unlink from Pocket Library?" confirmLabel="Unlink"
        message="Reading it in Pocket Library won't update this book any more, and the book won't be matched again. You can link it again later."
        onConfirm={unlink} onCancel={() => setUnlinking(false)} />}
    </div>
  )
}

function LibraryPicker({ book, onDone, onCancel }: { book: Book; onDone: () => void; onCancel: () => void }) {
  const { toast } = useToast()
  const [q, setQ] = useState(book.title)
  const [hits, setHits] = useState<Hit[] | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    const term = q.trim()
    if (!term) { setHits([]); return }
    let live = true
    const t = setTimeout(async () => {
      setBusy(true)
      try {
        const r = await api.get<Hit[]>(`${V1}/library/search?q=${encodeURIComponent(term)}`)
        if (live) { setHits(r); setErr('') }
      } catch (e) {
        if (live) setErr(e instanceof ApiError ? e.message : 'Pocket Library did not answer')
      } finally { if (live) setBusy(false) }
    }, 300)
    return () => { live = false; clearTimeout(t) }
  }, [q])
  const pick = async (h: Hit) => {
    try {
      await api.put(`${V1}/library/links/${h.id}`, { book_id: book.id })
      toast(`Linked to “${h.title}” in Pocket Library`, 'ok')
      onDone()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="lib-pick">
      <div className="row">
        <input value={q} onChange={e => setQ(e.target.value)} placeholder="Title or author in Pocket Library" autoFocus aria-label="Search Pocket Library" />
        <button type="button" className="btn ghost sm icon-only" onClick={onCancel} title="Cancel"><Icon name="x" size={14} /></button>
      </div>
      {busy && !hits && <Spinner size={12} />}
      {err ? <p className="core-hint">{err}</p> : hits && (hits.length ? (
        <ul className="lib-hits">
          {hits.map(h => (
            <li key={h.id}>
              <button type="button" className="lib-hit" onClick={() => pick(h)} title="Link to this book">
                {h.cover_url ? <img src={h.cover_url} alt="" loading="lazy" /> : <span className="lib-hit-ph" />}
                <span className="lib-hit-text">
                  <span className="lib-hit-title">{h.title}</span>
                  <span className="muted small">{[h.authors, h.series && `${h.series}${h.series_index ? ' #' + h.series_index : ''}`, h.language].filter(Boolean).join(' · ')}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      ) : <p className="core-hint">Nothing in Pocket Library matches.</p>)}
    </div>
  )
}
