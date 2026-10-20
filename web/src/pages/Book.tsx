import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Book as BookT, Quote, Read, Status } from '../lib/types'
import { daysBetween, fmtDate, fmtMonth, FORMAT_LABEL, progress, splitTags, STATUS_LABEL, todayISO, pageLabel } from '../lib/format'
import { Card, Confirm, Field, Loading, Modal, Segmented, useAsync, useToast } from '../components/ui'
import { Cover } from '../components/Cover'
import { AuthorLink } from '../components/AuthorBits'
import { useApp } from '../lib/app-context'
import { Markdown } from '../components/Markdown'
import { MarkdownEditor } from '../components/MarkdownEditor'
import { Stars } from '../components/Stars'
import { EditBook } from '../components/BookForm'
import { CoverPicker } from '../components/CoverPicker'
import { FinishModal } from '../components/FinishModal'
import { CharactersCard } from '../components/Characters'
import { NotesCard } from '../components/NotesCard'

interface Detail { book: BookT; reads: Read[]; quotes: Quote[] }

export default function Book() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data, error, setData, reload } = useAsync(() => api.get<Detail>(`${V1}/books/${id}`), [id])
  const [editing, setEditing] = useState(false)
  const [finishing, setFinishing] = useState<'read' | 'abandoned' | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [coverOpen, setCoverOpen] = useState(false)

  if (error) return <div className="card pad"><p>{error}</p><Link to="/library">← Library</Link></div>
  if (!data) return <Loading />
  const b = data.book
  const setBook = (nb: BookT) => setData({ ...data, book: nb })

  const patch = async (fields: Partial<BookT>, msg?: string) => {
    setBook({ ...b, ...fields })
    try { const r = await api.put<{ book: BookT }>(`${V1}/books/${b.id}`, fields); setBook(r.book); if (msg) toast(msg) } catch (e) { toast((e as Error).message, 'err'); reload() }
  }
  const changeStatus = async (s: Status) => {
    if (s === b.status) return
    if (s === 'read' || s === 'abandoned') { setFinishing(s); return }
    try { await api.post(`${V1}/books/${b.id}/status`, { status: s }); toast(STATUS_LABEL[s]); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const del = async () => { await api.del(`${V1}/books/${b.id}`); toast(`Deleted “${b.title}”`); navigate('/library') }

  return (
    <div className="book-page">
      <div className="book-side">
        <button type="button" className="cover-change" onClick={() => setCoverOpen(true)} title="Change cover">
          <Cover book={b} size="lg" />
          <span className="cover-change-label">Change cover</span>
        </button>
        <Segmented value={b.status} onChange={changeStatus} options={(['want', 'reading', 'read', 'abandoned'] as Status[]).map(s => ({ value: s, label: s === 'want' ? 'Want' : s === 'abandoned' ? 'Stopped' : STATUS_LABEL[s] }))} />
        <dl className="facts">
          {b.pages > 0 && <><dt>Pages</dt><dd>{b.pages}</dd></>}
          {b.published && <><dt>Published</dt><dd>{b.published}</dd></>}
          {b.publisher && <><dt>Publisher</dt><dd>{b.publisher}</dd></>}
          {b.format && <><dt>Format</dt><dd>{FORMAT_LABEL[b.format]}</dd></>}
          {b.language && <><dt>Language</dt><dd>{b.language}</dd></>}
          {b.isbn13 && <><dt>ISBN</dt><dd className="mono">{b.isbn13}</dd></>}
          <dt>Added</dt><dd>{fmtDate(b.created_at.slice(0, 10))}</dd>
          {b.source_url && <><dt>Source</dt><dd><a href={b.source_url} target="_blank" rel="noreferrer">{b.source === 'google' ? 'Google Books' : 'Open Library'} ↗</a></dd></>}
        </dl>
        <div className="row wrap">
          <button className="btn sm" onClick={() => setEditing(true)}>Edit details</button>
          <button className="btn ghost sm danger-text" onClick={() => setDeleting(true)}>Delete</button>
        </div>
      </div>

      <div className="book-main">
        <Link to="/library" className="muted small">← Library</Link>
        <h1 className="book-title">{b.title}</h1>
        {b.subtitle && <div className="book-subtitle">{b.subtitle}</div>}
        <div className="book-authors">{splitTags(b.authors).map((a, i) => <span key={a}>{i > 0 && ', '}<AuthorLink name={a} /></span>)}</div>
        {b.series && <div className="book-series"><Link to={`/series/${encodeURIComponent(b.series)}`}>📚 {b.series}{b.series_no !== '' && <> · #{b.series_no}</>}</Link></div>}
        <div className="row wrap mt-sm">
          <Stars value={b.rating} onChange={v => patch({ rating: v }, v ? `Rated ${v}★` : 'Rating cleared')} size="lg" />
          <button className={`fav-btn ${b.favorite ? 'on' : ''}`} onClick={() => patch({ favorite: !b.favorite })} title={b.favorite ? 'Favourite' : 'Mark as favourite'}>♥</button>
          {splitTags(b.tags).map(t => <Link key={t} to={`/library?tag=${encodeURIComponent(t)}`} className="badge tag">{t}</Link>)}
        </div>

        {b.status === 'reading' && <ProgressCard book={b} onSaved={setBook} onFinish={() => setFinishing('read')} />}
        {b.status === 'want' && <PlanCard book={b} onSave={patch} onStart={() => changeStatus('reading')} />}

        <ReviewCard book={b} onSave={review => patch({ review }, 'Thoughts saved')} />
        <NotesCard src={`${V1}/books/${b.id}/notes`} />
        <CharactersCard bookId={b.id} />
        <QuotesCard book={b} quotes={data.quotes} onChanged={reload} />
        <ReadsCard book={b} reads={data.reads} onChanged={reload} />
        {b.description && <Card title="About the book" className="mt"><Description text={b.description} /></Card>}
      </div>

      {coverOpen && <CoverPicker book={b} onClose={() => setCoverOpen(false)} onChanged={nb => setBook({ ...b, cover: nb.cover })} />}
      {editing && <EditBook book={b} onClose={() => setEditing(false)} onSaved={nb => { setBook(nb); setEditing(false) }} />}
      {finishing && <FinishModal book={b} abandon={finishing === 'abandoned'} onClose={() => setFinishing(null)} onDone={() => { setFinishing(null); reload() }} />}
      {deleting && <Confirm title={`Delete “${b.title}”?`} message="The book, your rating, thoughts, quotes and reading history are removed for good." onCancel={() => setDeleting(false)} onConfirm={del} />}
    </div>
  )
}

function ProgressCard({ book, onSaved, onFinish }: { book: BookT; onSaved: (b: BookT) => void; onFinish: () => void }) {
  const { toast } = useToast()
  const [page, setPage] = useState(String(book.current_page || ''))
  useEffect(() => setPage(String(book.current_page || '')), [book.current_page])
  const pct = progress(book)
  const save = async () => {
    const n = Number(page)
    if (!Number.isFinite(n) || n === book.current_page) return
    try { onSaved(await api.post<BookT>(`${V1}/books/${book.id}/progress`, { page: n })) } catch (e) { toast((e as Error).message, 'err') }
  }
  const days = book.started_on ? daysBetween(book.started_on, todayISO()) : null
  return (
    <Card className="mt" title="Progress" actions={<button className="btn primary sm" onClick={onFinish}>✓ Finished</button>}>
      {book.pages > 0 && <div className="bar lg mb-sm"><span className="fill" style={{ width: `${pct}%` }} /></div>}
      <div className="row wrap">
        <label className="page-input"><span className="muted small">Page</span>
          <input type="number" min={0} max={book.pages || undefined} value={page} onChange={e => setPage(e.target.value)} onBlur={save} onKeyDown={e => e.key === 'Enter' && (e.target as HTMLInputElement).blur()} />
          {book.pages > 0 ? <span className="muted small">of {book.pages} · {pct}%</span> : <span className="muted small">(add the page count under Edit details for a percentage)</span>}
        </label>
        <span className="spacer" />
        {days !== null && book.started_on && <span className="muted small">Started {fmtDate(book.started_on)} · day {days + 1}</span>}
      </div>
    </Card>
  )
}

function PlanCard({ book, onSave, onStart }: { book: BookT; onSave: (f: Partial<BookT>, msg?: string) => void; onStart: () => void }) {
  const [note, setNote] = useState(book.plan_note)
  return (
    <Card className="mt" title="In your plan" actions={<button className="btn primary sm" onClick={onStart}>Start reading</button>}>
      <div className="two-col tight">
        <Field label="Planned for" help={book.planned_for ? fmtMonth(book.planned_for) : 'No month: it waits in “Up next”.'}>
          <input type="month" value={book.planned_for} onChange={e => onSave({ planned_for: e.target.value }, 'Plan updated')} />
        </Field>
        <Field label="Why read it?"><input value={note} onChange={e => setNote(e.target.value)} onBlur={() => note !== book.plan_note && onSave({ plan_note: note }, 'Saved')} placeholder="Recommended by…" /></Field>
      </div>
    </Card>
  )
}

function ReviewCard({ book, onSave }: { book: BookT; onSave: (review: string) => void }) {
  const { settings } = useApp()
  const templates = settings?.thought_templates || []
  const [text, setText] = useState(book.review)
  const [editing, setEditing] = useState(!book.review)
  useEffect(() => { setText(book.review); setEditing(!book.review) }, [book.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const dirty = text !== book.review
  const cancel = () => { setText(book.review); setEditing(!book.review) }
  const save = () => { if (dirty) { onSave(text); setEditing(!text) } }
  const buttons = (
    <>
      {book.review && <button className="btn ghost sm" onClick={cancel}>Cancel</button>}
      <button className="btn primary sm" disabled={!dirty} onClick={save} title="Save (Ctrl/Cmd+S)">Save</button>
    </>
  )
  return (
    <Card className="mt" title="My thoughts" actions={editing ? buttons : <button className="btn ghost sm" onClick={() => setEditing(true)}>Edit</button>}>
      {editing ? (
        <div onKeyDown={e => {
          if ((e.metaKey || e.ctrlKey) && e.key === 's') { e.preventDefault(); save() }
          else if (e.key === 'Escape' && !e.defaultPrevented && book.review && !dirty) cancel()
        }}>
          {!text.trim() && templates.length > 0 && (
            <div className="row wrap mb-sm template-pick"><span className="muted small">Start from:</span>
              {templates.map(t => <button key={t.name} type="button" className="chip sm" onClick={() => setText(t.body)}>{t.name}</button>)}
            </div>
          )}
          <MarkdownEditor className="review-input" value={text} onChange={setText} autoFocus={!!book.review} rows={text.split('\n').length > 7 ? Math.min(28, text.split('\n').length + 2) : 7}
            placeholder={book.status === 'want' ? 'What do you expect from it?' : 'What stayed with you? What did you like, what not? Would you recommend it — to whom?'} />
          {text.split('\n').length > 12 && <div className="form-actions">{buttons}</div>}
        </div>
      ) : <div className="review" onDoubleClick={() => setEditing(true)}><ReviewText text={book.review} /></div>}
    </Card>
  )
}

/** Thoughts as written: "## " lines are section titles, "### " and "#### "
 *  sub-titles. Sections left empty (a template's unused headings, bare "1."
 *  prompts) are hidden; long thoughts open with "Show all". */
function ReviewText({ text }: { text: string }) {
  const [all, setAll] = useState(false)
  if (!/^#{2,4} /m.test(text)) return <Markdown text={text} />
  const blocks: { level: number; title: string; body: string }[] = []
  for (const line of text.split('\n')) {
    const m = line.match(/^(#{2,4}) (.*)$/)
    if (m) blocks.push({ level: m[1].length, title: m[2], body: '' })
    else if (blocks.length) blocks[blocks.length - 1].body += line + '\n'
    else blocks.push({ level: 0, title: '', body: line + '\n' })
  }
  const filled = (b: { body: string }) => b.body.split('\n').some(l => l.trim() && !/^\s*(\d+\.|[-*])\s*$/.test(l))
  // A title stays when it, or something nested under it, has text.
  const keep = blocks.map((b, i) => {
    if (filled(b)) return true
    for (let j = i + 1; j < blocks.length && blocks[j].level > b.level && b.level > 0; j++) if (filled(blocks[j])) return true
    return false
  })
  // Long notes: show the first ~1800 characters' worth of sections.
  let shown = blocks.length, size = 0
  if (!all && text.length > 2600) {
    for (let i = 0; i < blocks.length; i++) {
      size += blocks[i].body.length
      if (size > 1800) { shown = Math.max(1, i + 1); break }
    }
  }
  const words = text.split(/\s+/).length
  return <>
    {blocks.slice(0, shown).map((b, i) => keep[i] && (
      <div key={i} className={`review-section l${b.level}`}>
        {b.title && (b.level === 4 ? <h6>{b.title}</h6> : b.level === 3 ? <h5>{b.title}</h5> : <h4>{b.title}</h4>)}
        {filled(b) && <Markdown text={b.body} />}
      </div>
    ))}
    {shown < blocks.length && <button className="btn ghost sm read-more mt-sm" onClick={() => setAll(true)}>Show all · {Math.max(1, Math.round(words / 230))} min ▾</button>}
    {all && text.length > 2600 && <button className="btn ghost sm read-more mt-sm" onClick={() => setAll(false)}>Show less ▴</button>}
  </>
}

function QuotesCard({ book, quotes, onChanged }: { book: BookT; quotes: Quote[]; onChanged: () => void }) {
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<Quote> | null>(null)
  const [del, setDel] = useState<Quote | null>(null)
  return (
    <Card className="mt" title={`Quotes${quotes.length ? ` (${quotes.length})` : ''}`} actions={<button className="btn ghost sm" onClick={() => setEdit({})}>+ Add quote</button>}>
      {quotes.length === 0 ? <p className="muted">Passages worth keeping. Add one while you read.</p> : (
        <div className="quotes">
          {quotes.map(q => (
            <figure key={q.id} className="quote">
              <blockquote>{q.text}</blockquote>
              <figcaption>{q.page && <span className="muted">{pageLabel(q.page)}</span>}{q.note && <span className="note">{q.note}</span>}
                <span className="spacer" /><button className="btn ghost sm" onClick={() => setEdit(q)}>Edit</button><button className="btn ghost sm danger-text" onClick={() => setDel(q)}>✕</button>
              </figcaption>
            </figure>
          ))}
        </div>
      )}
      {edit && <QuoteModal bookId={book.id} quote={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); onChanged() }} />}
      {del && <Confirm title="Delete this quote?" message={<i>“{del.text.slice(0, 120)}{del.text.length > 120 ? '…' : ''}”</i>} onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/quotes/${del.id}`); setDel(null); toast('Quote deleted'); onChanged() }} />}
    </Card>
  )
}

export function QuoteModal({ bookId, quote, onClose, onSaved }: { bookId: number; quote: Partial<Quote>; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ text: quote.text || '', page: quote.page || '', note: quote.note || '' })
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      if (quote.id) await api.put(`${V1}/quotes/${quote.id}`, f); else await api.post(`${V1}/books/${bookId}/quotes`, f)
      toast('Quote saved'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Modal title={quote.id ? 'Edit quote' : 'Add quote'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 12 }}>
        <Field label="Quote"><textarea rows={5} value={f.text} onChange={e => setF({ ...f, text: e.target.value })} required autoFocus /></Field>
        <div className="two-col tight">
          <Field label="Page / location"><input value={f.page} onChange={e => setF({ ...f, page: e.target.value })} placeholder="142" maxLength={20} /></Field>
          <Field label="Your note"><input value={f.note} onChange={e => setF({ ...f, note: e.target.value })} placeholder="Why it matters" /></Field>
        </div>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Save</button></div>
      </form>
    </Modal>
  )
}

function ReadsCard({ book, reads, onChanged }: { book: BookT; reads: Read[]; onChanged: () => void }) {
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<Read> | null>(null)
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!edit) return
    try {
      if (edit.id) await api.put(`${V1}/reads/${edit.id}`, edit); else await api.post(`${V1}/books/${book.id}/reads`, edit)
      setEdit(null); toast('Saved'); onChanged()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const remove = async (r: Read) => { await api.del(`${V1}/reads/${r.id}`); toast('Removed'); onChanged() }
  const open = (r: Read) => !r.finished_on && !r.abandoned
  return (
    <Card className="mt" title="Reading history" actions={<button className="btn ghost sm" onClick={() => setEdit({ started_on: '', finished_on: todayISO(), abandoned: false })}>+ Add a read</button>}>
      {reads.length === 0 ? <p className="muted">{book.status === 'read' ? 'Marked as read without a date. Add when you read it to count it in that year’s stats.' : 'Each time you read this book shows up here.'}</p> : (
        <ul className="reads">
          {reads.map(r => (
            <li key={r.id}>
              <span className={`dot ${r.abandoned ? 'ab' : open(r) ? 'open' : ''}`} />
              <span>{r.started_on ? fmtDate(r.started_on) : '?'} → {open(r) ? <i>reading</i> : fmtDate(r.finished_on)}{r.abandoned && <span className="muted"> · stopped</span>}
                {r.started_on && r.finished_on && <span className="muted small"> · {daysBetween(r.started_on, r.finished_on) + 1} days</span>}</span>
              <span className="spacer" />
              <button className="btn ghost sm" onClick={() => setEdit(r)}>Edit</button>
              {!open(r) && <button className="btn ghost sm danger-text" onClick={() => remove(r)}>✕</button>}
            </li>
          ))}
        </ul>
      )}
      {edit && (
        <Modal title={edit.id ? 'Edit read' : 'Add a read'} onClose={() => setEdit(null)}>
          <form onSubmit={save} className="grid" style={{ gap: 12 }}>
            <div className="two-col tight">
              <Field label="Started"><input type="date" value={edit.started_on || ''} max={todayISO()} onChange={e => setEdit({ ...edit, started_on: e.target.value })} /></Field>
              <Field label="Finished"><input type="date" value={edit.finished_on || ''} max={todayISO()} onChange={e => setEdit({ ...edit, finished_on: e.target.value })} required={!edit.id} /></Field>
            </div>
            <label className="row"><input type="checkbox" checked={!!edit.abandoned} onChange={e => setEdit({ ...edit, abandoned: e.target.checked })} style={{ width: 'auto' }} /> Stopped before the end</label>
            <div className="form-actions"><button type="button" className="btn" onClick={() => setEdit(null)}>Cancel</button><button className="btn primary">Save</button></div>
          </form>
        </Modal>
      )}
    </Card>
  )
}

function Description({ text }: { text: string }) {
  const [open, setOpen] = useState(text.length < 600)
  return (
    <div className={`description ${open ? '' : 'clamped'}`}>
      {text}
      {!open && <button className="btn ghost sm more-btn" onClick={() => setOpen(true)}>Read more</button>}
    </div>
  )
}
