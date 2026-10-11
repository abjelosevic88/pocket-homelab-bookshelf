import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Book, TagCount } from '../lib/types'
import { Empty, Loading, PageHeader, Segmented, useAsync, useDebounce } from '../components/ui'
import { BookCard, BookRow } from '../components/BookCard'
import { AddBook } from '../components/AddBook'

const STATUSES = [
  { value: '', label: 'All' },
  { value: 'reading', label: 'Reading' },
  { value: 'want', label: 'Want' },
  { value: 'read', label: 'Read' },
  { value: 'abandoned', label: 'Abandoned' },
]
const SORTS = [
  { value: 'recent', label: 'Recently updated' },
  { value: 'added', label: 'Recently added' },
  { value: 'finished', label: 'Date finished' },
  { value: 'title', label: 'Title' },
  { value: 'author', label: 'Author' },
  { value: 'rating', label: 'Rating' },
]

export default function Library() {
  const [params, setParams] = useSearchParams()
  const status = params.get('status') || ''
  const sort = params.get('sort') || (status === 'read' ? 'finished' : 'recent')
  const tag = params.get('tag') || ''
  const year = params.get('year') || ''
  const author = params.get('author') || ''
  const fav = params.get('favorite') === '1'
  const [q, setQ] = useState(params.get('q') || '')
  const dq = useDebounce(q, 250)
  const [view, setView] = useState<'grid' | 'list'>(() => (localStorage.getItem('pb-view') as 'grid' | 'list') || 'grid')
  const [adding, setAdding] = useState(false)

  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const qs = new URLSearchParams({ sort, ...(status && { status }), ...(tag && { tag }), ...(year && { year }), ...(author && { author }), ...(fav && { favorite: '1' }), ...(dq && { q: dq }) })
  const { data, loading, reload } = useAsync(() => api.get<Book[]>(`${V1}/books?${qs}`), [qs.toString()])
  const tags = useAsync(() => api.get<TagCount[]>(`${V1}/tags`), [])
  const changeView = (v: 'grid' | 'list') => { setView(v); localStorage.setItem('pb-view', v) }

  return (
    <>
      <PageHeader title="Library" sub={data && `${data.length} ${data.length === 1 ? 'book' : 'books'}`} actions={<button className="btn primary" onClick={() => setAdding(true)}>+ Add book</button>} />
      <div className="toolbar">
        <div className="search"><span className="ico">⌕</span><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search title, author, tag, review…" /></div>
        <select value={sort} onChange={e => set('sort', e.target.value)} aria-label="Sort">{SORTS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}</select>
        {tags.data && tags.data.length > 0 && (
          <select value={tag} onChange={e => set('tag', e.target.value)} aria-label="Tag"><option value="">All tags</option>{tags.data.map(t => <option key={t.name} value={t.name}>{t.name} ({t.count})</option>)}</select>
        )}
        <Segmented size="sm" value={view} onChange={changeView} options={[{ value: 'grid', label: '▦', title: 'Covers' }, { value: 'list', label: '☰', title: 'List' }]} />
      </div>
      <div className="filters">
        {STATUSES.map(s => <button key={s.value} className={`chip ${status === s.value ? 'active' : ''}`} onClick={() => set('status', s.value)}>{s.label}</button>)}
        <button className={`chip ${fav ? 'active' : ''}`} onClick={() => set('favorite', fav ? '' : '1')}>♥ Favourites</button>
        {year && <button className="chip active" onClick={() => set('year', '')}>Read in {year} ✕</button>}
        {author && <button className="chip active" onClick={() => set('author', '')}>{author} ✕</button>}
      </div>
      {loading && !data ? <Loading /> : data && data.length === 0 ? (
        <div className="card"><Empty title={q || status || tag || year || author || fav ? 'No books match' : 'Your shelf is empty'}>{q || status || tag ? 'Try another filter.' : 'Add the first book: search by title or ISBN and the details fill themselves in.'}</Empty></div>
      ) : view === 'grid' ? (
        <div className="book-grid">{data?.map(b => <BookCard key={b.id} book={b} />)}</div>
      ) : (
        <div className="book-list">{data?.map(b => <BookRow key={b.id} book={b} />)}</div>
      )}
      {adding && <AddBook status={status === 'read' || status === 'reading' ? status : 'want'} onClose={() => setAdding(false)} onAdded={() => { reload(); tags.reload() }} />}
    </>
  )
}
