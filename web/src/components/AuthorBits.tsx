import { useEffect } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { authorPhotoSrc, coverColor, initials } from '../lib/format'
import { Loading } from './ui'

/** Round author photo, or initials on a colour. */
export function AuthorPhoto({ author, size = 'md' }: { author: { id: number; name: string; photo: string }; size?: 'sm' | 'md' | 'lg' }) {
  if (author.photo) return <img className={`author-photo ${size}`} src={authorPhotoSrc(author)} alt="" loading="lazy" />
  return <span className={`author-photo ph ${size}`} style={{ background: coverColor(author.name) }} aria-hidden>{initials(author.name)}</span>
}

/** Link to an author by name (books store names, not ids). */
export function AuthorLink({ name, className }: { name: string; className?: string }) {
  return <Link to={`/author/${encodeURIComponent(name)}`} className={className}>{name}</Link>
}

/** Route /author/:name → finds the author and opens their page. */
export function AuthorByName() {
  const { name } = useParams()
  const navigate = useNavigate()
  useEffect(() => {
    api.get<{ id: number }>(`${V1}/authors/resolve?name=${encodeURIComponent(name || '')}`)
      .then(r => navigate(`/authors/${r.id}`, { replace: true }))
      .catch(() => navigate(`/library?author=${encodeURIComponent(name || '')}`, { replace: true }))
  }, [name, navigate])
  return <Loading />
}
