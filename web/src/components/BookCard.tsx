import { Link } from 'react-router-dom'
import type { Book } from '../lib/types'
import { fmtDate, hasProgress, progress, splitTags, STATUS_SHORT } from '../lib/format'
import { Cover } from './Cover'
import { FormatBadge } from './FormatBadge'
import { Stars } from './Stars'
import { Icon } from './icons'

export function BookCard({ book }: { book: Book }) {
  const pct = progress(book)
  return (
    <Link to={`/books/${book.id}`} className="book-card">
      <div className="cover-wrap">
        <Cover book={book} size="md" />
        {book.favorite && <span className="fav" title="Favourite"><Icon name="heart" size={12} fill /></span>}
        {book.status !== 'read' && <span className={`status-pill ${book.status}`}>{STATUS_SHORT[book.status]}</span>}
        <FormatBadge format={book.format} compact className="on-cover" />
      </div>
      <div className="nm">{book.title}</div>
      <div className="au">{book.authors}</div>
      {book.status === 'reading' && hasProgress(book) && <div className="bar" title={`${pct}%`}><span className="fill" style={{ width: `${pct}%` }} /></div>}
      {book.rating > 0 && <Stars value={book.rating} size="sm" />}
    </Link>
  )
}

export function BookRow({ book }: { book: Book }) {
  const pct = progress(book)
  return (
    <Link to={`/books/${book.id}`} className="book-row">
      <Cover book={book} size="xs" />
      <div className="info">
        <div className="nm">{book.title}{book.favorite && <span className="fav-inline" title="Favourite"><Icon name="heart" size={12} fill className="inline-icon" /></span>}</div>
        <div className="meta">
          <FormatBadge format={book.format} />
          <span>{book.authors || 'Unknown author'}</span>
          {book.series && <span className="hide-sm">· {book.series}{book.series_no !== '' ? ` #${book.series_no}` : ''}</span>}
          {book.published && !book.series && <span className="hide-sm">· {book.published}</span>}
          {splitTags(book.tags).slice(0, 3).map(t => <span key={t} className="badge tag hide-sm">{t}</span>)}
        </div>
      </div>
      <div className="side">
        {book.rating > 0 ? <Stars value={book.rating} size="sm" /> : <span className={`status-pill inline ${book.status}`}>{book.status === 'reading' && hasProgress(book) ? `${pct}%` : STATUS_SHORT[book.status]}</span>}
        <span className="muted small">{book.status === 'read' ? fmtDate(book.finished_on) : book.status === 'reading' ? (book.started_on ? `since ${fmtDate(book.started_on)}` : '') : ''}</span>
      </div>
    </Link>
  )
}
