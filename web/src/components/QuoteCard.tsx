import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Quote } from '../lib/types'
import { pageLabel } from '../lib/format'
import { useToast } from './ui'
import { Cover } from './Cover'
import { Icon } from './icons'

/** A highlight with ♥ favourite and hide (= never in the daily review). */
export function QuoteCard({ quote, onChange, big = false }: { quote: Quote; onChange?: (q: Quote) => void; big?: boolean }) {
  const { toast } = useToast()
  const [q, setQ] = useState(quote)
  const flag = async (f: { favorite?: boolean; hidden?: boolean }) => {
    const next = { ...q, ...f }
    setQ(next)
    try { await api.post(`${V1}/quotes/${q.id}/flags`, f); onChange?.(next); if (f.hidden) toast('Hidden: it will not come back in your daily highlights') } catch (e) { setQ(q); toast((e as Error).message, 'err') }
  }
  return (
    <figure className={`quote card daily-quote ${big ? 'big' : ''} ${q.hidden ? 'is-hidden' : ''}`}>
      <Link to={`/books/${q.book_id}`} className="q-cover"><Cover book={{ id: q.book_id, title: q.book_title, authors: q.book_authors, cover: q.book_cover }} size={big ? 'sm' : 'xs'} /></Link>
      <div>
        <blockquote>{q.text}</blockquote>
        {q.note && <p className="q-note">— {q.note}</p>}
        <figcaption>
          <Link to={`/books/${q.book_id}`}>{q.book_title}</Link>{q.book_authors && <span className="muted">, {q.book_authors.split(',')[0]}</span>}{q.page && <span className="muted"> · {pageLabel(q.page)}</span>}
          <span className="spacer" />
          <button type="button" className={`btn ghost sm icon-only fav-q ${q.favorite ? 'on' : ''}`} onClick={() => flag({ favorite: !q.favorite })} title={q.favorite ? 'Favourite — comes back more often' : 'Favourite'} aria-label="Favourite" aria-pressed={q.favorite}><Icon name="heart" size={15} fill={q.favorite} /></button>
          <button type="button" className="btn ghost sm" onClick={() => flag({ hidden: !q.hidden })} title={q.hidden ? 'Show in daily highlights again' : 'Hide from daily highlights'}>{q.hidden ? 'Unhide' : 'Hide'}</button>
        </figcaption>
      </div>
    </figure>
  )
}
