import { useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import type { Book, Candidate, Status } from '../lib/types'
import { Modal, useAsync, useToast } from './ui'
import { Select } from './Select'

interface Entry { text: string; note: string; page: string; loc_start: number; loc_end: number; added: string; note_only: boolean }
interface PreviewBook {
  title: string
  author: string
  entries: Entry[]
  highlights: number
  notes: number
  known: number
  match: { book_id: number; title: string; by: 'title' | 'openlibrary' } | null
  candidate: Candidate | null
}
interface Choice { target: string; status: Status } // target: book id, "new" or "skip"
interface Result { books_created: number; books_updated: number; added: number; updated: number; skipped: number }

/** Upload "My Clippings.txt", check where each Kindle book goes, import. */
export function KindleImport({ onClose, onDone }: { onClose: () => void; onDone?: () => void }) {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const [preview, setPreview] = useState<PreviewBook[] | null>(null)
  const [clippings, setClippings] = useState(0)
  const [choices, setChoices] = useState<Choice[]>([])
  const [result, setResult] = useState<Result | null>(null)
  const shelf = useAsync(() => api.get<Book[]>(`${V1}/books?sort=title`), [])

  const read = async (file: File) => {
    setBusy(true)
    try {
      const r = await upload<{ books: PreviewBook[]; clippings: number }>(`${V1}/import/kindle/preview`, 'file', file)
      setPreview(r.books)
      setClippings(r.clippings)
      setChoices(r.books.map(b => ({ target: b.match ? String(b.match.book_id) : 'new', status: 'read' })))
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }

  const set = (i: number, c: Partial<Choice>) => setChoices(cs => cs.map((x, j) => j === i ? { ...x, ...c } : x))

  const run = async () => {
    if (!preview) return
    setBusy(true)
    try {
      const books = preview.map((b, i) => {
        const c = choices[i]
        if (c.target === 'skip') return { target: 'skip' }
        if (c.target === 'new') return { target: 'new', status: c.status, title: b.title, author: b.author, candidate: b.candidate, entries: b.entries }
        return { target: 'book', book_id: Number(c.target), entries: b.entries }
      })
      const r = await api.post<Result>(`${V1}/import/kindle`, { books })
      setResult(r)
      onDone?.()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }

  const chosen = preview ? preview.filter((_, i) => choices[i]?.target !== 'skip') : []
  const total = chosen.reduce((n, b) => n + b.entries.length, 0)
  const fresh = preview ? preview.reduce((n, b, i) => n + (choices[i]?.target === 'skip' ? 0 : choices[i]?.target === 'new' ? b.entries.length : (b.match && String(b.match.book_id) === choices[i]?.target ? b.entries.length - b.known : b.entries.length)), 0) : 0
  const newBooks = choices.filter(c => c.target === 'new').length

  if (result) return (
    <Modal title="Kindle highlights imported" onClose={onClose}>
      <p><b>{result.added}</b> new highlights{result.updated > 0 && <>, <b>{result.updated}</b> updated (longer version or note added)</>}{result.skipped > 0 && <>, {result.skipped} already there</>}.</p>
      {result.books_created > 0 && <p>{result.books_created} {result.books_created === 1 ? 'book was' : 'books were'} added to your shelf.</p>}
      <p className="muted small">Importing the same file again later only adds what is new.</p>
      <div className="form-actions"><button className="btn primary" onClick={onClose}>Done</button></div>
    </Modal>
  )

  return (
    <Modal title="Import Kindle highlights" onClose={onClose} size="lg">
      {!preview ? (
        <div className="grid" style={{ gap: 14 }}>
          <p>Your Kindle keeps every highlight and note in one file, <code>My Clippings.txt</code>. Connect the Kindle to a computer with a USB cable and you find it in the <code>documents</code> folder.</p>
          <p className="muted small">Each Kindle book is matched to your shelf by title, and otherwise by looking it up in Open Library and comparing with the books you have (so an English Kindle edition finds your translated copy). You check every match before anything is saved. Bookmarks are ignored, notes are attached to their highlight, and highlights you changed on the Kindle keep only the last version.</p>
          <label className="btn primary lg" style={{ alignSelf: 'start' }}>
            {busy ? <><span className="spinner sm" /> Reading and matching…</> : 'Choose My Clippings.txt…'}
            <input type="file" accept=".txt,text/plain" hidden disabled={busy} onChange={e => e.target.files?.[0] && read(e.target.files[0])} />
          </label>
        </div>
      ) : (
        <>
          <p className="muted">{clippings} clippings from {preview.length} {preview.length === 1 ? 'book' : 'books'}. Check where each book's highlights should go.</p>
          <div className="kindle-list">
            {preview.map((b, i) => {
              const c = choices[i]
              return (
                <div key={i} className={`kindle-row ${c.target === 'skip' ? 'skipped' : ''}`}>
                  <div className="info">
                    <div className="nm">{b.title}</div>
                    <div className="muted small">{b.author || 'Unknown author'} · {b.highlights} {b.highlights === 1 ? 'highlight' : 'highlights'}{b.notes > 0 && <> · {b.notes} {b.notes === 1 ? 'note' : 'notes'}</>}
                      {b.match && String(b.match.book_id) === c.target && b.known > 0 && <> · <span className="ok">{b.known} already saved</span></>}</div>
                    <div className="sample small">“{b.entries[0]?.text.slice(0, 140)}{(b.entries[0]?.text.length || 0) > 140 ? '…' : ''}”</div>
                  </div>
                  <div className="target">
                    <Select value={c.target} onChange={v => set(i, { target: v })} ariaLabel="Import into" options={[
                      ...(b.match ? [{ value: String(b.match.book_id), label: `→ ${b.match.title}`, hint: b.match.by === 'openlibrary' ? 'same Open Library work' : 'same title' }] : []),
                      { value: 'new', label: `＋ New book${b.candidate ? `: ${b.candidate.title}` : ''}`, hint: b.candidate ? 'from Open Library' : undefined },
                      { value: 'skip', label: '— Skip —' },
                      ...(shelf.data || []).filter(s => s.id !== b.match?.book_id).map(s => ({ value: String(s.id), label: s.title, hint: s.authors, group: 'Another book on your shelf' })),
                    ]} />
                    {c.target === 'new' && (
                      <Select value={c.status} onChange={v => set(i, { status: v as Status })} ariaLabel="Status of the new book"
                        options={[{ value: 'read', label: 'as Read' }, { value: 'reading', label: 'as Reading' }, { value: 'want', label: 'as Want to read' }]} />
                    )}
                  </div>
                </div>
              )
            })}
          </div>
          <div className="form-actions">
            <span className="muted small spacer">{fresh} new of {total} highlights{newBooks > 0 && ` · ${newBooks} new ${newBooks === 1 ? 'book' : 'books'}`}</span>
            <button className="btn ghost" onClick={() => setPreview(null)} disabled={busy}>← Other file</button>
            <button className="btn primary" onClick={run} disabled={busy || total === 0}>{busy ? 'Importing…' : `Import ${total} highlights`}</button>
          </div>
        </>
      )}
    </Modal>
  )
}
