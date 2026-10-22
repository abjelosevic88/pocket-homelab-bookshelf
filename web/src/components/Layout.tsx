// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import React, { createContext, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Book } from '../lib/types'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ErrorBoundary } from './ErrorBoundary'
import { AddBook } from './AddBook'
import { FinishModal } from './FinishModal'
import { QuoteModal } from '../pages/Book'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery, useToast } from './ui'
import { AskButton } from './AskButton'

const SIDEBAR_KEY = 'pb-sidebar'

/** Something was added or changed from the shell: pages and the sidebar reload. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('pb:changed'))
export function useOnChange(cb: () => void) {
  const ref = useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pb:changed', h)
    return () => window.removeEventListener('pb:changed', h)
  }, [])
}

/** The page tells the top bar what it shows (a book's title, an author's name). */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

/** Shell actions a sidebar row or the palette can start. */
export type ShellAction = { kind: 'add' } | { kind: 'finish'; book: Book } | { kind: 'quote'; book: Book }

export default function Layout() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { toast } = useToast()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumb] = useState('')
  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [action, setAction] = useState<ShellAction | null>(null)
  const act = (a: ShellAction) => { closeDrawer(); setAction(a) }
  const actRef = useRef(act)
  actRef.current = act

  useEffect(() => {
    const open = () => setShortcuts(true)
    window.addEventListener('pb:shortcuts', open)
    return () => window.removeEventListener('pb:shortcuts', open)
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t?.closest?.('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'a': e.preventDefault(); actRef.current({ kind: 'add' }); break
        case 'n': go('/now'); break
        case 'l': go('/library'); break
        case 'p': go('/planner'); break
        case 'd': go('/daily'); break
        case 'q': go('/quotes'); break
        case 'w': go('/authors'); break
        case 'e': go('/series'); break
        case 's': go('/stats'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'add', label: 'Add a book', icon: 'plus', hint: 'A', keywords: 'new search isbn', run: () => act({ kind: 'add' }) },
    { id: 'now', label: 'Reading now', icon: 'book-open', hint: 'N', keywords: 'home progress', run: () => navigate('/now') },
    { id: 'library', label: 'Library', icon: 'library', hint: 'L', keywords: 'shelf books', run: () => navigate('/library') },
    { id: 'plan', label: 'Plan', icon: 'bookmark', hint: 'P', keywords: 'want queue next planner', run: () => navigate('/planner') },
    { id: 'daily', label: 'Daily highlights', icon: 'sun', hint: 'D', keywords: 'review today', run: () => navigate('/daily') },
    { id: 'quotes', label: 'Quotes', icon: 'quote', hint: 'Q', keywords: 'highlights kindle', run: () => navigate('/quotes') },
    { id: 'authors', label: 'Authors', icon: 'feather', hint: 'W', keywords: 'writers', run: () => navigate('/authors') },
    { id: 'series', label: 'Series', icon: 'layers', hint: 'E', run: () => navigate('/series') },
    { id: 'stats', label: 'Stats', icon: 'chart', hint: 'S', keywords: 'year goal', run: () => navigate('/stats') },
    { id: 'favs', label: 'Favourites', icon: 'heart', keywords: 'loved', run: () => navigate('/library?favorite=1') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'kindle', label: 'Import Kindle highlights', icon: 'upload', keywords: 'clippings import', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onAction={act} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className="content"><div className="content-inner"><ErrorBoundary key={loc.pathname} where="page"><Outlet /></ErrorBoundary></div></main>
        </div>
        {mobile && <button type="button" className="fab" aria-label="Add a book" title="Add a book" onClick={() => act({ kind: 'add' })}><Icon name="plus" size={26} strokeWidth={2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {action?.kind === 'add' && <AddBook onClose={() => setAction(null)} onAdded={b => { announceChange(); navigate(`/books/${b.id}`) }} />}
        {action?.kind === 'finish' && <FinishModal book={action.book} onClose={() => setAction(null)} onDone={() => { setAction(null); announceChange() }} />}
        {action?.kind === 'quote' && <QuoteModal bookId={action.book.id} quote={{}} onClose={() => setAction(null)} onSaved={() => { setAction(null); toast(`Saved to “${action.book.title}”`); announceChange() }} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = {
  now: ['Reading now', '/now'], library: ['Library', '/library'], books: ['Library', '/library'], planner: ['Plan', '/planner'],
  daily: ['Daily highlights', '/daily'], quotes: ['Quotes', '/quotes'], authors: ['Authors', '/authors'], author: ['Authors', '/authors'],
  series: ['Series', '/series'], stats: ['Stats', '/stats'],
}

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const deep = loc.pathname.split('/').filter(Boolean).length > 1
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pb:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  let title: React.ReactNode = section
  if (deep && section) {
    title = <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb || '…'}</span></>
  }
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">{title}</h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
