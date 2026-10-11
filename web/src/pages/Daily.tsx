import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { DailySet } from '../lib/types'
import { fmtDate } from '../lib/format'
import { Card, Empty, Loading, PageHeader, useAsync, useToast } from '../components/ui'
import { QuoteCard } from '../components/QuoteCard'

/** Today's highlights (and earlier days'), Readwise style. */
export default function Daily() {
  const { toast } = useToast()
  const [params, setParams] = useSearchParams()
  const date = params.get('date') || ''
  const { data, loading, reload, setData } = useAsync(() => api.get<{ set: DailySet; days: string[]; today: string }>(`${V1}/daily${date ? `?date=${date}` : ''}`), [date])
  const shuffle = async () => {
    try { const r = await api.post<{ set: DailySet }>(`${V1}/daily/shuffle`); if (data) setData({ ...data, set: r.set }); toast('New highlights for today') } catch (e) { toast((e as Error).message, 'err') }
  }
  if (loading && !data) return <Loading />
  if (!data) return null
  const { set, days, today } = data
  const isToday = set.date === today
  const i = days.indexOf(set.date)
  const older = i >= 0 ? days[i + 1] : days[0]
  const newer = i > 0 ? days[i - 1] : ''
  const go = (d: string) => setParams(d && d !== today ? { date: d } : {})
  return (
    <>
      <PageHeader title={isToday ? 'Today’s highlights' : `Highlights · ${fmtDate(set.date)}`}
        sub={isToday ? <>Picked from {set.quotes.length ? 'the ones you have not seen for longest' : 'your quotes'} — favourites come back more often.{set.sent_at && <> Sent to your phone.</>}</> : 'An earlier day'}
        actions={<>
          {older && <button className="btn ghost" onClick={() => go(older)}>‹ Earlier</button>}
          {!isToday && <button className="btn ghost" onClick={() => go(newer)}>{newer ? 'Later ›' : 'Today ›'}</button>}
          {isToday && <button className="btn" onClick={shuffle}>↻ Shuffle</button>}
        </>} />
      {set.quotes.length === 0 ? (
        <Card><Empty title={isToday ? 'No highlights yet' : 'Nothing on this day'}>{isToday ? <>Add quotes to your books or <Link to="/quotes">import Kindle highlights</Link>.</> : ''}</Empty></Card>
      ) : (
        <div className="daily-list">
          {set.quotes.map(q => <QuoteCard key={q.id} quote={q} big onChange={() => reload()} />)}
        </div>
      )}
      <p className="muted small mt">♡ makes a highlight come back more often; Hide keeps it out of the daily review (it stays in Quotes). Delivery to your phone: <Link to="/settings?tab=daily">Settings → Daily highlights</Link>.</p>
    </>
  )
}
