import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Book, Dashboard, Quote } from '../lib/types'
import { daysBetween, fmtDate, progress, todayISO } from '../lib/format'
import { Card, Empty, Loading, PageHeader, useAsync, useToast } from '../components/ui'
import { Cover } from '../components/Cover'
import { Stars } from '../components/Stars'
import { AddBook } from '../components/AddBook'
import { FinishModal } from '../components/FinishModal'

export default function Now() {
  const { settings } = useApp()
  const { toast } = useToast()
  const reading = useAsync(() => api.get<Book[]>(`${V1}/books?status=reading&sort=recent`), [])
  const next = useAsync(() => api.get<Book[]>(`${V1}/books?status=want&sort=queue&limit=3`), [])
  const recent = useAsync(() => api.get<Book[]>(`${V1}/books?status=read&sort=finished&limit=6`), [])
  const dash = useAsync(() => api.get<Dashboard>(`${V1}/dashboard`), [])
  const quote = useAsync(() => api.get<Quote[]>(`${V1}/quotes?random=1`), [])
  const [adding, setAdding] = useState(false)
  const [finishing, setFinishing] = useState<Book | null>(null)
  const reloadAll = () => { reading.reload(); next.reload(); recent.reload(); dash.reload() }

  const start = async (b: Book) => {
    try { await api.post(`${V1}/books/${b.id}/status`, { status: 'reading' }); toast(`Started “${b.title}”`); reloadAll() } catch (e) { toast((e as Error).message, 'err') }
  }
  const d = dash.data
  const goal = settings?.yearly_goal || 0
  const year = new Date().getFullYear()
  const q = quote.data?.[0]

  return (
    <>
      <PageHeader title="Reading now" sub={d && <>{d.read_this_year} {d.read_this_year === 1 ? 'book' : 'books'} read in {year}{d.pages_this_year > 0 && <> · {d.pages_this_year.toLocaleString()} pages</>}</>}
        actions={<button className="btn primary" onClick={() => setAdding(true)}>+ Add book</button>} />

      {goal > 0 && d && (
        <Link to="/stats" className="goal card">
          <div className="goal-text"><b>{d.read_this_year}</b> of {goal} books this year<span className="muted"> · {goalPace(d.read_this_year, goal)}</span></div>
          <div className="bar lg"><span className="fill" style={{ width: `${Math.min(100, (d.read_this_year * 100) / goal)}%` }} /></div>
        </Link>
      )}

      {reading.loading && !reading.data ? <Loading /> : reading.data && reading.data.length > 0 ? (
        <div className="now-list">
          {reading.data.map(b => <ReadingCard key={b.id} book={b} onChanged={reloadAll} onFinish={() => setFinishing(b)} />)}
        </div>
      ) : (
        <Card><Empty title="Not reading anything right now">Pick something from your plan below, or add a book.</Empty></Card>
      )}

      <div className="two-col mt">
        <Card title="Up next" actions={<Link to="/planner" className="btn ghost sm">Plan →</Link>}>
          {next.data && next.data.length > 0 ? (
            <ul className="mini-list">
              {next.data.map(b => (
                <li key={b.id}>
                  <Link to={`/books/${b.id}`} className="row grow"><Cover book={b} size="xs" /><span><b>{b.title}</b><span className="muted small block">{b.authors}</span>{b.plan_note && <span className="muted small block note">“{b.plan_note}”</span>}</span></Link>
                  <button className="btn sm" onClick={() => start(b)}>Start</button>
                </li>
              ))}
            </ul>
          ) : <p className="muted">Your plan is empty. Add books you want to read and they line up here.</p>}
        </Card>
        <Card title="Recently finished" actions={<Link to="/library?status=read&sort=finished" className="btn ghost sm">All →</Link>}>
          {recent.data && recent.data.length > 0 ? (
            <ul className="mini-list">
              {recent.data.map(b => (
                <li key={b.id}>
                  <Link to={`/books/${b.id}`} className="row grow"><Cover book={b} size="xs" /><span><b>{b.title}</b><span className="muted small block">{b.authors}{b.finished_on && <> · {fmtDate(b.finished_on)}</>}</span></span></Link>
                  {b.rating > 0 && <Stars value={b.rating} size="sm" />}
                </li>
              ))}
            </ul>
          ) : <p className="muted">Finished books show up here with your rating.</p>}
        </Card>
      </div>

      {q && (
        <figure className="quote-hero mt">
          <blockquote>{q.text}</blockquote>
          <figcaption>— <Link to={`/books/${q.book_id}`}>{q.book_title}</Link>{q.book_authors && <>, {q.book_authors}</>}{q.page && <span className="muted"> · p. {q.page}</span>}</figcaption>
          <button className="btn ghost sm" onClick={quote.reload} title="Another quote">↻</button>
        </figure>
      )}

      {adding && <AddBook status="reading" onClose={() => setAdding(false)} onAdded={reloadAll} />}
      {finishing && <FinishModal book={finishing} onClose={() => setFinishing(null)} onDone={() => { setFinishing(null); reloadAll() }} />}
    </>
  )
}

function goalPace(done: number, goal: number): string {
  const now = new Date()
  const start = new Date(now.getFullYear(), 0, 1).getTime()
  const end = new Date(now.getFullYear() + 1, 0, 1).getTime()
  const expected = Math.round((goal * (now.getTime() - start)) / (end - start))
  if (done >= goal) return 'goal reached 🎉'
  if (done > expected) return `${done - expected} ahead of schedule`
  if (done === expected) return 'right on schedule'
  return `${expected - done} behind schedule`
}

function ReadingCard({ book, onChanged, onFinish }: { book: Book; onChanged: () => void; onFinish: () => void }) {
  const { toast } = useToast()
  const [page, setPage] = useState(String(book.current_page || ''))
  const pct = progress(book)
  const save = async () => {
    const n = Number(page)
    if (!Number.isFinite(n) || n === book.current_page) return
    try { await api.post(`${V1}/books/${book.id}/progress`, { page: n }); onChanged() } catch (e) { toast((e as Error).message, 'err') }
  }
  const days = book.started_on ? daysBetween(book.started_on, todayISO()) : null
  return (
    <div className="reading-card card">
      <Link to={`/books/${book.id}`}><Cover book={book} size="sm" /></Link>
      <div className="body">
        <Link to={`/books/${book.id}`} className="nm">{book.title}</Link>
        <div className="muted">{book.authors}</div>
        {days !== null && <div className="muted small">{days === 0 ? 'Started today' : `Day ${days + 1}`} · since {fmtDate(book.started_on)}</div>}
        <div className="progress-row">
          {book.pages > 0 && <div className="bar lg"><span className="fill" style={{ width: `${pct}%` }} /></div>}
          <label className="page-input">
            <span className="muted small">Page</span>
            <input type="number" min={0} max={book.pages || undefined} value={page} onChange={e => setPage(e.target.value)} onBlur={save} onKeyDown={e => e.key === 'Enter' && (e.target as HTMLInputElement).blur()} />
            {book.pages > 0 && <span className="muted small">of {book.pages} · {pct}%</span>}
          </label>
        </div>
        <div className="row mt-sm"><button className="btn primary sm" onClick={onFinish}>✓ Finished</button><Link to={`/books/${book.id}`} className="btn ghost sm">Notes & quotes</Link></div>
      </div>
    </div>
  )
}
