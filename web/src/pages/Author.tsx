import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, upload, V1 } from '../lib/api'
import type { Author as AuthorT, AuthorCandidate, Book, Candidate, Status } from '../lib/types'
import { fmtDate, lifespan } from '../lib/format'
import { Card, Confirm, Field, Loading, Modal, useAsync, useToast } from '../components/ui'
import { AuthorPhoto } from '../components/AuthorBits'
import { BookRow } from '../components/BookCard'
import { RemoteCover } from '../components/Cover'
import { withDetails } from '../components/BookSearch'
import { FindAuthor } from './Authors'
import { AddBook } from '../components/AddBook'
import { AuthorPhotoPicker } from '../components/AuthorPhotoPicker'
import { langName } from '../lib/languages'

interface Detail { author: AuthorT; books: Book[] }
type Work = Candidate & { on_shelf?: number }

export default function Author() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data, error, reload, setData } = useAsync(() => api.get<Detail>(`${V1}/authors/${id}`), [id])
  const works = useAsync(() => api.get<{ works: Work[]; linked: boolean; unavailable?: boolean }>(`${V1}/authors/${id}/works`), [id, data?.author.ol_key])
  const [editing, setEditing] = useState(false)
  const [relinking, setRelinking] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [busy, setBusy] = useState(false)
  const [showAll, setShowAll] = useState(false)
  const [details, setDetails] = useState<Work | null>(null)
  const [photoOpen, setPhotoOpen] = useState(false)
  // Still being looked up in the background: check back shortly.
  useEffect(() => { if (data && !data.author.enriched_at) { const t = setTimeout(reload, 3000); return () => clearTimeout(t) } }, [data, reload])

  if (error) return <div className="card pad"><p>{error}</p><Link to="/authors">← Authors</Link></div>
  if (!data) return <Loading />
  const a = data.author

  const refresh = async (key?: string) => {
    setBusy(true)
    try { const r = await api.post<AuthorT>(`${V1}/authors/${a.id}/refresh`, key ? { key } : {}); setData({ ...data, author: r }); works.reload(); toast(r.ol_key ? 'Updated from Open Library' : 'Not found on Open Library') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const addWork = async (w: Work, status: Status) => {
    try {
      const full = await withDetails(w)
      const body = {
        title: full.title, subtitle: full.subtitle, authors: a.name, isbn13: full.isbn13, isbn10: full.isbn10, pages: full.pages, published: full.published,
        publisher: full.publisher, language: full.language, description: full.description, source: full.source, source_id: full.source_id, source_url: full.source_url,
        cover_url: full.cover_url, status, allow_duplicate: true,
      }
      await api.post(`${V1}/books`, body)
      toast(status === 'want' ? `“${w.title}” added to your plan` : `“${w.title}” added as read`)
      reload(); works.reload()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const del = async () => { await api.del(`${V1}/authors/${a.id}`); toast(`Removed ${a.name}`); navigate('/authors') }

  const other = (works.data?.works || []).filter(w => !w.on_shelf)
  const shownOther = showAll ? other : other.slice(0, 18)
  const span = lifespan(a.birth_date, a.death_date)

  return (
    <div className="author-page">
      <Link to="/authors" className="muted small">← Authors</Link>
      <div className="author-head">
        <button type="button" className="photo-change" onClick={() => setPhotoOpen(true)} title="Change photo">
          <AuthorPhoto author={a} size="lg" />
          <span className="photo-change-label">Change</span>
        </button>
        <div className="info">
          <h1>{a.name}</h1>
          <div className="muted">{fmtYearRange(a.birth_date, a.death_date) || span}</div>
          <div className="row wrap mt-sm">
            {a.wikipedia && <a className="btn ghost sm" href={a.wikipedia} target="_blank" rel="noreferrer">Wikipedia ↗</a>}
            {a.ol_key && <a className="btn ghost sm" href={`https://openlibrary.org/authors/${a.ol_key}`} target="_blank" rel="noreferrer">Open Library ↗</a>}
            <button className="btn sm" onClick={() => setEditing(true)}>Edit</button>
            <button className="btn ghost sm" onClick={() => refresh()} disabled={busy}>{busy ? 'Updating…' : '↻ Refresh'}</button>
            <button className="btn ghost sm" onClick={() => setRelinking(true)} title="Pick the right Open Library author">Wrong person?</button>
            {a.books === 0 && <button className="btn ghost sm danger-text" onClick={() => setDeleting(true)}>Remove</button>}
          </div>
        </div>
      </div>

      <div className="stats mt">
        <div className="stat"><div className="n">{a.read}</div><div className="l">read</div></div>
        <div className="stat"><div className="n">{a.reading}</div><div className="l">reading</div></div>
        <div className="stat"><div className="n">{a.want}</div><div className="l">planned</div></div>
        <div className="stat"><div className="n">{a.avg_rating ? a.avg_rating.toFixed(1) : '–'}</div><div className="l">your average ★</div></div>
        <div className="stat"><div className="n">{a.pages ? a.pages.toLocaleString() : '–'}</div><div className="l">pages read</div></div>
      </div>

      <Card title="About">
        {!a.enriched_at ? <p className="muted"><span className="spinner sm" /> Looking up a biography…</p>
          : a.bio ? <>
            <Biography short={a.bio} full={a.bio_full} more={a.bio_source.includes('wikipedia') ? a.bio_source : ''} />
            {a.bio_source && <p className="muted small mt-sm">Source: <a href={a.bio_source} target="_blank" rel="noreferrer">{a.bio_source.includes('wikipedia') ? 'Wikipedia' : 'Open Library'}</a>{a.bio_source.includes('wikipedia') && ' (text under CC BY-SA)'}</p>}
          </>
          : <p className="muted">No biography found. {a.ol_key ? '' : 'Use “Wrong person?” to link the right Open Library author, or write one with Edit.'}</p>}
        {a.last_read && <p className="small mt-sm"><span className="muted">Last finished:</span> {fmtDate(a.last_read)}</p>}
      </Card>

      <Card title={`On your shelf (${data.books.length})`} className="mt">
        {data.books.length ? <div className="book-list">{data.books.map(b => <BookRow key={b.id} book={b} />)}</div> : <p className="muted">None yet — pick something below.</p>}
      </Card>

      <Card title={`Other books by ${a.name}`} className="mt">
        {works.loading && !works.data ? <Loading /> : !works.data?.linked ? (
          <p className="muted">Not linked to Open Library, so the bibliography is unknown. Use “Wrong person?” to pick the right author.</p>
        ) : works.data.unavailable ? <p className="muted">Open Library did not answer. Try again in a moment.</p>
          : other.length === 0 ? <p className="muted">You have everything Open Library lists. 🎉</p> : (
            <>
              <div className="work-grid">
                {shownOther.map(w => (
                  <div key={w.source_id} className="work">
                    <button type="button" className="work-open" onClick={() => setDetails(w)} title="More about this book"><RemoteCover url={w.thumb_url} title={w.title} size="sm" /></button>
                    <div className="info">
                      <button type="button" className="nm work-title" onClick={() => setDetails(w)} title={w.title}>{w.title}</button>
                      {w.original_title && <div className="muted small orig" title={w.original_title}>{w.original_title}</div>}
                      <div className="muted small">{[w.published, w.pages ? `${w.pages} p.` : '', w.language ? langName(w.language) : ''].filter(Boolean).join(' · ')}</div>
                      <div className="row mt-sm">
                        <button className="btn sm" onClick={() => addWork(w, 'want')} title="Add to your reading plan">+ Want</button>
                        <button className="btn ghost sm" onClick={() => addWork(w, 'read')} title="You have read it">Read it</button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              {other.length > shownOther.length && <div className="mt"><button className="btn ghost" onClick={() => setShowAll(true)}>Show all {other.length}</button></div>}
              <p className="muted small mt">From Open Library. Books available in your preferred languages (Settings → Metadata) come first, shown as that edition; the original title below. Click a book for details.</p>
            </>
          )}
      </Card>

      {photoOpen && <AuthorPhotoPicker author={a} onClose={() => setPhotoOpen(false)} onChanged={reload} />}
      {details && <AddBook initial={{ ...details, authors: [a.name] }} status="want" onClose={() => setDetails(null)} onAdded={() => { reload(); works.reload() }} />}
      {editing && <EditAuthor author={a} onClose={() => setEditing(false)} onSaved={na => { setEditing(false); if (na.id !== a.id) navigate(`/authors/${na.id}`, { replace: true }); else reload() }} />}
      {relinking && <FindAuthor initial={a.name} onClose={() => setRelinking(false)} onPick={(c: AuthorCandidate) => { setRelinking(false); refresh(c.key) }} />}
      {deleting && <Confirm title={`Remove ${a.name}?`} message="They have no books on your shelf. The biography and photo are removed." onCancel={() => setDeleting(false)} onConfirm={del} />}
    </div>
  )
}

/** The short biography, and the full one (with section titles) behind “Read more”. */
function Biography({ short, full, more }: { short: string; full: string; more: string }) {
  const [open, setOpen] = useState(false)
  const long = full || (short.length > 700 ? short : '')
  if (!long) return <div className="bio">{short}</div>
  if (!open) return (
    <>
      <div className={`bio ${full ? '' : 'clamped'}`}>{short}</div>
      <button className="btn ghost sm mt-sm read-more" onClick={() => setOpen(true)}>Read more{full ? ` · ${Math.max(1, Math.round(full.split(/\s+/).length / 230))} min` : ''} ▾</button>
    </>
  )
  return (
    <>
      <div className="bio full">{long.split('\n').map((l, i) =>
        l.startsWith('### ') ? <h4 key={i}>{l.slice(4)}</h4>
          : l.startsWith('## ') ? <h3 key={i}>{l.slice(3)}</h3>
            : l.trim() ? <p key={i}>{l}</p> : null)}</div>
      <div className="row wrap">
        <button className="btn ghost sm read-more" onClick={() => setOpen(false)}>Show less ▴</button>
        {full && more && <a className="btn ghost sm" href={more} target="_blank" rel="noreferrer">Continue on Wikipedia ↗</a>}
      </div>
    </>
  )
}

/** "21 October 1929" … "22 January 2018" when both are full dates. */
function fmtYearRange(b: string, d: string): string { return /\d{1,2} \w+ \d{4}/.test(b) && /\d{1,2} \w+ \d{4}/.test(d) ? `${b} – ${d}` : '' }

function EditAuthor({ author, onClose, onSaved }: { author: AuthorT; onClose: () => void; onSaved: (a: AuthorT) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: author.name, birth_date: author.birth_date, death_date: author.death_date, bio: author.bio })
  const [photoUrl, setPhotoUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      let a = await api.put<AuthorT>(`${V1}/authors/${author.id}`, f)
      if (photoUrl) a = await api.post<AuthorT>(`${V1}/authors/${a.id}/photo`, { url: photoUrl })
      toast(f.name !== author.name ? `Renamed — ${a.books} ${a.books === 1 ? 'book' : 'books'} updated` : 'Saved'); onSaved(a)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const uploadPhoto = async (file: File) => {
    try { await upload(`${V1}/authors/${author.id}/photo`, 'photo', file); toast('Photo updated'); onSaved(author) } catch (e) { toast((e as Error).message, 'err') }
  }
  const removePhoto = async () => { await api.del(`${V1}/authors/${author.id}/photo`); toast('Photo removed'); onSaved(author) }
  return (
    <Modal title="Edit author" onClose={onClose} size="lg">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <div className="cover-edit">
          <AuthorPhoto author={author} size="lg" />
          <div className="grid" style={{ gap: 8, flex: 1 }}>
            <div className="row wrap">
              <label className="btn">Upload photo…<input type="file" accept="image/*" hidden onChange={e => e.target.files?.[0] && uploadPhoto(e.target.files[0])} /></label>
              {author.photo && <button type="button" className="btn ghost danger-text" onClick={removePhoto}>Remove photo</button>}
            </div>
            <Field label="Photo from a web address"><input value={photoUrl} onChange={e => setPhotoUrl(e.target.value.trim())} placeholder="https://…" /></Field>
          </div>
        </div>
        <Field label="Name" help={author.books > 0 ? `Also changes the name on ${author.books} ${author.books === 1 ? 'book' : 'books'}. Renaming to an existing author merges the two.` : undefined}><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required /></Field>
        <div className="two-col tight">
          <Field label="Born"><input value={f.birth_date} onChange={e => setF({ ...f, birth_date: e.target.value })} placeholder="21 October 1929" /></Field>
          <Field label="Died"><input value={f.death_date} onChange={e => setF({ ...f, death_date: e.target.value })} /></Field>
        </div>
        <Field label="Biography" help="Edited text is kept when you refresh from Open Library."><textarea rows={8} value={f.bio} onChange={e => setF({ ...f, bio: e.target.value })} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Modal>
  )
}
