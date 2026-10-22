// Left column: add a book, sections, the books in progress, shelves, settings — the Pocket Calendar sidebar.
import { useCallback, useEffect, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Book, Dashboard } from '../lib/types'
import { hasProgress, progress } from '../lib/format'
import { Icon } from './icons'
import { Cover } from './Cover'
import { useOnChange, type ShellAction } from './Layout'
import { type Anchor, type MenuItem, Menu, rectOf } from './ui'

export function Sidebar({ onAction, onNavigate }: { onAction: (a: ShellAction) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const loc = useLocation()
  const [reading, setReading] = useState<Book[] | null>(null)
  const [dash, setDash] = useState<Dashboard | null>(null)
  const load = useCallback(() => {
    api.get<Book[]>(`${V1}/books?status=reading&sort=recent`).then(setReading).catch(() => {})
    api.get<Dashboard>(`${V1}/dashboard`).then(setDash).catch(() => {})
  }, [])
  // progress, a finished book or a new one changes the counts: refresh on every page change too
  useEffect(load, [load, loc.pathname])
  useOnChange(load)
  useEffect(() => {
    let t = 0
    const h = () => { clearTimeout(t); t = window.setTimeout(load, 400) }
    window.addEventListener('pb:mutated', h)
    return () => { clearTimeout(t); window.removeEventListener('pb:mutated', h) }
  }, [load])
  const title = settings?.title || status?.title || 'Pocket Bookshelf'
  const nav: { to: string; label: string; icon: string; count?: number }[] = [
    { to: '/now', label: 'Reading now', icon: 'book-open' },
    { to: '/library', label: 'Library', icon: 'library', count: dash?.books },
    { to: '/planner', label: 'Plan', icon: 'bookmark', count: dash?.want },
    { to: '/daily', label: 'Daily highlights', icon: 'sun' },
    { to: '/quotes', label: 'Quotes', icon: 'quote', count: dash?.quotes },
    { to: '/authors', label: 'Authors', icon: 'feather' },
    { to: '/series', label: 'Series', icon: 'layers' },
    { to: '/stats', label: 'Stats', icon: 'chart' },
  ]
  const shelves: { q: string; label: string; icon: string; count?: number }[] = [
    { q: 'status=want', label: 'Want to read', icon: 'bookmark', count: dash?.want },
    { q: 'status=reading', label: 'Reading', icon: 'book-open', count: dash?.reading },
    { q: 'status=read', label: 'Read', icon: 'check-circle', count: dash?.read_total },
    { q: 'status=abandoned', label: 'Abandoned', icon: 'stop' },
    { q: 'favorite=1', label: 'Favourites', icon: 'heart' },
  ]
  return (
    <aside className="sidebar" aria-label="Bookshelf">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Bookshelf' ? title : <>Pocket <b>Bookshelf</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^\d/.test(status.version) ? 'v' + status.version : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" onClick={() => onAction({ kind: 'add' })} title="Add a book (a)">
        <Icon name="plus" size={17} strokeWidth={2.2} />Add book<kbd className="kbd-on-primary">A</kbd>
      </button>

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive || (n.to === '/library' && loc.pathname.startsWith('/books/')) || (n.to === '/authors' && loc.pathname.startsWith('/author/')) ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}{!!n.count && <span className="sb-count">{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      <Group title="Reading" add={() => onAction({ kind: 'add' })}>
        {reading === null ? null : reading.length === 0
          ? <p className="sb-empty">Nothing on the go. <Link to="/planner" onClick={onNavigate}>Pick from your plan</Link></p>
          : <ul className="cal-list">{reading.map(b => <BookRow key={b.id} b={b} onAction={onAction} onNavigate={onNavigate} />)}</ul>}
      </Group>

      <Group title="Shelves" closed={(reading?.length || 0) > 4}>
        <ul className="cal-list">
          {shelves.map(s => {
            const active = loc.pathname === '/library' && new URLSearchParams(loc.search).toString() === s.q
            return (
              <li key={s.q} className="cal-row shelf-row">
                <Link to={`/library?${s.q}`} onClick={onNavigate} className={`cal-check ${active ? 'is-active' : ''}`}>
                  <span className="shelf-ico"><Icon name={s.icon} size={15} /></span>
                  <span className="cal-name ellipsis">{s.label}</span>
                  {!!s.count && <span className="sb-pct">{s.count}</span>}
                </Link>
              </li>
            )
          })}
        </ul>
      </Group>

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pb:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}

function Group({ title, add, closed, children }: { title: string; add?: () => void; closed?: boolean; children: React.ReactNode }) {
  const [open, setOpen] = useState(!closed)
  return (
    <section className="sb-sec">
      <h4 className="sb-h">
        <button type="button" className="sb-h-btn" aria-expanded={open} onClick={() => setOpen(o => !o)}><Icon name={open ? 'chevron-down' : 'chevron-right'} size={13} />{title}</button>
        {add && <button type="button" className="btn ghost icon-only xs" aria-label="Add a book" title="Add a book (a)" onClick={add}><Icon name="plus" size={14} /></button>}
      </h4>
      {open && children}
    </section>
  )
}

function BookRow({ b, onAction, onNavigate }: { b: Book; onAction: (a: ShellAction) => void; onNavigate: () => void }) {
  const navigate = useNavigate()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const pct = progress(b)
  const items: MenuItem[] = [
    { label: 'Open', icon: 'book', onSelect: () => { onNavigate(); navigate(`/books/${b.id}`) } },
    { label: 'Add a quote', icon: 'quote', onSelect: () => onAction({ kind: 'quote', book: b }) },
    'sep',
    { label: 'Finished…', icon: 'check-circle', onSelect: () => onAction({ kind: 'finish', book: b }) },
  ]
  return (
    <li className="cal-row book-row-sb">
      <NavLink to={`/books/${b.id}`} onClick={onNavigate} className={({ isActive }) => `cal-check ${isActive ? 'is-active' : ''}`} title={[b.title, b.authors].filter(Boolean).join(' · ')}>
        <Cover book={b} size="xxs" />
        <span className="cal-name ellipsis">{b.title}</span>
        {hasProgress(b) && <span className="sb-pct">{pct}%</span>}
      </NavLink>
      <button type="button" className="btn ghost icon-only xs cal-more" aria-label={`Options for ${b.title}`} aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(rectOf(e.currentTarget))}><Icon name="more" size={15} /></button>
      {menu && <Menu anchor={menu} items={items} onClose={() => setMenu(null)} width={200} />}
    </li>
  )
}
