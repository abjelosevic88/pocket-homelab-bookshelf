import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiError, V1 } from '../lib/api'
import type { Book, Candidate, Format, Status } from '../lib/types'
import { todayISO } from '../lib/format'
import { Field, Modal, Segmented, useToast } from './ui'
import { BookSearch, fromCandidate, withDetails } from './BookSearch'
import { RemoteCover } from './Cover'
import { Stars } from './Stars'
import { langName } from '../lib/languages'
import { BookFields, emptyBook, type BookDraft } from './BookForm'

type Step = { kind: 'search' } | { kind: 'pick'; c: Candidate; loading: boolean } | { kind: 'manual' }

/** Add a book: search the catalogues (or type it in), then say where it stands. */
/** A catalogue record for an Open Library work known only by key and title
 *  (series entries) — AddBook fetches the rest when it opens. */
export function workCandidate(w: { ol_work?: string; title: string; year?: string; cover_url?: string; thumb_url?: string }, author: string): Candidate | undefined {
  if (!w.ol_work) return undefined
  return { source: 'openlibrary', source_id: w.ol_work, source_url: 'https://openlibrary.org' + w.ol_work, title: w.title, subtitle: '', authors: author ? [author] : [], published: w.year || '', pages: 0, isbn13: '', isbn10: '', publisher: '', language: '', description: '', subjects: [], cover_url: w.cover_url || '', thumb_url: w.thumb_url || '' }
}

