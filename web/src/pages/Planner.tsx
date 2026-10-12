import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Book, Dashboard, SeriesNext } from '../lib/types'
import { fmtMonth } from '../lib/format'
import { Card, Confirm, Empty, Loading, PageHeader, useAsync, useToast } from '../components/ui'
import { Cover, RemoteCover } from '../components/Cover'
import { AddBook, workCandidate } from '../components/AddBook'

function thisMonth(): string { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}` }

/** The reading plan: want-to-read books in the order you mean to read them,
 *  "Up next" first, then whatever is pencilled in for a month. */
export default function Planner() {
  const { settings } = useApp()
  const { toast } = useToast()
  const { data, setData, loading, reload } = useAsync(() => api.get<Book[]>(`${V1}/books?status=want&sort=queue`), [])
  const dash = useAsync(() => api.get<Dashboard>(`${V1}/dashboard`), [])
  const [adding, setAdding] = useState(false)
  const [removing, setRemoving] = useState<Book | null>(null)
  const next = useAsync(() => api.get<SeriesNext[]>(`${V1}/series/next`), [])
  const [addNext, setAddNext] = useState<SeriesNext | null>(null)

  const groups: { key: string; books: Book[] }[] = []
  for (const b of data || []) {
    const g = groups.find(x => x.key === b.planned_for)
    if (g) g.books.push(b); else groups.push({ key: b.planned_for, books: [b] })
  }

  const move = async (b: Book, dir: -1 | 1) => {
    if (!data) return
    const group = data.filter(x => x.planned_for === b.planned_for)
    const i = group.findIndex(x => x.id === b.id), j = i + dir
    if (j < 0 || j >= group.length) return
    const order = [...data]
    const a = order.findIndex(x => x.id === group[i].id), c = order.findIndex(x => x.id === group[j].id);
    [order[a], order[c]] = [order[c], order[a]]
    setData(order)
    try { await api.put(`${V1}/planner/order`, { ids: order.map(x => x.id) }) } catch (e) { toast((e as Error).message, 'err'); reload() }
  }
  const setMonth = async (b: Book, m: string) => {
    try { await api.put(`${V1}/books/${b.id}`, { planned_for: m }); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const start = async (b: Book) => {
    try { await api.post(`${V1}/books/${b.id}/status`, { status: 'reading' }); toast(`Started “${b.title}” — good luck!`); reload(); dash.reload(); next.reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const remove = async (b: Book) => { setRemoving(null); await api.del(`${V1}/books/${b.id}`); toast('Removed from your plan'); reload() }

  const goal = settings?.yearly_goal || 0
  const d = dash.data
  const monthsLeft = 12 - new Date().getMonth()
  const toGo = d ? Math.max(0, goal - d.read_this_year) : 0

  return (
    <>
      <PageHeader title="Reading plan" sub={data && `${data.length} ${data.length === 1 ? 'book' : 'books'} waiting`} actions={<button className="btn primary" onClick={() => setAdding(true)}>+ Add to plan</button>} />
      {goal > 0 && d && (
        <div className="callout mb">
          {toGo === 0 ? <>Yearly goal of {goal} reached 🎉 — everything from here is a bonus.</> : <><b>{toGo}</b> more {toGo === 1 ? 'book' : 'books'} to reach {goal} this year — about <b>{(toGo / monthsLeft).toFixed(1)}</b> a month. {data && data.length < toGo && <span className="muted">Your plan has {data.length}.</span>}</>}
        </div>
      )}
      {next.data && next.data.length > 0 && (
        <Card className="mb" title="Next in your series">
          <ul className="mini-list">
            {next.data.map(n => (
              <li key={n.series}>
                {n.book ? <Link to={`/books/${n.book.id}`}><Cover book={n.book} size="xs" /></Link> : <button type="button" className="series-open" onClick={() => setAddNext(n)} title="See the book"><RemoteCover url={n.thumb_url || ''} title={n.title} size="xs" /></button>}
                <span className="grow">{n.book ? <b>{n.book.title}</b> : <button type="button" className="series-open" onClick={() => setAddNext(n)}><b>{n.title}</b></button>}
                  <span className="muted small block"><Link to={`/series/${encodeURIComponent(n.series)}`}>{n.series}</Link> · #{n.no}{n.total ? ` of ${n.total}` : ''} · you read {n.read}{n.book ? ' · already in your plan' : ''}</span></span>
                {n.book ? <button className="btn sm" onClick={() => start(n.book!)}>Start</button> : <button className="btn sm" onClick={() => setAddNext(n)}>+ Add to plan</button>}
              </li>
            ))}
          </ul>
        </Card>
      )}
      {loading && !data ? <Loading /> : !data || data.length === 0 ? (
        <Card><Empty title="Nothing planned yet">Add books you want to read. Put them in order, pencil some in for a month, and start the next one from here.</Empty></Card>
      ) : groups.map(g => (
        <section key={g.key || 'next'} className="plan-group">
          <h3>{g.key ? fmtMonth(g.key) : 'Up next'}{g.key && g.key < thisMonth() && <span className="badge warn">overdue</span>}<span className="muted small">{g.books.length}</span></h3>
          {g.books.map((b, i) => (
            <div key={b.id} className="plan-row">
              <span className="pos">{i + 1}</span>
              <Link to={`/books/${b.id}`}><Cover book={b} size="xs" /></Link>
              <div className="info">
                <Link to={`/books/${b.id}`} className="nm">{b.title}</Link>
                <div className="muted small">{b.authors}{b.pages > 0 && <> · {b.pages} p.</>}</div>
                {b.plan_note && <div className="note small">“{b.plan_note}”</div>}
              </div>
              <div className="actions">
                <input type="month" value={b.planned_for} onChange={e => setMonth(b, e.target.value)} title="Plan for a month (empty = up next)" aria-label="Planned month" />
                <span className="order">
                  <button className="btn ghost sm" onClick={() => move(b, -1)} disabled={i === 0} aria-label="Move up">▲</button>
                  <button className="btn ghost sm" onClick={() => move(b, 1)} disabled={i === g.books.length - 1} aria-label="Move down">▼</button>
                </span>
                <button className="btn sm" onClick={() => start(b)}>Start</button>
                <button className="btn ghost sm danger-text" onClick={() => setRemoving(b)} aria-label="Remove">✕</button>
              </div>
            </div>
          ))}
        </section>
      ))}
      {adding && <AddBook status="want" onClose={() => setAdding(false)} onAdded={reload} />}
      {addNext && <AddBook status="want" initial={workCandidate(addNext, addNext.author.split(',')[0])}
        query={addNext.ol_work ? undefined : `${addNext.title} ${addNext.author.split(',')[0]}`} extra={{ series: addNext.series, series_no: addNext.no }} onClose={() => setAddNext(null)} onAdded={() => { reload(); next.reload() }} />}
      {removing && <Confirm title={`Remove “${removing.title}”?`} message="It is deleted from your shelf. (To keep it but not plan it, just leave it at the end.)" onCancel={() => setRemoving(null)} onConfirm={() => remove(removing)} />}
    </>
  )
}
