import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Quote } from '../lib/types'
import { Card, Confirm, Empty, Loading, PageHeader, useAsync, useDebounce, useToast } from '../components/ui'
import { Cover } from '../components/Cover'
import { QuoteModal } from './Book'

export default function Quotes() {
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const { data, loading, reload } = useAsync(() => api.get<Quote[]>(`${V1}/quotes${dq ? `?q=${encodeURIComponent(dq)}` : ''}`), [dq])
  const random = useAsync(() => api.get<Quote[]>(`${V1}/quotes?random=1`), [])
  const [edit, setEdit] = useState<Quote | null>(null)
  const [del, setDel] = useState<Quote | null>(null)
  const r = random.data?.[0]
  return (
    <>
      <PageHeader title="Quotes" sub={data && `${data.length} saved`} />
      {r && !dq && (
        <figure className="quote-hero mb">
          <blockquote>{r.text}</blockquote>
          <figcaption>— <Link to={`/books/${r.book_id}`}>{r.book_title}</Link>{r.book_authors && <>, {r.book_authors}</>}</figcaption>
          <button className="btn ghost sm" onClick={random.reload} title="Another quote">↻</button>
        </figure>
      )}
      <div className="toolbar"><div className="search"><span className="ico">⌕</span><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search quotes, notes, books…" /></div></div>
      {loading && !data ? <Loading /> : data && data.length === 0 ? (
        <Card><Empty title={dq ? 'No quotes match' : 'No quotes yet'}>{dq ? '' : 'Open a book and use “+ Add quote” to keep a passage.'}</Empty></Card>
      ) : (
        <div className="quote-list">
          {data?.map(qt => (
            <figure key={qt.id} className="quote card">
              <Link to={`/books/${qt.book_id}`} className="q-cover"><Cover book={{ id: qt.book_id, title: qt.book_title, authors: qt.book_authors, cover: qt.book_cover }} size="xs" /></Link>
              <div>
                <blockquote>{qt.text}</blockquote>
                <figcaption>
                  <Link to={`/books/${qt.book_id}`}>{qt.book_title}</Link>{qt.page && <span className="muted"> · p. {qt.page}</span>}
                  {qt.note && <span className="note">{qt.note}</span>}
                  <span className="spacer" />
                  <button className="btn ghost sm" onClick={() => setEdit(qt)}>Edit</button>
                  <button className="btn ghost sm danger-text" onClick={() => setDel(qt)}>✕</button>
                </figcaption>
              </div>
            </figure>
          ))}
        </div>
      )}
      {edit && <QuoteModal bookId={edit.book_id} quote={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload() }} />}
      {del && <Confirm title="Delete this quote?" message={<i>“{del.text.slice(0, 120)}”</i>} onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/quotes/${del.id}`); setDel(null); toast('Quote deleted'); reload() }} />}
    </>
  )
}
