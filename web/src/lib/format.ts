import type { Book, Status } from './types'

export function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function ago(iso: string): string {
  const t = new Date(iso).getTime()
  const s = Math.max(0, (Date.now() - t) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  if (s < 86400 * 7) return `${Math.floor(s / 86400)} d ago`
  return new Date(iso).toLocaleDateString()
}

/** "2026-03-14" → "14 Mar 2026"; "" stays "". */
export function fmtDate(d: string): string {
  if (!d) return ''
  const [y, m, day] = d.slice(0, 10).split('-').map(Number)
  return new Date(y, m - 1, day).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}

/** "2026-11" → "November 2026". */
export function fmtMonth(ym: string): string {
  const [y, m] = ym.split('-').map(Number)
  return new Date(y, m - 1, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
}

export function daysBetween(a: string, b: string): number {
  const p = (s: string) => { const [y, m, d] = s.split('-').map(Number); return new Date(y, m - 1, d).getTime() }
  return Math.round((p(b) - p(a)) / 86400000)
}

export function initials(name: string): string {
  const parts = name.trim().split(/[\s@._-]+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[1][0]).toUpperCase()
}

const PALETTE = ['#2563eb', '#7c3aed', '#0891b2', '#059669', '#d97706', '#dc2626', '#db2777', '#4b5563', '#b45309', '#0d9488']
export function colorFor(id: number): string { return PALETTE[Math.abs(id) % PALETTE.length] }

/** Stable muted colour for a cover placeholder, from the title. */
const COVERS = ['#5b4b8a', '#2f5d62', '#8a4b4b', '#3e5a87', '#6b5b3e', '#4a6b45', '#7a4a6e', '#33475b', '#8a6a3e', '#4b4b6b']
export function coverColor(title: string): string {
  let h = 0
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) | 0
  return COVERS[Math.abs(h) % COVERS.length]
}

export function splitTags(s: string): string[] { return s.split(',').map(t => t.trim()).filter(Boolean) }

export const STATUS_LABEL: Record<Status, string> = { want: 'Want to read', reading: 'Reading', read: 'Read', abandoned: 'Abandoned' }
export const STATUS_SHORT: Record<Status, string> = { want: 'Want', reading: 'Reading', read: 'Read', abandoned: 'Abandoned' }
export const FORMAT_LABEL: Record<string, string> = { paper: 'Paper', ebook: 'E-book', audio: 'Audiobook', unknown: 'Not set' }

export function coverSrc(b: { id: number; cover: string }): string { return `/api/v1/books/${b.id}/cover?v=${encodeURIComponent(b.cover)}` }

export function progress(b: Pick<Book, 'current_page' | 'pages'> & { reader_percent?: number | null }): number {
  if (!b.pages) return b.reader_percent != null ? Math.round(b.reader_percent * 100) : 0
  if (!b.current_page) return 0
  return Math.min(100, Math.round((b.current_page * 100) / b.pages))
}

/** Whether there is a percentage to show: a page count, or an e-reader that syncs the book. */
export function hasProgress(b: Pick<Book, 'pages'> & { reader_percent?: number | null }): boolean {
  return b.pages > 0 || b.reader_percent != null
}

export function ratingText(r: number): string { return r ? `${r % 1 ? r.toFixed(1) : r}★` : '' }

/** "142" → "p. 142"; "loc. 1201" and other free text stay as they are. */
export function pageLabel(page: string): string { return /^\d/.test(page) ? `p. ${page}` : page }

/** "21 October 1929" / "1929" → "1929". */
export function year(d: string): string { const m = d.match(/\d{4}/); return m ? m[0] : '' }

/** "1929–2018", "b. 1966", or "". */
export function lifespan(birth: string, death: string): string {
  const b = year(birth), d = year(death)
  if (b && d) return `${b}–${d}`
  if (b) return `b. ${b}`
  if (d) return `d. ${d}`
  return ''
}

export function authorPhotoSrc(a: { id: number; photo: string }): string { return `/api/v1/authors/${a.id}/photo?v=${encodeURIComponent(a.photo)}` }
