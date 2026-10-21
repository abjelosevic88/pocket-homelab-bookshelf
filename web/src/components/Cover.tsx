import { coverColor, coverSrc } from '../lib/format'

/** The book's cover, or a coloured placeholder with title and author. */
export function Cover({ book, size = 'md' }: { book: { id: number; title: string; authors?: string; cover: string }; size?: 'xxs' | 'xs' | 'sm' | 'md' | 'lg' }) {
  if (book.cover) return <img className={`cover ${size}`} src={coverSrc(book)} alt="" loading="lazy" />
  return (
    <div className={`cover ph ${size}`} style={{ background: coverColor(book.title) }} aria-hidden>
      <span className="t">{book.title}</span>
      {book.authors && size !== 'xs' && size !== 'xxs' && <span className="a">{book.authors.split(',')[0]}</span>}
    </div>
  )
}

/** A remote image (search results) with the same placeholder fallback. */
export function RemoteCover({ url, title, size = 'sm' }: { url: string; title: string; size?: 'xs' | 'sm' | 'md' | 'lg' }) {
  if (url) return <img className={`cover ${size}`} src={url} alt="" loading="lazy" referrerPolicy="no-referrer" />
  return <div className={`cover ph ${size}`} style={{ background: coverColor(title) }} aria-hidden><span className="t">{title}</span></div>
}
