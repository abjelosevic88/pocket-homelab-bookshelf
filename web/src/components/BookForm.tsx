import { useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import type { Book, Candidate } from '../lib/types'
import { Field, Modal, Segmented, useToast } from './ui'
import { Icon } from './icons'
import { BookSearch, fromCandidate, withDetails } from './BookSearch'
import { Cover, RemoteCover } from './Cover'
import { Select } from './Select'
import { useAsync } from './ui'
import type { SeriesSummary } from '../lib/types'

export type BookDraft = Pick<Book, 'title' | 'subtitle' | 'authors' | 'isbn13' | 'isbn10' | 'pages' | 'published' | 'publisher' | 'language' | 'description' | 'source' | 'source_id' | 'source_url' | 'tags' | 'format'> & { series?: string; series_no?: string }

export function emptyBook(): BookDraft {
  return { title: '', subtitle: '', authors: '', isbn13: '', isbn10: '', pages: 0, published: '', publisher: '', language: '', description: '', source: '', source_id: '', source_url: '', tags: '', format: '' }
}

/** The bibliographic fields of a book. */
export function BookFields({ draft, onChange, full = false }: { draft: BookDraft; onChange: (d: BookDraft) => void; full?: boolean }) {
  const set = <K extends keyof BookDraft>(k: K) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    onChange({ ...draft, [k]: k === 'pages' ? Math.max(0, Number(e.target.value) || 0) : e.target.value })
  return (
    <div className="grid" style={{ gap: 12 }}>
      <Field label="Title"><input value={draft.title} onChange={set('title')} required autoFocus={!draft.title} /></Field>
      <Field label="Subtitle"><input value={draft.subtitle} onChange={set('subtitle')} /></Field>
      <Field label="Authors" help="Separate several with commas."><input value={draft.authors} onChange={set('authors')} /></Field>
      <div className="three-col tight">
        <Field label="Pages"><input type="number" min={0} value={draft.pages || ''} onChange={set('pages')} /></Field>
        <Field label="Published"><input value={draft.published} onChange={set('published')} placeholder="1969" /></Field>
        <Field label="Language"><input value={draft.language} onChange={set('language')} placeholder="en" /></Field>
      </div>
      <div className="three-col tight">
        <Field label="ISBN-13"><input value={draft.isbn13} onChange={set('isbn13')} inputMode="numeric" /></Field>
        <Field label="ISBN-10"><input value={draft.isbn10} onChange={set('isbn10')} /></Field>
        <Field label="Publisher"><input value={draft.publisher} onChange={set('publisher')} /></Field>
      </div>
      {full && <>
        <div className="two-col tight">
          <Field group label="Format"><Segmented size="sm" value={draft.format} onChange={v => onChange({ ...draft, format: v })} options={[{ value: '', label: '–' }, { value: 'paper', label: 'Paper' }, { value: 'ebook', label: 'E-book' }, { value: 'audio', label: 'Audio' }]} /></Field>
          <Field label="Tags" help="Comma separated."><input value={draft.tags} onChange={set('tags')} /></Field>
        </div>
        <div className="two-col tight">
          <Field group label="Series" help="Pick one of yours or type a new name."><SeriesPicker value={draft.series || ''} onChange={v => onChange({ ...draft, series: v })} /></Field>
          <Field label="Number in series" help="1, 2 … — 0 for a prequel, 2.5 for an in-between."><input value={draft.series_no || ''} onChange={e => onChange({ ...draft, series_no: e.target.value.trim() })} placeholder="e.g. 4" style={{ maxWidth: 120 }} /></Field>
        </div>
        <Field label="Description"><textarea rows={5} value={draft.description} onChange={set('description')} /></Field>
      </>}
    </div>
  )
}

/** Edit a book's details and cover; can re-fetch both from the catalogues. */
export function EditBook({ book, onClose, onSaved }: { book: Book; onClose: () => void; onSaved: (b: Book) => void }) {
  const { toast } = useToast()
  const [draft, setDraft] = useState<BookDraft>({ ...book })
  const [coverUrl, setCoverUrl] = useState('')
  const [finding, setFinding] = useState(false)
  const [busy, setBusy] = useState(false)

  const apply = async (c: Candidate) => {
    setFinding(false)
    const full = await withDetails(c)
    setDraft(d => ({ ...d, ...Object.fromEntries(Object.entries(fromCandidate(full)).filter(([, v]) => v)) }))
    if (full.cover_url) setCoverUrl(full.cover_url)
    toast('Details filled in — check them and save')
  }

  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const r = await api.put<{ book: Book; warning?: string }>(`${V1}/books/${book.id}`, { ...draft, cover_url: coverUrl || undefined })
      if (r.warning) toast(r.warning, 'err'); else toast('Saved')
      onSaved(r.book)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }

  const uploadCover = async (f: File) => {
    try { const b = await upload<Book>(`${V1}/books/${book.id}/cover`, 'cover', f); toast('Cover updated'); onSaved(b) } catch (e) { toast((e as Error).message, 'err') }
  }
  const removeCover = async () => { await api.del(`${V1}/books/${book.id}/cover`); onSaved({ ...book, cover: '' }); toast('Cover removed') }

  if (finding) return (
    <Modal title="Find details & cover" onClose={() => setFinding(false)} size="lg">
      <BookSearch initial={[book.title, book.authors.split(',')[0]].filter(Boolean).join(' ')} onPick={apply} />
      <div className="form-actions"><button type="button" className="btn" onClick={() => setFinding(false)}>Cancel</button></div>
    </Modal>
  )

  return (
    <Modal title="Edit details" onClose={onClose} size="lg">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <div className="cover-edit">
          {coverUrl ? <RemoteCover url={coverUrl} title={draft.title} size="sm" /> : <Cover book={book} size="sm" />}
          <div className="grid" style={{ gap: 8 }}>
            <div className="row wrap">
              <button type="button" className="btn" onClick={() => setFinding(true)}><Icon name="search" size={15} />Find details & cover</button>
              <label className="btn ghost">Upload cover…<input type="file" accept="image/*" hidden onChange={e => e.target.files?.[0] && uploadCover(e.target.files[0])} /></label>
              {book.cover && !coverUrl && <button type="button" className="btn ghost danger-text" onClick={removeCover}>Remove cover</button>}
            </div>
            <Field label="Cover from a web address" help={coverUrl ? 'Downloaded when you save.' : 'Paste an image URL to use it as the cover.'}><input value={coverUrl} onChange={e => setCoverUrl(e.target.value.trim())} placeholder="https://…" /></Field>
          </div>
        </div>
        <BookFields draft={draft} onChange={setDraft} full />
        {draft.source_url && <p className="muted small">Source: <a href={draft.source_url} target="_blank" rel="noreferrer">{draft.source === 'google' ? 'Google Books' : draft.source === 'openlibrary' ? 'Open Library' : draft.source_url}</a></p>}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary" disabled={busy || !draft.title.trim()}>Save</button></div>
      </form>
    </Modal>
  )
}

function SeriesPicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { data } = useAsync(() => api.get<SeriesSummary[]>(`${V1}/series`), [])
  const names = Array.from(new Set([...(data || []).map(s => s.name), ...(value ? [value] : [])])).sort()
  return <Select value={value} onChange={onChange} placeholder="No series" clearLabel="No series" ariaLabel="Series"
    options={names.map(n => ({ value: n, label: n }))} onCreate={name => name} createLabel="New series" />
}
