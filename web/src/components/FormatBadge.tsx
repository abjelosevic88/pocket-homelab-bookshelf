// What kind of book it is: audiobook, e-book, paper, or not set.
import type { Format } from '../lib/types'
import { FORMAT_LABEL } from '../lib/format'
import { Icon, type IconName } from './icons'

const ICON: Record<string, IconName> = { audio: 'headphones', ebook: 'tablet', paper: 'book', other: 'help' }

/** A pill with the format; `compact` is an icon-only dot for covers. */
export function FormatBadge({ format, compact = false, className = '' }: { format: Format; compact?: boolean; className?: string }) {
  const kind = format || 'other'
  const label = format ? FORMAT_LABEL[format] : 'Other'
  if (compact) {
    return <span className={`fmt-dot ${kind} ${className}`} title={label} aria-label={label}><Icon name={ICON[kind]} size={12} strokeWidth={2} /></span>
  }
  return <span className={`fmt-badge ${kind} ${className}`}><Icon name={ICON[kind]} size={12} strokeWidth={2} />{label}</span>
}
