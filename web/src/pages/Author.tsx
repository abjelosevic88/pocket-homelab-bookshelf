import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, upload, V1 } from '../lib/api'
import type { Author as AuthorT, AuthorCandidate, Book, Candidate, Status } from '../lib/types'
import { fmtDate, lifespan } from '../lib/format'
import { Card, Confirm, Field, Loading, Modal, useAsync, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { useCrumb } from '../components/Layout'
import { AuthorPhoto } from '../components/AuthorBits'
import { BookRow } from '../components/BookCard'
import { RemoteCover } from '../components/Cover'
import { withDetails } from '../components/BookSearch'
import { FindAuthor } from './Authors'
import { AddBook } from '../components/AddBook'
import { AuthorPhotoPicker } from '../components/AuthorPhotoPicker'
import { langName } from '../lib/languages'
import { GetMenu, KIND_ICON, liveAsk, openAction, postGet, stateText, wantFollowUp, type Ask, type WantGetIt } from '../components/GetIt'

interface Detail { author: AuthorT; books: Book[] }
// The author's books in Pocket Requests: here (as an ebook / audiobook) or asked for.
interface RqBook { book_key: string; title: string; year: number; poster: string; ebook: { url: string } | null; audiobook: { url: string } | null; asked: Ask[] }
interface RqAuthor { available: boolean; reason?: string; items: RqBook[]; languages: string[]; app_url?: string }

/** fold makes titles comparable: lowercase, no marks or punctuation. */
const fold = (s: string) => s.normalize('NFD').replace(/[\u0300-\u036f]/g, '').replace(/đ/gi, 'dj').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim()
const sameTitle = (a: string, b: string) => { const x = fold(a), y = fold(b); return !!x && !!y && (x === y || x.startsWith(y + ' ') || y.startsWith(x + ' ')) }
type Work = Candidate & { on_shelf?: number }

export default function Author() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data, error, reload, setData } = useAsync(() => api.get<Detail>(`${V1}/authors/${id}`), [id])
  const works = useAsync(() => api.get<{ works: Work[]; linked: boolean; unavailable?: boolean }>(`${V1}/authors/${id}/works`), [id, data?.author.ol_key])
  const rq = useAsync(() => api.get<RqAuthor>(`${V1}/authors/${id}/requests`), [id])
  const [editing, setEditing] = useState(false)
  const [relinking, setRelinking] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [busy, setBusy] = useState(false)
  const [showAll, setShowAll] = useState(false)
  const [details, setDetails] = useState<Work | null>(null)
  const [photoOpen, setPhotoOpen] = useState(false)
  // Still being looked up in the background: check back shortly.
  useEffect(() => { if (data && !data.author.enriched_at) { const t = setTimeout(reload, 3000); return () => clearTimeout(t) } }, [data, reload])

  useCrumb(data?.author.name)

  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error} · <Link to="/authors">All authors</Link></span></div>
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
      const r = await api.post<{ book: Book; get_it?: WantGetIt }>(`${V1}/books`, body)
      toast(status === 'want' ? `“${w.title}” added to your plan` : `“${w.title}” added as read`)
      wantFollowUp(toast, r.book, r.get_it)
      reload(); works.reload()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const del = async () => { await api.del(`${V1}/authors/${a.id}`); toast(`Removed ${a.name}`); navigate('/authors') }

  const other = (works.data?.works || []).filter(w => !w.on_shelf)
  const rqItems = rq.data?.items || []
  const rqFor = (title: string) => rqItems.find(i => sameTitle(i.title, title))
  const rqOk = !!rq.data?.available
  const langs = rq.data?.languages || []
  // Pocket Requests: ask for one of the author's books (on the shelf: through the book; else by its key)
  const getBook = async (kind: 'ebook' | 'audiobook', language: string, target: { bookId?: number; key?: string; title: string }) => {
    const r = target.bookId ? await postGet(`${V1}/books/${target.bookId}/requests`, { kind, language })
      : await postGet(`${V1}/authors/${a.id}/requests`, { kind, language, book_key: target.key, title: target.title })
    toast(r.msg, r.ok ? 'ok' : 'err', openAction(r.url))
    rq.reload()
  }
  const workKey = (w: Work) => rqFor(w.title)?.book_key || (w.source === 'openlibrary' && /W$/.test(w.source_id) ? 'ol:' + w.source_id.replace(/^\/works\//, '') : '')
  // what Requests has that is neither on the shelf nor in the list below
  const extra = rqItems.filter(i => !data.books.some(b => sameTitle(b.title, i.title)) && !other.some(w => sameTitle(w.title, i.title)) && !(works.data?.works || []).some(w => sameTitle(w.title, i.title)))
  const shownOther = showAll ? other : other.slice(0, 18)
  const span = lifespan(a.birth_date, a.death_date)

  return (
    <div className="author-page">
      <div className="author-head">
        <button type="button" className="photo-change" onClick={() => setPhotoOpen(true)} title="Change photo">
          <AuthorPhoto author={a} size="lg" />
          <span className="photo-change-label">Change</span>
        </button>
        <div className="info">
          <h1>{a.name}</h1>
          <div className="muted">{fmtYearRange(a.birth_date, a.death_date) || span}</div>
          <div className="row wrap mt-sm">
            {a.wikipedia && <a className="btn ghost sm" href={a.wikipedia} target="_blank" rel="noreferrer">Wikipedia<Icon name="external" size={13} /></a>}
            {a.ol_key && <a className="btn ghost sm" href={`https://openlibrary.org/authors/${a.ol_key}`} target="_blank" rel="noreferrer">Open Library<Icon name="external" size={13} /></a>}
            <button type="button" className="btn sm" onClick={() => setEditing(true)}><Icon name="edit" size={14} />Edit</button>
            <button type="button" className="btn ghost sm" onClick={() => refresh()} disabled={busy}><Icon name="refresh" size={14} />{busy ? 'Updating…' : 'Refresh'}</button>
            <button type="button" className="btn ghost sm" onClick={() => setRelinking(true)} title="Pick the right Open Library author">Wrong person?</button>
            {a.books === 0 && <button type="button" className="btn ghost sm danger-text" onClick={() => setDeleting(true)}>Remove</button>}
          </div>
        </div>
      </div>

      <div className="stats mt">
        <div className="stat"><div className="n">{a.read}</div><div className="l">read</div></div>
        <div className="stat"><div className="n">{a.reading}</div><div className="l">reading</div></div>
        <div className="stat"><div className="n">{a.want}</div><div className="l">planned</div></div>
        <div className="stat"><div className="n">{a.avg_rating ? a.avg_rating.toFixed(1) : '–'}</div><div className="l">your average rating</div></div>
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
        {data.books.length ? <div className="book-list">{data.books.map(b => (
          <div key={b.id} className="author-shelf-line">
            <BookRow book={b} />
            <span className="author-get"><RqBadges item={rqFor(b.title)} />{rqOk && <GetMenu languages={langs} onGet={(k, l) => getBook(k, l, { bookId: b.id, title: b.title })} />}</span>
          </div>
        ))}</div> : <p className="muted">None yet — pick something below.</p>}
        {rq.data && !rq.data.available && rq.data.reason && <p className="core-hint mt-sm">Pocket Requests: {rq.data.reason}</p>}
      </Card>

      <Card title={`Other books by ${a.name}`} className="mt">
        {works.loading && !works.data ? <Loading /> : !works.data?.linked ? (
          <p className="muted">Not linked to Open Library, so the bibliography is unknown. Use “Wrong person?” to pick the right author.</p>
        ) : works.data.unavailable ? <p className="muted">Open Library did not answer. Try again in a moment.</p>
          : other.length === 0 ? <p className="muted">You have everything Open Library lists.</p> : (
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
                        <button type="button" className="btn sm" onClick={() => addWork(w, 'want')} title="Add to your reading plan"><Icon name="plus" size={14} />Want</button>
                        <button type="button" className="btn ghost sm" onClick={() => addWork(w, 'read')} title="You have read it">Read it</button>
                        {rqOk && <GetMenu languages={langs} onGet={(k, l) => getBook(k, l, { key: workKey(w), title: w.title })} />}
                      </div>
                      <RqBadges item={rqFor(w.title)} />
                    </div>
                  </div>
                ))}
              </div>
              {other.length > shownOther.length && <div className="mt"><button type="button" className="btn ghost" onClick={() => setShowAll(true)}>Show all {other.length}</button></div>}
              <p className="muted small mt">From Open Library. Books available in your preferred languages (Settings → Book search) come first, shown as that edition; the original title below. Click a book for details.</p>
            </>
          )}
      </Card>

      {rqOk && extra.length > 0 && (
        <Card title={`More by ${a.name} in Pocket Requests`} className="mt">
          <div className="work-grid">
            {extra.map(i => (
              <div key={i.book_key} className="work">
                <RemoteCover url={i.poster} title={i.title} size="sm" />
                <div className="info">
                  <div className="nm work-title" title={i.title}>{i.title}</div>
                  <div className="muted small">{i.year || ''}</div>
                  <div className="row mt-sm"><GetMenu languages={langs} className="btn sm" onGet={(k, l) => getBook(k, l, { key: i.book_key, title: i.title })}><Icon name="download" size={14} />Get it</GetMenu></div>
                  <RqBadges item={i} />
                </div>
              </div>
            ))}
          </div>
          <p className="muted small mt">Books Pocket Requests knows by {a.name} that aren’t on your shelves or in the list above.</p>
        </Card>
      )}

      {photoOpen && <AuthorPhotoPicker author={a} onClose={() => setPhotoOpen(false)} onChanged={reload} />}
      {details && <AddBook initial={{ ...details, authors: [a.name] }} status="want" onClose={() => setDetails(null)} onAdded={() => { reload(); works.reload() }} />}
      {editing && <EditAuthor author={a} onClose={() => setEditing(false)} onSaved={na => { setEditing(false); if (na.id !== a.id) navigate(`/authors/${na.id}`, { replace: true }); else reload() }} />}
      {relinking && <FindAuthor initial={a.name} onClose={() => setRelinking(false)} onPick={(c: AuthorCandidate) => { setRelinking(false); refresh(c.key) }} />}
      {deleting && <Confirm title={`Remove ${a.name}?`} message="They have no books on your shelf. The biography and photo are removed." onCancel={() => setDeleting(false)} onConfirm={del} />}
    </div>
  )
}