export function AddBook({ status: initialStatus = 'want', initial, query, extra, onClose, onAdded }: { status?: Status; initial?: Candidate; query?: string; extra?: Record<string, unknown>; onClose: () => void; onAdded?: (b: Book) => void }) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const [step, setStep] = useState<Step>({ kind: 'search' })
  const [draft, setDraft] = useState<BookDraft>(emptyBook())
  const [status, setStatus] = useState<Status>(initialStatus)
  const [finished, setFinished] = useState(todayISO())
  const [rating, setRating] = useState(0)
  const [format, setFormat] = useState<Format>('')
  const [tags, setTags] = useState('')
  const [planNote, setPlanNote] = useState('')
  const [dup, setDup] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  const started = useRef(false)
  useEffect(() => { if (initial && !started.current) { started.current = true; pick(initial) } }) // eslint-disable-line react-hooks/exhaustive-deps

  const pick = async (c: Candidate) => {
    setDup(c.on_shelf || null)
    setDraft({ ...emptyBook(), ...fromCandidate(c) })
    setStep({ kind: 'pick', c, loading: true })
    const full = await withDetails(c)
    setDraft(d => ({ ...d, description: d.description || full.description, authors: initial?.authors?.length ? d.authors : d.authors || (full.authors || []).join(', ') }))
    setStep({ kind: 'pick', c: full, loading: false })
  }

  const save = async () => {
    const coverUrl = step.kind === 'pick' ? step.c.cover_url : ''
    setBusy(true)
    try {
      const body: Record<string, unknown> = { ...draft, status, format, tags: tags || draft.tags, cover_url: coverUrl, allow_duplicate: dup !== null, ...(extra || {}) }
      if (status === 'read') { body.finished_on = finished; body.rating = rating }
      if (status === 'want') body.plan_note = planNote
      const r = await api.post<{ book: Book; warning?: string }>(`${V1}/books`, body)
      if (r.warning) toast(r.warning, 'err'); else toast(`Added “${r.book.title}”`)
      onAdded?.(r.book)
      onClose()
    } catch (e) {
      const err = e as ApiError
      if (err.status === 409 && err.data.book_id) { setDup(err.data.book_id as number); toast('Already on your shelf — press Add again to add a second copy', 'err') } else toast(err.message, 'err')
    } finally { setBusy(false) }
  }

  const subjects = step.kind === 'pick' ? step.c.subjects || [] : []
  const tagList = tags.split(',').map(t => t.trim()).filter(Boolean)
  const toggleTag = (t: string) => setTags(tagList.some(x => x.toLowerCase() === t.toLowerCase()) ? tagList.filter(x => x.toLowerCase() !== t.toLowerCase()).join(', ') : [...tagList, t].join(', '))

  const verdict = (
    <div className="grid" style={{ gap: 12 }}>
      <Field group label="Where does it stand?">
        <Segmented value={status} onChange={setStatus} options={[{ value: 'want', label: 'Want to read' }, { value: 'reading', label: 'Reading' }, { value: 'read', label: 'Read' }]} />
      </Field>
      {status === 'read' && (
        <div className="two-col tight">
          <Field label="Finished on"><input type="date" value={finished} max={todayISO()} onChange={e => setFinished(e.target.value)} /></Field>
          <Field group label="Rating"><Stars value={rating} onChange={setRating} /></Field>
        </div>
      )}
      {status === 'want' && <Field label="Why read it?" help="Who recommended it, what you hope to get out of it. Shown in the plan."><input value={planNote} onChange={e => setPlanNote(e.target.value)} maxLength={200} /></Field>}
      <div className="two-col tight">
        <Field group label="Format"><Segmented size="sm" value={format} onChange={setFormat} options={[{ value: '', label: '–' }, { value: 'paper', label: 'Paper' }, { value: 'ebook', label: 'E-book' }, { value: 'audio', label: 'Audio' }]} /></Field>
        <Field label="Tags"><input value={tags} onChange={e => setTags(e.target.value)} placeholder="sci-fi, book club" /></Field>
      </div>
      {subjects.length > 0 && <div className="chips">{subjects.map(s => <button key={s} type="button" className={`chip sm ${tagList.some(t => t.toLowerCase() === s.toLowerCase()) ? 'active' : ''}`} onClick={() => toggleTag(s)}>+ {s}</button>)}</div>}
    </div>
  )

  return (
    <Modal title={step.kind === 'manual' ? 'Add a book by hand' : initial ? 'Book details' : 'Add a book'} onClose={onClose} size="lg">
      {step.kind === 'search' && (
        <>
          <BookSearch onPick={pick} initial={query} />
          <div className="form-actions"><button className="btn ghost" onClick={() => setStep({ kind: 'manual' })}>Not found? Add it by hand</button></div>
        </>
      )}
      {step.kind === 'pick' && (
        <div className="pick">
          <div className="pick-head">
            <RemoteCover url={step.c.cover_url || step.c.thumb_url} title={step.c.title} size="md" />
            <div className="info">
              <h2>{draft.title}</h2>
              {draft.subtitle && <div className="muted">{draft.subtitle}</div>}
              {step.c.original_title && <div className="muted small">Original title: {step.c.original_title}</div>}
              <div className="authors">{draft.authors || 'Unknown author'}</div>
              <div className="meta muted small">{[draft.published, draft.pages ? `${draft.pages} pages` : '', draft.publisher, draft.isbn13, step.c.editions ? `${step.c.editions} editions` : ''].filter(Boolean).join(' · ')}</div>
              {step.c.languages && step.c.languages.length > 1 && <div className="meta muted small">Available in {step.c.languages.slice(0, 8).map(langName).join(', ')}{step.c.languages.length > 8 ? ` and ${step.c.languages.length - 8} more` : ''}</div>}
              {step.c.source_url && <a className="small" href={step.c.source_url} target="_blank" rel="noreferrer">{step.c.source === 'google' ? 'Google Books' : 'Open Library'} ↗</a>}
              {step.loading ? <p className="muted small mt-sm"><span className="spinner sm" /> Fetching description…</p> : draft.description && <p className="desc small">{draft.description}</p>}
              {dup && <div className="callout mt-sm small">Already on your shelf. <a href={`/books/${dup}`} onClick={e => { e.preventDefault(); onClose(); navigate(`/books/${dup}`) }}>Open it</a> — or add another copy.</div>}
            </div>
          </div>
          <hr className="mt mb" />
          {verdict}
          <div className="form-actions">
            {initial ? <button className="btn ghost" onClick={onClose}>Cancel</button> : <button className="btn ghost" onClick={() => setStep({ kind: 'search' })}>← Back to results</button>}
            <button className="btn primary" onClick={save} disabled={busy || step.loading}>{busy ? 'Adding…' : 'Add to shelf'}</button>
          </div>
        </div>
      )}
      {step.kind === 'manual' && (
        <div className="grid" style={{ gap: 14 }}>
          <BookFields draft={draft} onChange={setDraft} />
          <hr />
          {verdict}
          <div className="form-actions">
            <button className="btn ghost" onClick={() => setStep({ kind: 'search' })}>← Search instead</button>
            <button className="btn primary" onClick={save} disabled={busy || !draft.title.trim()}>{busy ? 'Adding…' : 'Add to shelf'}</button>
          </div>
        </div>
      )}
    </Modal>
  )
}
