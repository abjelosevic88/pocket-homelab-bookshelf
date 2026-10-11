import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Candidate, SeriesEntry } from '../lib/types'
import { STATUS_SHORT } from '../lib/format'
import { Card, Loading, Modal, useAsync, useToast } from '../components/ui'
import { Cover } from '../components/Cover'
import { Stars } from '../components/Stars'
import { AddBook } from '../components/AddBook'

interface Detail { name: string; qid: string; total: number; read: number; books: number; entries: SeriesEntry[] }

export default function SeriesDetail() {
  const { name = '' } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data, loading, reload } = useAsync(() => api.get<Detail>(`${V1}/series/detail?name=${encodeURIComponent(name)}`), [name])
  const [adding, setAdding] = useState<SeriesEntry | null>(null)
  const [renaming, setRenaming] = useState(false)
  const [newName, setNewName] = useState(name)
  if (loading && !data) return <Loading />
  if (!data) return null
  const author = data.entries.find(e => e.book)?.book?.authors.split(',')[0] || ''
  const total = Math.max(data.total, data.books)
  const rename = async () => {
    try { const r = await api.put<{ books: number }>(`${V1}/series/rename`, { from: name, to: newName }); toast(`Renamed on ${r.books} books`); setRenaming(false); navigate(`/series/${encodeURIComponent(newName)}`, { replace: true }) } catch (e) { toast((e as Error).message, 'err') }
  }
  const candidate = (e: SeriesEntry): Candidate | undefined => e.ol_work ? { source: 'openlibrary', source_id: e.ol_work, source_url: 'https://openlibrary.org' + e.ol_work, title: e.title, subtitle: '', authors: author ? [author] : [], published: e.year || '', pages: 0, isbn13: '', isbn10: '', publisher: '', language: '', description: '', subjects: [], cover_url: '', thumb_url: '' } : undefined
  return (
    <div className="series-page">
      <Link to="/series" className="muted small">← Series</Link>
      <div className="row wrap mt-sm" style={{ alignItems: 'baseline' }}>
        <h1 className="grow">{data.name}</h1>
        <button className="btn ghost sm" onClick={() => { setNewName(name); setRenaming(true) }}>Rename</button>
        {data.qid && <a className="btn ghost sm" href={`https://www.wikidata.org/wiki/${data.qid}`} target="_blank" rel="noreferrer">Wikidata ↗</a>}
      </div>
      <div className="muted">{author}</div>
      <div className="series-progress mt">
        <b>{data.read}</b> of {total || '?'} read
        {total > 0 && <div className="bar lg mt-sm"><span className="fill" style={{ width: `${Math.round(data.read / total * 100)}%` }} /></div>}
      </div>
      <Card className="mt">
        <ol className="series-list">
          {data.entries.map((e, i) => (
            <li key={i} className={e.book ? 'have' : 'missing'}>
              <span className="no">{e.no !== '' ? `#${e.no}` : '·'}</span>
              {e.book ? (
                <Link to={`/books/${e.book.id}`} className="row grow">
                  <Cover book={e.book} size="xs" />
                  <span className="grow"><b>{e.book.title}</b>{e.title && e.title !== e.book.title && <span className="muted small block">{e.title}</span>}</span>
                  {e.book.rating > 0 ? <Stars value={e.book.rating} size="sm" /> : <span className={`status-pill inline ${e.book.status}`}>{STATUS_SHORT[e.book.status]}</span>}
                </Link>
              ) : (
                <>
                  <span className="grow"><b>{e.title || 'Untitled'}</b>{e.year && <span className="muted small"> · {e.year}</span>}<span className="muted small block">Not on your shelf</span></span>
                  <button className="btn sm" onClick={() => setAdding(e)}>+ Want</button>
                </>
              )}
            </li>
          ))}
        </ol>
        {!data.qid && <p className="muted small mt">This series is not linked to Wikidata, so only your own books are listed.</p>}
      </Card>
      {adding && <AddBook status="want" initial={candidate(adding)} query={adding.ol_work ? undefined : `${adding.title} ${author}`}
        extra={{ series: data.name, series_no: adding.no, series_qid: data.qid }} onClose={() => setAdding(null)} onAdded={reload} />}
      {renaming && (
        <Modal title="Rename series" onClose={() => setRenaming(false)}>
          <form onSubmit={e => { e.preventDefault(); rename() }} className="grid" style={{ gap: 12 }}>
            <input value={newName} onChange={e => setNewName(e.target.value)} autoFocus />
            <p className="muted small">Renames it on all {data.books} books. Renaming to the name of another series merges them.</p>
            <div className="form-actions"><button type="button" className="btn" onClick={() => setRenaming(false)}>Cancel</button><button className="btn primary" disabled={!newName.trim()}>Rename</button></div>
          </form>
        </Modal>
      )}
    </div>
  )
}