/** RqBadges: here as an ebook / audiobook (opens it), or asked for (how far along). */
function RqBadges({ item }: { item?: RqBook }) {
  if (!item) return null
  const asks = item.asked.filter(liveAsk)
  if (!item.ebook && !item.audiobook && !asks.length) return null
  return (
    <span className="rq-badges">
      {item.ebook && <a className="rq-badge here" href={item.ebook.url} target="_blank" rel="noreferrer" title="Open the ebook"><Icon name={KIND_ICON.ebook} size={11} />Ebook here</a>}
      {item.audiobook && <a className="rq-badge here" href={item.audiobook.url} target="_blank" rel="noreferrer" title="Open the audiobook"><Icon name={KIND_ICON.audiobook} size={11} />Audiobook here</a>}
      {asks.map(x => (
        <a key={x.id} className="rq-badge" href={x.url} target="_blank" rel="noreferrer" title="The request in Pocket Requests">
          <Icon name={KIND_ICON[x.kind]} size={11} />{x.language ? langName(x.language) + ' · ' : ''}{stateText(x)}
        </a>
      ))}
    </span>
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
      <button type="button" className="btn ghost sm mt-sm read-more" onClick={() => setOpen(true)}>Read more{full ? ` · ${Math.max(1, Math.round(full.split(/\s+/).length / 230))} min` : ''}<Icon name="chevron-down" size={14} /></button>
    </>
  )
  return (
    <>
      <div className="bio full">{long.split('\n').map((l, i) =>
        l.startsWith('### ') ? <h4 key={i}>{l.slice(4)}</h4>
          : l.startsWith('## ') ? <h3 key={i}>{l.slice(3)}</h3>
            : l.trim() ? <p key={i}>{l}</p> : null)}</div>
      <div className="row wrap">
        <button type="button" className="btn ghost sm read-more" onClick={() => setOpen(false)}>Show less<Icon name="chevron-up" size={14} /></button>
        {full && more && <a className="btn ghost sm" href={more} target="_blank" rel="noreferrer">Continue on Wikipedia<Icon name="external" size={13} /></a>}
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
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Modal>
  )
}
