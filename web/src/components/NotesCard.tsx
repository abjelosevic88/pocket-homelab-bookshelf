import { api } from '../lib/api'
import { Card, useAsync } from './ui'

// Your notes about this, from Pocket Notes (notes there that link to this page), and a way to
// start one. Shows nothing where there is no Pocket Notes to link to (no Pocket Core, as when
// Bookshelf runs on its own); greyed out, saying what to switch on, where the link is off.
interface NotesBox {
  available: boolean
  missing?: boolean
  reason?: string
  notes: { id: number; title: string; folder: string; excerpt: string; modified: number; url: string }[]
  new_url?: string
  notes_url?: string
}

const when = (unix: number) => new Date(unix * 1000).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })

export function NotesCard({ src, className = 'mt' }: { src: string; className?: string }) {
  const { data } = useAsync(() => api.get<NotesBox>(src), [src])
  if (!data || data.missing) return null
  const add = data.available && data.new_url
    ? <a className="btn ghost sm" href={data.new_url} target="_blank" rel="noreferrer">+ New note</a>
    : <span title={data.reason}><button className="btn ghost sm" disabled aria-disabled>+ New note</button></span>
  return (
    <Card className={className} title={`Notes${data.notes.length ? ` (${data.notes.length})` : ''}`} actions={add}>
      {!data.available ? <p className="muted small notes-off">🔒 {data.reason}</p>
        : data.notes.length === 0 ? <p className="muted small">No notes about this yet. <b>+ New note</b> starts one in Pocket Notes, linked here; any note that links to this page shows up too.</p>
        : <ul className="mini-list notes-box">{data.notes.map(n => (
            <li key={n.id}><a href={n.url} target="_blank" rel="noreferrer" className="grow">
              <b>{n.title || 'Untitled'}</b>
              {n.excerpt && <span className="muted small clamp2">{n.excerpt}</span>}
              <span className="muted small">{n.folder || 'Notes'} · {when(n.modified)}</span>
            </a></li>))}
          </ul>}
    </Card>
  )
}
