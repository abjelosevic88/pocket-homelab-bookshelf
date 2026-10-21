import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Book, TagCount } from '../lib/types'
import { Empty, Loading, PageHeader, Segmented, useAsync, useDebounce } from '../components/ui'
import { BookCard, BookRow } from '../components/BookCard'
import { AddBook } from '../components/AddBook'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { useOnChange } from '../components/Layout'

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
  useOnChange(() => { reload(); tags.reload() })
  const changeView = (v: 'grid' | 'list') => { setView(v); localStorage.setItem('pb-view', v) }

  return (
    <>
      <PageHeader title="Library" sub={data && `${data.length} ${data.length === 1 ? 'book' : 'books'}`} actions={<button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add book</button>} />
      <div className="toolbar">
        <div className="search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search title, author, tag, review…" /></div>
        <Select value={sort} onChange={v => set('sort', v)} ariaLabel="Sort" options={SORTS.map(s => ({ value: s.value, label: `Sort: ${s.label.toLowerCase()}` }))} />
        {tags.data && tags.data.length > 0 && (
          <Select value={tag} onChange={v => set('tag', v)} ariaLabel="Tag" placeholder="All tags" clearLabel="All tags" options={tags.data.map(t => ({ value: t.name, label: t.name, hint: String(t.count) }))} />
        )}
        <Segmented size="sm" value={view} onChange={changeView} label="View" options={[{ value: 'grid', label: <Icon name="grid" size={14} />, title: 'Covers' }, { value: 'list', label: <Icon name="list" size={14} />, title: 'List' }]} />
      </div>
      <div className="filters">
        {STATUSES.map(s => <button type="button" key={s.value} className={`chip ${status === s.value ? 'active' : ''}`} onClick={() => set('status', s.value)}>{s.label}</button>)}
        <button type="button" className={`chip ${fav ? 'active' : ''}`} onClick={() => set('favorite', fav ? '' : '1')}><Icon name="heart" size={13} fill={fav} />Favourites</button>
        {year && <button type="button" className="chip active" onClick={() => set('year', '')}>Read in {year}<Icon name="x" size={13} /></button>}
        {author && <button type="button" className="chip active" onClick={() => set('author', '')}>{author}<Icon name="x" size={13} /></button>}
      </div>
      {loading && !data ? <Loading /> : data && data.length === 0 ? (
        <div className="card"><Empty icon="library" title={q || status || tag || year || author || fav ? 'No books match' : 'Your shelf is empty'}>{q || status || tag ? 'Try another filter.' : 'Add the first book: search by title or ISBN and the details fill themselves in.'}</Empty></div>
      ) : view === 'grid' ? (
        <div className="book-grid">{data?.map(b => <BookCard key={b.id} book={b} />)}</div>
      ) : (
        <div className="book-list">{data?.map(b => <BookRow key={b.id} book={b} />)}</div>
      )}
      {adding && <AddBook status={status === 'read' || status === 'reading' ? status : 'want'} onClose={() => setAdding(false)} onAdded={() => { reload(); tags.reload() }} />}
    </>
  )
}
