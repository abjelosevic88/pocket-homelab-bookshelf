import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { SeriesSummary } from '../lib/types'
import { Card, Empty, Loading, PageHeader, useAsync, useToast } from '../components/ui'
import { Cover } from '../components/Cover'

export default function Series() {
  const { toast } = useToast()
  const { data, loading, reload } = useAsync(() => api.get<SeriesSummary[]>(`${V1}/series`), [])
  const detect = async () => {
    await api.post(`${V1}/series/detect`)
    toast('Looking for series in the background — this takes a minute')
    setTimeout(reload, 30000)
  }
  return (
    <>
      <PageHeader title="Series" sub={data && `${data.length} series on your shelf`} actions={<button className="btn ghost" onClick={detect} title="Look up all books without a series again">↻ Look for series</button>} />
      {loading && !data ? <Loading /> : !data || data.length === 0 ? (
        <Card><Empty title="No series yet">Series are found automatically (from Wikidata) for books linked to Open Library. You can also set one on a book under Edit details.</Empty></Card>
      ) : (
        <div className="series-grid">
          {data.map(s => {
            const total = Math.max(s.total, s.books)
            const pct = total ? Math.round((s.read / total) * 100) : 0
            return (
              <Link key={s.name} to={`/series/${encodeURIComponent(s.name)}`} className="series-card card">
                <div className="stack">{s.covers.map((b, i) => <div key={b.id} className="stack-item" style={{ zIndex: 10 - i, transform: `translateX(${i * 22}px)` }}><Cover book={b} size="xs" /></div>)}</div>
                <div className="info">
                  <div className="nm">{s.name}</div>
                  <div className="muted small">{s.authors}</div>
                  <div className="small mt-sm"><b>{s.read}</b> of {s.total ? s.total : '?'} read{s.want > 0 && <span className="muted"> · {s.want} planned</span>}{s.reading > 0 && <span> · reading</span>}</div>
                  {s.total > 0 && <div className="bar"><span className="fill" style={{ width: `${pct}%` }} /></div>}
                </div>
              </Link>
            )
          })}
        </div>
      )}
    </>
  )
}
