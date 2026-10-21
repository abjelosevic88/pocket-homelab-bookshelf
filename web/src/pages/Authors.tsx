import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Author, AuthorCandidate } from '../lib/types'
import { lifespan } from '../lib/format'
import { Empty, Loading, Modal, PageHeader, useAsync, useDebounce, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { AuthorPhoto } from '../components/AuthorBits'
import { Stars } from '../components/Stars'
import { Select } from '../components/Select'

const SORTS = [{ value: 'read', label: 'Most read' }, { value: 'name', label: 'Name' }, { value: 'rating', label: 'Rating' }, { value: 'recent', label: 'Read recently' }, { value: 'added', label: 'Recently added' }]

export default function Authors() {
  const [q, setQ] = useState('')
  const [sort, setSort] = useState(() => localStorage.getItem('pb-author-sort') || 'read')
  const [finding, setFinding] = useState(false)
  const dq = useDebounce(q, 200)
  const { data, loading, reload } = useAsync(() => api.get<Author[]>(`${V1}/authors?sort=${sort}${dq ? `&q=${encodeURIComponent(dq)}` : ''}`), [sort, dq])
  // Freshly linked authors get their bio and photo in the background: refresh while that runs.
  const pending = data?.some(a => !a.enriched_at)
  useEffect(() => { if (!pending) return; const t = setTimeout(reload, 4000); return () => clearTimeout(t) }, [pending, data, reload])
  const changeSort = (v: string) => { setSort(v); localStorage.setItem('pb-author-sort', v) }
  return (
    <>
      <PageHeader title="Authors" sub={data && <>{data.length} authors{pending && <span className="muted"> · looking up bios and photos…</span>}</>} actions={<button type="button" className="btn primary" onClick={() => setFinding(true)}><Icon name="search" size={15} />Find author</button>} />
      <div className="toolbar">
        <div className="search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Filter authors…" /></div>
        <Select value={sort} onChange={changeSort} ariaLabel="Sort" options={SORTS.map(s => ({ value: s.value, label: `Sort: ${s.label.toLowerCase()}` }))} />
      </div>
      {loading && !data ? <Loading /> : data && data.length === 0 ? (
        <div className="card"><Empty icon="feather" title={dq ? 'No authors match' : 'No authors yet'}>Authors appear here as you add books, or find one with “Find author”.</Empty></div>
      ) : (
        <div className="author-grid">
          {data?.map(a => (
            <Link key={a.id} to={`/authors/${a.id}`} className="author-card card">
              <AuthorPhoto author={a} />
              <div className="info">
                <div className="nm">{a.name}{a.followed && a.books === 0 && <span className="badge cat">following</span>}</div>
                <div className="muted small">{lifespan(a.birth_date, a.death_date)}</div>
                <div className="small counts">
                  {a.read > 0 ? <b>{a.read} read</b> : <span className="muted">none read yet</span>}
                  {a.reading > 0 && <span> · {a.reading} reading</span>}
                  {a.want > 0 && <span className="muted"> · {a.want} planned</span>}
                </div>
                {a.avg_rating > 0 && <Stars value={Math.round(a.avg_rating * 2) / 2} size="sm" />}
              </div>
            </Link>
          ))}
        </div>
      )}
      {finding && <FindAuthor onClose={() => setFinding(false)} />}
    </>
  )
}

/** Search Open Library for an author and add them. */
export function FindAuthor({ onClose, initial = '', onPick }: { onClose: () => void; initial?: string; onPick?: (c: AuthorCandidate) => void }) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const [q, setQ] = useState(initial)
  const dq = useDebounce(q.trim(), 450)
  const [results, setResults] = useState<AuthorCandidate[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [adding, setAdding] = useState('')
  useEffect(() => {
    if (dq.length < 3) { setResults(null); return }
    let live = true
    setBusy(true)
    api.get<{ results: AuthorCandidate[] }>(`${V1}/authors/search?q=${encodeURIComponent(dq)}`)
      .then(r => live && setResults(r.results)).catch(() => live && setResults([])).finally(() => live && setBusy(false))
    return () => { live = false }
  }, [dq])
  const add = async (c: AuthorCandidate) => {
    if (onPick) { onPick(c); return }
    if (c.author_id) { onClose(); navigate(`/authors/${c.author_id}`); return }
    setAdding(c.key)
    try {
      const a = await api.post<Author>(`${V1}/authors`, { name: c.name, key: c.key })
      toast(`Added ${a.name}`); onClose(); navigate(`/authors/${a.id}`)
    } catch (e) { toast((e as Error).message, 'err') } finally { setAdding('') }
  }
  return (
    <Modal title={onPick ? 'Which Open Library author is it?' : 'Find an author'} onClose={onClose} size="lg">
      <div className="book-search">
        <div className="search-bar"><Icon name="search" size={16} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Author name" autoFocus />{busy && <span className="spinner sm" />}</div>
        {results && results.length === 0 && !busy && <p className="muted mt">No results. Try the name in another spelling.</p>}
        {!results && <p className="muted small mt">{q.trim().length > 0 && q.trim().length < 3 ? 'Keep typing…' : 'Searches Open Library. Adding an author fetches a short biography and photo, and lists their books so you can plan what to read.'}</p>}
        {results && results.length > 0 && (
          <ul className="results">
            {results.map(c => (
              <li key={c.key}>
                <button type="button" onClick={() => add(c)} disabled={adding !== ''}>
                  <img className="author-photo sm" src={c.photo_url} alt="" loading="lazy" onError={e => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} />
                  <span className="info">
                    <span className="nm">{c.name} <span className="muted small">{lifespan(c.birth_date, c.death_date)}</span></span>
                    <span className="meta">{c.work_count} works{c.top_work && <> · best known for <i>{c.top_work}</i></>}{c.subjects?.length ? <> · {c.subjects.slice(0, 2).join(', ')}</> : null}</span>
                  </span>
                  <span className="tags">{adding === c.key ? <span className="spinner sm" /> : c.author_id && !onPick ? <span className="badge cat">on your list</span> : <span className="badge">{onPick ? 'Use' : 'Add'}</span>}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  )
}
