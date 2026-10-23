import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Icon } from './icons'

export interface SelectOption { value: string; label: string; hint?: string; group?: string; disabled?: boolean }

interface Props {
  value: string
  onChange: (value: string) => void
  options: SelectOption[]
  placeholder?: string
  /** Shown while filtering; "Create “x”" appears when nothing matches exactly. */
  onCreate?: (label: string) => Promise<string> | string
  createLabel?: string
  ariaLabel?: string
  className?: string
  disabled?: boolean
  /** Also offer an "× clear" row when a value is set (value "" means cleared). */
  clearLabel?: string
}

const Chevron = () => <span className="sel-chevron"><Icon name="chevron-down" size={14} /></span>

/** A searchable combobox: looks like an input, opens a filterable list, keyboard friendly. */
export function Select({ value, onChange, options, placeholder = 'Choose…', onCreate, createLabel = 'Create', ariaLabel, className = '', disabled, clearLabel }: Props) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [hl, setHl] = useState(0)
  const [busy, setBusy] = useState(false)
  const [pos, setPos] = useState<{ left: number; top?: number; bottom?: number; minWidth: number; maxWidth: number; maxH: number } | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const pop = useRef<HTMLDivElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const id = useId()

  const current = options.find(o => o.value === value)
  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    return s ? options.filter(o => o.label.toLowerCase().includes(s) || (o.hint || '').toLowerCase().includes(s)) : options
  }, [q, options])
  const exact = filtered.some(o => o.label.toLowerCase() === q.trim().toLowerCase())
  const canCreate = Boolean(onCreate) && q.trim() !== '' && !exact
  const rows: { kind: 'opt' | 'create' | 'clear'; opt?: SelectOption }[] = [
    ...(clearLabel && value && !q ? [{ kind: 'clear' as const }] : []),
    ...filtered.map(o => ({ kind: 'opt' as const, opt: o })),
    ...(canCreate ? [{ kind: 'create' as const }] : []),
  ]

  useEffect(() => {
    if (!open) return
    setQ(''); setHl(Math.max(0, rows.findIndex(r => r.opt?.value === value)))
    const t = setTimeout(() => search.current?.focus(), 0)
    // the list sits in a portal (fixed, so dialogs never clip it): a click in it is not outside
    const onDoc = (e: MouseEvent) => { const t = e.target as Node; if (root.current && !root.current.contains(t) && !pop.current?.contains(t)) setOpen(false) }
    document.addEventListener('mousedown', onDoc)
    return () => { clearTimeout(t); document.removeEventListener('mousedown', onDoc) }
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { setHl(0) }, [q])
  useLayoutEffect(() => {
    if (!open || !root.current) { setPos(null); return }
    const place = () => {
      const r = root.current!.getBoundingClientRect()
      const vw = window.innerWidth, vh = window.innerHeight
      const below = vh - r.bottom - 12, above = r.top - 12
      const minWidth = Math.min(r.width, vw - 16)
      const left = Math.max(8, Math.min(vw - Math.max(minWidth, 220) - 8, r.left))
      const maxWidth = Math.max(minWidth, Math.min(380, vw - left - 8))
      if (below < 300 && above > below) setPos({ left, bottom: vh - r.top + 4, minWidth, maxWidth, maxH: Math.min(330, above) })
      else setPos({ left, top: r.bottom + 4, minWidth, maxWidth, maxH: Math.min(330, below) })
    }
    place()
    // follow the field when the page or a dialog scrolls under it
    const onScroll = (e: Event) => { if (!pop.current?.contains(e.target as Node)) place() }
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', place)
    return () => { window.removeEventListener('scroll', onScroll, true); window.removeEventListener('resize', place) }
  }, [open])
  useEffect(() => {
    const el = list.current?.querySelector<HTMLElement>(`[data-i="${hl}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }, [hl, open])

  const pick = async (row: (typeof rows)[number]) => {
    if (row.kind === 'clear') { onChange(''); setOpen(false); return }
    if (row.kind === 'create' && onCreate) {
      setBusy(true)
      try { const v = await onCreate(q.trim()); onChange(v); setOpen(false) } finally { setBusy(false) }
      return
    }
    if (row.opt && !row.opt.disabled) { onChange(row.opt.value); setOpen(false) }
  }
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setHl(h => Math.min(rows.length - 1, h + 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setHl(h => Math.max(0, h - 1)) }
    else if (e.key === 'Enter') { e.preventDefault(); if (rows[hl]) pick(rows[hl]) }
    else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); setOpen(false) }
    else if (e.key === 'Tab') setOpen(false)
  }

  let group = ''
  return (
    <div ref={root} className={`sel ${className} ${open ? 'open' : ''}`} onKeyDown={open ? onKey : undefined}>
      <button type="button" className={`sel-btn ${current ? '' : 'is-empty'}`} onClick={() => !disabled && setOpen(o => !o)} aria-haspopup="listbox" aria-expanded={open} aria-label={ariaLabel} aria-controls={id} disabled={disabled}
        onKeyDown={e => { if (!open && (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); setOpen(true) } }}>
        <span className="sel-val">{current ? current.label : placeholder}</span>
        <Chevron />
      </button>
      {open && createPortal(
        <div ref={pop} className="sel-pop" data-keep-open style={{ position: 'fixed', left: pos?.left ?? -9999, top: pos?.top ?? 'auto', bottom: pos?.bottom ?? 'auto', minWidth: pos?.minWidth, maxWidth: pos?.maxWidth }}>
          <div className="sel-search"><Icon name="search" size={14} /><input ref={search} value={q} onChange={e => setQ(e.target.value)} placeholder="Type to filter…" aria-label="Filter options" autoComplete="off" /></div>
          <div className="sel-list" role="listbox" id={id} ref={list} style={{ maxHeight: pos ? Math.max(120, pos.maxH - 44) : undefined }}>
            {rows.length === 0 && <div className="sel-empty muted">No matches</div>}
            {rows.map((r, i) => {
              const head = r.opt?.group && r.opt.group !== group ? (group = r.opt.group) : null
              return (
                <div key={r.kind === 'opt' ? r.opt!.value : r.kind}>
                  {head && <div className="sel-group">{head}</div>}
                  <div role="option" aria-selected={r.opt?.value === value} aria-disabled={r.opt?.disabled || undefined} data-i={i} className={`sel-opt ${i === hl ? 'hl' : ''} ${r.opt && r.opt.value === value ? 'cur' : ''} ${r.kind !== 'opt' ? 'action' : ''} ${r.opt?.disabled ? 'off' : ''}`}
                    onMouseEnter={() => setHl(i)} onMouseDown={e => e.preventDefault()} onClick={() => pick(r)}>
                    {r.kind === 'clear' ? <><Icon name="x" size={14} /><span>{clearLabel}</span></> : r.kind === 'create' ? <><Icon name="plus" size={14} /><span>{busy ? 'Creating…' : `${createLabel} “${q.trim()}”`}</span></> : <><span className="sel-lbl">{r.opt!.label}</span>{r.opt!.hint && <span className="sel-hint muted">{r.opt!.hint}</span>}</>}
                    {r.opt && r.opt.value === value && <span className="sel-check"><Icon name="check" size={14} /></span>}
                  </div>
                </div>
              )
            })}
          </div>
        </div>, document.body)}
    </div>
  )
}
