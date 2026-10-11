import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Stats as StatsT } from '../lib/types'
import { fmtDate, FORMAT_LABEL } from '../lib/format'
import { Card, Loading, PageHeader, Segmented, useAsync } from '../components/ui'
import { Stars } from '../components/Stars'
import { AuthorLink } from '../components/AuthorBits'

const MONTHS = ['J', 'F', 'M', 'A', 'M', 'J', 'J', 'A', 'S', 'O', 'N', 'D']

export default function Stats() {
  const [year, setYear] = useState(new Date().getFullYear())
  const { data: s } = useAsync(() => api.get<StatsT>(`${V1}/stats?year=${year}`), [year])
  if (!s) return <Loading />
  const maxMonth = Math.max(1, ...s.months.map(m => m.books))
  const maxRating = Math.max(1, ...s.ratings)
  const goalPct = s.goal ? Math.min(100, Math.round((s.totals.books * 100) / s.goal)) : 0
  return (
    <>
      <PageHeader title="Stats" actions={s.years.length > 1 && <Segmented size="sm" value={year} onChange={setYear} options={s.years.slice(0, 6).map(y => ({ value: y, label: String(y) }))} />} />
      <div className="stats">
        <Link to={`/library?year=${year}`} className="stat"><div className="n">{s.totals.books}</div><div className="l">books read in {year}</div></Link>
        <div className="stat"><div className="n">{s.totals.pages.toLocaleString()}</div><div className="l">pages</div></div>
        <div className="stat"><div className="n">{s.totals.avg_rating ? s.totals.avg_rating.toFixed(1) : '–'}</div><div className="l">average rating</div></div>
        <div className="stat"><div className="n">{s.abandoned}</div><div className="l">abandoned</div></div>
      </div>
      {s.goal > 0 && (
        <Card className="mb">
          <div className="goal-text"><b>{s.totals.books}</b> of {s.goal} books · {goalPct}%</div>
          <div className="bar lg"><span className="fill" style={{ width: `${goalPct}%` }} /></div>
        </Card>
      )}
      <div className="two-col">
        <Card title="Books per month">
          <div className="month-bars">
            {s.months.map((m, i) => (
              <div key={i} className="mb-col" title={`${m.books} books · ${m.pages} pages`}>
                <span className="v">{m.books || ''}</span>
                <span className="b" style={{ height: `${(m.books / maxMonth) * 100}%` }} />
                <span className="l">{MONTHS[i]}</span>
              </div>
            ))}
          </div>
        </Card>
        <Card title="Ratings">
          <div className="rating-bars">
            {[...s.ratings].map((n, i) => ({ n, v: (i + 1) / 2 })).reverse().map(({ n, v }) => (
              <div key={v} className="rb-row"><span className="l">{v % 1 ? v.toFixed(1) : `${v}.0`}★</span><span className="track"><span className="b" style={{ width: `${(n / maxRating) * 100}%` }} /></span><span className="n">{n || ''}</span></div>
            ))}
          </div>
        </Card>
      </div>
      <div className="three-col mt">
        <Card title="Authors">
          {s.top_authors.length ? <ol className="rank">{s.top_authors.map(a => <li key={a.name}><AuthorLink name={a.name} /><span className="muted">{a.count}</span></li>)}</ol> : <p className="muted">—</p>}
        </Card>
        <Card title="Tags">
          {s.top_tags.length ? <ol className="rank">{s.top_tags.map(t => <li key={t.name}><Link to={`/library?tag=${encodeURIComponent(t.name)}`}>{t.name}</Link><span className="muted">{t.count}</span></li>)}</ol> : <p className="muted">Tag books to see what you read most.</p>}
        </Card>
        <Card title="Formats">
          {s.formats.length ? <ol className="rank">{s.formats.map(f => <li key={f.name}><span>{FORMAT_LABEL[f.name] || f.name}</span><span className="muted">{f.count}</span></li>)}</ol> : <p className="muted">—</p>}
          {s.longest && <p className="small mt"><span className="muted">Longest:</span> <Link to={`/books/${s.longest.id}`}>{s.longest.title}</Link> <span className="muted">({s.longest.pages} p.)</span></p>}
        </Card>
      </div>
      <Card title={`Finished in ${year}`} className="mt">
        {s.finished.length ? (
          <ul className="mini-list">{s.finished.map((b, i) => (
            <li key={`${b.id}-${i}`}><Link to={`/books/${b.id}`} className="grow"><b>{b.title}</b> <span className="muted small">{b.authors}</span></Link>{b.rating ? <Stars value={b.rating} size="sm" /> : null}<span className="muted small date">{fmtDate(b.date || '')}</span></li>
          ))}</ul>
        ) : <p className="muted">Nothing finished in {year} yet.</p>}
      </Card>
      <p className="muted small mt">All time: {s.all_time.books} books · {s.all_time.pages.toLocaleString()} pages{s.all_time.avg_rating > 0 && <> · average {s.all_time.avg_rating.toFixed(1)}★</>} · {s.quotes} quotes</p>
    </>
  )
}
