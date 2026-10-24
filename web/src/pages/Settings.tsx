import { PocketModelSelect } from '../components/PocketModelSelect'
import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, upload, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Book, KosyncDocument, KosyncEvent, KosyncState, DailySettings, Settings as SettingsT, ThoughtTemplate, User } from '../lib/types'
import { Avatar, Confirm, CopyField, Field, MenuButton, type MenuItem, Loading, Modal, NeedsCore, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { KindleImport } from '../components/KindleImport'
import { MarkdownEditor } from '../components/MarkdownEditor'
import { LANGUAGES, langName } from '../lib/languages'
import { Select } from '../components/Select'
import { Cover } from '../components/Cover'

type TabId = 'general' | 'metadata' | 'templates' | 'daily' | 'ai' | 'reader' | 'audiobooks' | 'library' | 'profile' | 'users' | 'api' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'Your yearly goal and start page, and the app title.' },
  { id: 'metadata', label: 'Book search', icon: 'search', sub: 'Where book details and covers come from, and which languages come first.' },
  { id: 'templates', label: 'Templates', icon: 'text', sub: 'Starting points for “My thoughts” on a book.' },
  { id: 'daily', label: 'Daily highlights', icon: 'sun', sub: 'A few of your highlights every morning — and whether they are sent to your phone.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Pocket AI or OpenAI, for the characters of the novels on your shelf.' },
  { id: 'reader', label: 'Reader sync', icon: 'refresh', sub: 'Reading progress from KOReader, CrossPoint and other e-readers that speak KOSync.' },
  { id: 'audiobooks', label: 'Audiobooks', icon: 'book', sub: 'Audiobooks you finish in Pocket Audiobooks, as reads on your shelves.' },
  { id: 'library', label: 'Library', icon: 'book', sub: 'Ebooks you read in Pocket Library, as reads on your shelves, and your highlights as quotes.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'users', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box.' },
  { id: 'api', label: 'API tokens', icon: 'key', sub: 'Personal tokens for widgets, shortcuts and scripts.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Download the database, export and import your shelf, Kindle highlights and covers.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]
// other names the sections were or could be linked by
const ALIAS: Record<string, TabId> = { people: 'users', tokens: 'api', search: 'metadata', backup: 'data', import: 'data', kosync: 'reader', koreader: 'reader', crosspoint: 'reader' }

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const legacy = new URLSearchParams(window.location.search).get('tab') || ''
  const want = raw || legacy
  const tab: TabId = (TABS.find(t => t.id === (ALIAS[want] || want))?.id) || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  // old links (/settings?tab=daily) land on /settings/daily
  useEffect(() => { if (legacy && !raw) go(tab) }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  // phones: the section tabs scroll sideways — keep the open one in view
  useEffect(() => { document.querySelector('.set-tabs-mobile .tabs button.active')?.scrollIntoView({ block: 'nearest', inline: 'center' }) }, [tab, mobile])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (e.key === 'Escape' && !t.closest?.('input, textarea') && !document.querySelector('.modal-backdrop, .menu, .sel-pop, .slash-menu')) navigate('/')
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])

  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back to the bookshelf" title="Back to the bookshelf (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title">
              <h2>{current.label}</h2>
              <p>{current.sub}</p>
            </div>
            {tab === 'general' && <General />}
            {tab === 'metadata' && <Metadata />}
            {tab === 'templates' && <Templates />}
            {tab === 'daily' && <DailyTab />}
            {tab === 'ai' && <AITab />}
            {tab === 'reader' && <ReaderSync />}
            {tab === 'audiobooks' && <Audiobooks />}
            {tab === 'library' && <LibrarySync />}
            {tab === 'profile' && <Profile />}
            {tab === 'users' && <Users />}
            {tab === 'api' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

/** A settings card: title, explanation, actions on the right — Pocket Calendar's section. */
function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  // Settings may arrive after the tab mounted (direct load of /settings): adopt them once.
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async () => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, f); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Reading" sub="These are yours: everyone in the household has their own goal, start page, languages, templates and keys.">
        <SettingRow title="Yearly reading goal" help="Books per year. 0 = no goal. Shown on Now, Plan and Stats.">
          <input type="number" min={0} max={1000} value={f.yearly_goal} onChange={e => setF({ ...f, yearly_goal: Math.max(0, Number(e.target.value) || 0) })} style={{ width: 100 }} aria-label="Yearly reading goal" />
        </SettingRow>
        <SettingRow title="Open on" help="The page the app starts on.">
          <Segmented size="sm" value={f.landing} onChange={v => setF({ ...f, landing: v })} options={[{ value: 'now', label: 'Now' }, { value: 'library', label: 'Library' }, { value: 'planner', label: 'Plan' }]} label="Open on" />
        </SettingRow>
      </Section>
      <Section title="This box" sub={isAdmin ? 'This applies to everyone on this box.' : 'Set by an admin for everyone.'}>
        <SettingRow title="App title" help="Shown in the sidebar and the browser tab.">
          <input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} style={{ width: 220 }} aria-label="App title" />
        </SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={save} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Metadata() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const [testing, setTesting] = useState(false)
  if (!f) return <Loading />
  const test = async () => {
    setTesting(true)
    try {
      const r = await api.get<{ results: unknown[]; warnings: string[] | null }>(`${V1}/search?q=9780441478125&source=google`)
      toast(r.results.length ? 'Google Books works' : 'Google Books answered, but found nothing')
    } catch (e) {
      const m = (e as Error).message
      if (/HTTP 50[234]/.test(m)) toast('Your key is accepted, but Google Books is having temporary trouble (HTTP 503). New keys often do for a while. Searches use Open Library meanwhile; try again later.', 'err')
      else if (/HTTP 400|API key not valid|keyInvalid/i.test(m)) toast('Google rejected the key. Check it was copied completely.', 'err')
      else if (/HTTP 403/.test(m)) toast('Google refused the key: enable the “Books API” for its project, and remove website/IP restrictions from the key.', 'err')
      else toast(m, 'err')
    } finally { setTesting(false) }
  }
  return (
    <div className="stack loose">
      <Section title="Book search" sub="When you add a book, its details and cover come from free public catalogues. Covers are downloaded and kept with your data, so nothing is loaded from outside when you browse.">
        <SettingRow title="Search in" help="The default for the add-book search; you can switch per search.">
          <Segmented size="sm" value={f.search_source} onChange={v => setF({ ...f, search_source: v })} options={[{ value: 'all', label: 'All' }, { value: 'openlibrary', label: 'Open Library' }, { value: 'google', label: 'Google Books' }]} label="Search in" />
        </SettingRow>
        <SettingRow stack title="Preferred languages" help="Search results and authors' bibliographies show books available in these languages first, as that edition (title and cover). The first one wins when a book exists in several.">
          <LanguagePicker value={f.languages} onChange={v => setF({ ...f, languages: v })} />
        </SettingRow>
        <SettingRow stack title="Google Books API key" help={<>Free: Google Cloud console → enable “Books API” → Credentials → API key. Without a key Google refuses anonymous searches. {f.google_books_env && !f.google_books_key && 'A key is set through GOOGLE_BOOKS_API_KEY.'}</>}>
          <input value={f.google_books_key} onChange={e => setF({ ...f, google_books_key: e.target.value.trim() })} placeholder={f.google_books_env ? 'using GOOGLE_BOOKS_API_KEY' : 'AIza…'} autoComplete="off" spellCheck={false} aria-label="Google Books API key" />
        </SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={save} disabled={busy}>Save changes</button><button type="button" className="btn" onClick={test} disabled={testing || !(f.google_books_key || f.google_books_env)}>{testing ? 'Testing…' : 'Test Google Books'}</button></div>
      <Section title="Sources" sub="With “All”, both are searched at once and the same book (same ISBN) is shown once. ISBN searches pick that exact edition.">
        <dl className="set-dl">
          <div><dt>Open Library <span className="badge cat">always on</span></dt><dd className="muted">openlibrary.org — no key, community catalogue, good for older and non-English books.</dd></div>
          <div><dt>Google Books {f.sources.includes('google') ? <span className="badge cat">on</span> : <span className="badge">needs key</span>}</dt><dd className="muted">Broad coverage of recent editions, page counts and descriptions.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and keeps their own shelf: books, ratings, thoughts, quotes, plan, goal, settings and keys are private to each person — admins included. Admins manage the accounts and the app title.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge cat">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their whole shelf: books, thoughts, quotes and reading history." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f)
      toast(user.id ? 'User saved' : 'User added'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters. Tell them to change it under Settings → Profile."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one.">
        <div className="stack">
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </div>
      </Section>}
      <div className="row"><button type="submit" className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

interface AudiobooksState { core: boolean; enabled: boolean; syncing: boolean; last?: { at: string; added: number; finished: number; started: number; error?: string; off?: boolean } }

// Audiobooks: what is finished in Pocket Audiobooks (web player or any
// Audiobookshelf app) becomes a read here, through Pocket Core.
function Audiobooks() {
  const { toast } = useToast()
  const { data, reload, setData } = useAsync(() => api.get<AudiobooksState>(`${V1}/audiobooks`), [])
  const [busy, setBusy] = useState(false)
  if (!data) return <Loading />
  const set = async (enabled: boolean) => {
    try { setData(await api.put<AudiobooksState>(`${V1}/audiobooks`, { enabled })) } catch (e) { toast((e as Error).message, 'err') }
  }
  const sync = async () => {
    setBusy(true)
    try {
      const r = await api.post<NonNullable<AudiobooksState['last']>>(`${V1}/audiobooks/sync`)
      toast(r.error ? r.error : `Done: ${r.finished} finished, ${r.started} started, ${r.added} added to your shelves`, r.error ? 'err' : 'ok')
      reload()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const l = data.last
  return (
    <div className="stack loose">
      <Section title="From Pocket Audiobooks" icon="book" sub="Every audiobook you finish — in the Pocket Audiobooks player or any Audiobookshelf app — becomes a read here, on the day you finished it. One you have started (half a minute of listening is enough) goes on “Reading”. It is matched to a book already on your shelves by title and author, or added as an audio book with its cover. A book you remove is not added again.">
        {!data.core && <NeedsCore what="Needs Pocket Core: audiobooks come from Pocket Audiobooks through it." />}
        <SettingRow title="Take in my audiobooks" help="Every ten minutes, as you (through the Pocket Core link “Pocket Bookshelf → Pocket Audiobooks”).">
          <Switch checked={data.core && data.enabled} disabled={!data.core} onChange={set} label="Take in audiobooks" />
        </SettingRow>
        {data.core && l && (
          <p className={`small ${l.error && !l.off ? 'danger-text' : 'muted'}`}>
            {l.error ? l.error : `Last looked ${new Date(l.at).toLocaleString()}: ${l.finished} finished, ${l.started} started, ${l.added} added.`}
          </p>
        )}
        {data.core && <div className="row"><button type="button" className="btn sm" onClick={sync} disabled={busy || !data.enabled}><Icon name="refresh" size={14} />{busy ? 'Looking…' : 'Look now'}</button></div>}
      </Section>
    </div>
  )
}

interface LibraryState { core: boolean; enabled: boolean; syncing: boolean; last?: { at: string; added: number; finished: number; started: number; quotes: number; error?: string; off?: boolean } }

// Library: what is read in Pocket Library (its web reader, KOReader or a
// CrossPoint) becomes reads here, and highlights become quotes, through Pocket Core.
function LibrarySync() {
  const { toast } = useToast()
  const { data, reload, setData } = useAsync(() => api.get<LibraryState>(`${V1}/library`), [])
  const [busy, setBusy] = useState(false)
  if (!data) return <Loading />
  const set = async (enabled: boolean) => {
    try { setData(await api.put<LibraryState>(`${V1}/library`, { enabled })) } catch (e) { toast((e as Error).message, 'err') }
  }
  const sync = async () => {
    setBusy(true)
    try {
      const r = await api.post<NonNullable<LibraryState['last']>>(`${V1}/library/sync`)
      toast(r.error ? r.error : `Done: ${r.finished} finished, ${r.started} started, ${r.added} added, ${r.quotes} quotes`, r.error ? 'err' : 'ok')
      reload()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const l = data.last
  return (
    <div className="stack loose">
      <Section title="From Pocket Library" icon="book" sub="Every ebook you finish in Pocket Library — in its reader, in KOReader or on a CrossPoint — becomes a read here, on the day you finished it. One you are reading goes on “Reading” with its page, and your highlights and notes there become quotes. A book is matched by ISBN, else by title and author, or added as an ebook with its cover. A book you remove is not added again.">
        {!data.core && <NeedsCore what="Needs Pocket Core: ebooks come from Pocket Library through it." />}
        <SettingRow title="Take in my reading" help="Every ten minutes, as you (through the Pocket Core link “Pocket Bookshelf → Pocket Library”).">
          <Switch checked={data.core && data.enabled} disabled={!data.core} onChange={set} label="Take in reading from Pocket Library" />
        </SettingRow>
        {data.core && l && (
          <p className={`small ${l.error && !l.off ? 'danger-text' : 'muted'}`}>
            {l.error ? l.error : `Last looked ${new Date(l.at).toLocaleString()}: ${l.finished} finished, ${l.started} started, ${l.added} added, ${l.quotes} quotes.`}
          </p>
        )}
        {data.core && <div className="row"><button type="button" className="btn sm" onClick={sync} disabled={busy || !data.enabled}><Icon name="refresh" size={14} />{busy ? 'Looking…' : 'Look now'}</button></div>}
      </Section>
    </div>
  )
}

function ReaderSync() {
  const { toast } = useToast()
  const { user } = useApp()
  const { data, setData, reload } = useAsync(() => api.get<KosyncState>(`${V1}/kosync`), [])
  const books = useAsync(() => api.get<Book[]>(`${V1}/books?sort=title`), [])
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [off, setOff] = useState(false)
  const [linking, setLinking] = useState<string | null>(null)
  const [history, setHistory] = useState<KosyncDocument | null>(null)
  const [remote, setRemote] = useState({ url: 'https://sync.crosspointreader.com', user: '', password: '' })
  const [pulling, setPulling] = useState(false)
  const [editing, setEditing] = useState(false)
  useEffect(() => { if (data) setUsername(data.account?.username || (user?.email || '').split('@')[0].replace(/[^A-Za-z0-9._@+-]/g, '')) }, [data?.account?.username]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!data) return <Loading />
  const acc = data.account
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setData(await api.put<KosyncState>(`${V1}/kosync/account`, { username, password })); setPassword(''); setEditing(false); toast(acc ? 'Saved — log in again on your devices if you changed anything' : 'Reader sync is on') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const link = async (d: KosyncDocument, book_id: number | null, ignored = false) => {
    try { await api.put(`${V1}/kosync/documents/${d.document}`, { book_id, ignored }); setLinking(null); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const savePrefs = async (body: Record<string, unknown>, ok: string) => {
    setBusy(true)
    try { setData(await api.put<KosyncState>(`${V1}/kosync/prefs`, body)); setRemote(r => ({ ...r, password: '' })); books.reload(); toast(ok) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const pull = async () => {
    setPulling(true)
    try { const r = await api.post<{ documents: number; updated: number; linked: number; quotes: number }>(`${V1}/kosync/pull`); toast(`${r.documents} books checked · ${r.updated} updated · ${r.linked} linked · ${r.quotes} highlights`); reload(); books.reload() } catch (e) { toast((e as Error).message, 'err'); reload() } finally { setPulling(false) }
  }
  const forget = async (d: KosyncDocument) => { try { await api.del(`${V1}/kosync/documents/${d.document}`); reload() } catch (e) { toast((e as Error).message, 'err') } }
  const name = (d: KosyncDocument) => d.title ? `${d.title}${d.authors ? ` — ${d.authors}` : ''}` : d.filename ? d.filename.split('/').pop() : 'Unknown book'
  const when = (ts: number) => new Date(ts * 1000).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
  const options = (books.data || []).map(b => ({ value: String(b.id), label: b.authors ? `${b.title} — ${b.authors}` : b.title }))
  const doc = (d: KosyncDocument) => {
    const book = d.book_id ? (books.data || []).find(b => b.id === d.book_id) : undefined
    const pct = Math.round(d.percentage * 100)
    const items: MenuItem[] = [
      { label: 'History', icon: 'clock', hint: 'every update as it arrived', onSelect: () => setHistory(d) },
      d.book_id ? { label: 'Unlink from the book', icon: 'x', onSelect: () => link(d, null) }
        : d.ignored ? { label: 'Stop ignoring', icon: 'eye', onSelect: () => link(d, null, false) }
        : { label: 'Ignore this file', icon: 'eye', onSelect: () => link(d, null, true) },
      'sep',
      { label: 'Forget', icon: 'trash', danger: true, hint: 'the device sends it again on its next sync', onSelect: () => forget(d) },
    ]
    return (
      <li key={d.document} className="kosync-doc">
        <Cover book={book || { id: 0, title: d.title || '?', authors: d.authors, cover: '' }} size="xs" />
        <div className="info">
          <div className="nm ellipsis">{book ? <Link to={`/books/${book.id}`}>{book.title}</Link> : d.title || name(d)}</div>
          <div className="muted small ellipsis">{book ? book.authors : d.title ? d.authors : <>No title from the device yet · file <span className="mono">{d.document.slice(0, 12)}…</span></>}</div>
          <div className="kosync-progress">
            <div className="bar"><span className="fill" style={{ width: `${pct}%` }} /></div>
            <span className="muted small">{pct}% · {d.device || 'device'} · {when(d.timestamp)}</span>
          </div>
          {linking === d.document && <div className="kosync-link"><Select value={d.book_id ? String(d.book_id) : ''} onChange={v => v && link(d, Number(v))} options={options} placeholder="Find the book on your shelf…" ariaLabel="Book" /></div>}
        </div>
        <div className="actions">
          {d.ignored ? <span className="badge">Ignored</span> : !d.book_id && <span className="badge warn">Not linked</span>}
          <button type="button" className="btn sm" onClick={() => setLinking(linking === d.document ? null : d.document)}>{linking === d.document ? 'Cancel' : d.book_id ? 'Change' : 'Link'}</button>
          <MenuButton items={items} label={`More for ${name(d)}`} align="right" width={250}><Icon name="more" size={16} /></MenuButton>
        </div>
      </li>
    )
  }
  return (
    <div className="stack loose">
      <Section title="Your sync account" icon="key" sub="E-readers log in with a username and password of their own, not your Bookshelf login. Pick them here, then enter the same on each device."
        actions={acc && <button type="button" className="btn ghost sm danger-text" onClick={() => setOff(true)}>Turn off</button>}>
        {acc && <>
          <SettingRow title="Status" help={<>Last device sync: {acc.last_seen ? when(Date.parse(acc.last_seen) / 1000) : 'never'}</>}><span className="badge ok">On</span></SettingRow>
          <SettingRow stack title="Server address" help="Enter it as the custom sync server on each device."><CopyField value={data.url} label="Sync server" /></SettingRow>
          <SettingRow stack title="Username"><CopyField value={acc.username} label="Sync username" /></SettingRow>
        </>}
        {acc && !editing
          ? <div className="kosync-foot"><button type="button" className="btn sm" onClick={() => setEditing(true)}><Icon name="edit" size={14} />Change username or password</button></div>
          : (
            <form onSubmit={save} className="kosync-form">
              <Field label="Username"><input value={username} onChange={e => setUsername(e.target.value.trim())} autoComplete="off" spellCheck={false} required /></Field>
              <Field label={acc ? 'New password' : 'Password'} help={acc ? 'Leave empty to keep the current one.' : 'At least 8 characters. Devices only send a hash of it.'}><input type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" minLength={acc ? undefined : 8} required={!acc} /></Field>
              <div className="kosync-form-actions">
                <button type="submit" className="btn primary" disabled={busy}>{acc ? 'Save' : 'Turn on reader sync'}</button>
                {acc && <button type="button" className="btn ghost" onClick={() => { setEditing(false); setPassword(''); setUsername(acc.username) }}>Cancel</button>}
              </div>
            </form>
          )}
      </Section>

      {acc && <Section title="Connect a device" icon="link" sub="Use the server address, username and password from above.">
        <div className="kosync-devices">
          <div className="kosync-device">
            <h4>KOReader</h4>
            <ol>
              <li>Open a book → Tools → <b>Progress sync</b>.</li>
              <li><b>Custom sync server</b> → the server address.</li>
              <li>Register / login → <b>Login</b>. Registering is closed.</li>
            </ol>
          </div>
          <div className="kosync-device">
            <h4>CrossPoint</h4>
            <ol>
              <li>Settings → <b>KOReader Sync</b>.</li>
              <li>Server address, username and password.</li>
              <li>Turn on <b>send metadata</b> so books match by title.</li>
            </ol>
          </div>
          <div className="kosync-device">
            <h4>CrossPoint Sync</h4>
            <ol>
              <li>Connections → <b>Another KOSync server</b>.</li>
              <li>Server address, username, password → <b>Connect</b>.</li>
              <li>It sends no titles: connect it below as well.</li>
            </ol>
          </div>
        </div>
      </Section>}

      <Section title="CrossPoint Sync" icon="refresh" sub="Its mirror sends only a file hash. With your CrossPoint Sync account Bookshelf reads each book's title, author, cover and page count, your highlights (as quotes), and the pages you read each day (Stats). Marking a book read or stopped here tells CrossPoint Sync too. It checks right after an unknown book shows up, and every 15 minutes."
        actions={data.prefs.remote_url && <button type="button" className="btn sm" onClick={pull} disabled={pulling}><Icon name="refresh" size={14} />{pulling ? 'Syncing…' : 'Sync now'}</button>}>
        {data.prefs.remote_url ? <>
          <SettingRow title={<>Connected as <b>{data.prefs.remote_user}</b></>} help={data.prefs.remote_url}><button type="button" className="btn ghost sm danger-text" disabled={busy} onClick={() => savePrefs({ remote_url: '', remote_user: '' }, 'Disconnected')}>Disconnect</button></SettingRow>
          <SettingRow title="Last checked" help={data.prefs.last_error && <span className="danger-text">{data.prefs.last_error}</span>}><span className="muted small">{data.prefs.last_pull ? when(Date.parse(data.prefs.last_pull) / 1000) : 'never'}</span></SettingRow>
        </> : (
          <form className="kosync-form" onSubmit={e => { e.preventDefault(); savePrefs({ remote_url: remote.url, remote_user: remote.user, remote_password: remote.password }, 'Connected to CrossPoint Sync') }}>
            <Field label="Server" className="wide"><input value={remote.url} onChange={e => setRemote({ ...remote, url: e.target.value.trim() })} spellCheck={false} required /></Field>
            <Field label="Username"><input value={remote.user} onChange={e => setRemote({ ...remote, user: e.target.value.trim() })} autoComplete="off" spellCheck={false} required /></Field>
            <Field label="Password" help="Only its hash is stored, the same thing devices send."><input type="password" value={remote.password} onChange={e => setRemote({ ...remote, password: e.target.value })} autoComplete="off" required /></Field>
            <div className="kosync-form-actions"><button type="submit" className="btn primary" disabled={busy}>Connect</button></div>
          </form>
        )}
      </Section>

      <Section title="Books from your e-readers" icon="book" sub="Each file a device reads. Progress on a linked book moves its page and moves it from Want to read to Reading.">
        <SettingRow title="Add books that are not on my shelf" help="Looked up in the catalogues and added as Reading, with a cover. Off: they wait here to be linked."><Switch checked={data.prefs.add_missing} onChange={v => savePrefs({ add_missing: v }, v ? 'Missing books will be added' : 'Missing books wait here to be linked')} label="Add books that are not on my shelf" /></SettingRow>
        {data.documents.length === 0
          ? <p className="muted kosync-empty">Nothing synced yet. Books show up here after a device sends its first progress.</p>
          : <ul className="kosync-docs">{data.documents.map(doc)}</ul>}
      </Section>
      {history && <SyncHistory doc={history} title={name(history) || ''} onClose={() => setHistory(null)} />}
      {off && <Confirm confirmLabel="Turn off" title="Turn reader sync off?" message="Devices can no longer log in or send progress. The books listed here and their links stay." onCancel={() => setOff(false)} onConfirm={async () => { setOff(false); try { await api.del(`${V1}/kosync/account`); setPassword(''); setEditing(false); reload() } catch (e) { toast((e as Error).message, 'err') } }} />}
    </div>
  )
}

function SyncHistory({ doc, title, onClose }: { doc: KosyncDocument; title: string; onClose: () => void }) {
  const { data, error } = useAsync(() => api.get<KosyncEvent[]>(`${V1}/kosync/documents/${doc.document}/events`), [doc.document])
  return (
    <Modal title={title} onClose={onClose} size="lg">
      <p className="muted small mb">Every progress update for this file, exactly as it arrived. Document <code>{doc.document}</code></p>
      {error ? <p className="danger-text">{String(error)}</p> : !data ? <Loading /> : data.length === 0 ? <p className="muted">No updates kept yet.</p> : (
        <div className="stack tight">
          {data.map(e => (
            <details key={e.id}>
              <summary><b>{Math.round(e.percentage * 100)}%</b> · {e.source === 'crosspoint' ? 'from CrossPoint Sync' : e.device || 'device'} · <span className="muted">{new Date(e.timestamp * 1000).toLocaleString()}</span></summary>
              <pre className="mono small" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{JSON.stringify(e.body, null, 2)}</pre>
            </details>
          ))}
        </div>
      )}
    </Modal>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you and sees your shelf. Good for Homepage widgets (<code>GET /api/v1/dashboard</code>), logging pages from an iOS Shortcut (<code>POST /api/v1/books/{'{id}'}/progress</code>) or a random quote somewhere (<code>GET /api/v1/quotes?random=1</code>). See <code>docs/api.md</code>.</>}>
      <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" aria-label="Token name" /><button type="submit" className="btn primary"><Icon name="plus" size={15} />Create</button></form>
      {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
      {data && data.length > 0 ? (
        <table className="table compact">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
          <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? t.last_used_at.slice(0, 16).replace('T', ' ') : 'never'}</td><td className="muted">{t.created_at.slice(0, 10)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
        </table>
      ) : <p className="muted">No tokens yet.</p>}
      {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </Section>
  )
}

function Data() {
  const { toast } = useToast()
  const { user } = useApp()
  const [busy, setBusy] = useState(false)
  const [covers, setCovers] = useState<{ running: boolean; busy?: boolean; done: number; found: number; total: number } | null>(null)
  const [kindle, setKindle] = useState(false)
  const importJSON = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<{ created: number; skipped: number }>(`${V1}/import`, JSON.parse(await file.text())); toast(`Imported ${r.created} books, ${r.skipped} skipped`); pollCovers() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const importCSV = async (file: File) => {
    setBusy(true)
    try { const r = await upload<{ created: number; skipped: number }>(`${V1}/import/goodreads`, 'file', file); toast(`Imported ${r.created} books, ${r.skipped} already there or invalid`); pollCovers() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const pollCovers = async (start = false) => {
    const st = start ? await api.post<typeof covers>(`${V1}/covers/fetch-missing`) : await api.get<typeof covers>(`${V1}/covers/fetch-missing`)
    setCovers(st)
    if (st?.running) setTimeout(() => pollCovers(), 3000)
  }
  return (
    <div className="stack loose">
      {user?.role === 'admin' && <Section title="Backup" icon="database" sub={<>A consistent copy of the SQLite database (<code>VACUUM INTO</code>) — every account's shelf, so only admins can take it. Covers live next to it in <code>covers/</code>. Restore by stopping the app and replacing <code>pocket-bookshelf.db</code> in the data directory.</>}
        actions={<a className="btn primary sm" href={`${V1}/backup`}><Icon name="download" size={14} />Download database</a>} />}
      <Section title="Export" icon="download" sub="JSON is complete (books, thoughts, reading history, quotes) and can be imported again. CSV is one row per book for spreadsheets.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Export CSV</a></div>
      </Section>
      <Section title="Import" icon="upload" sub="Goodreads: My Books → Import and export → Export library. StoryGraph: Manage account → Export. Ratings, reviews, read dates and shelves come along; books already on your shelf are skipped. Covers are looked up afterwards by ISBN.">
        <div className="row wrap">
          <label className="btn">{busy ? 'Importing…' : <><Icon name="upload" size={15} />Import JSON…</>}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
          <label className="btn">{busy ? 'Importing…' : <><Icon name="upload" size={15} />Goodreads / StoryGraph CSV…</>}<input type="file" accept=".csv,text/csv" hidden onChange={e => e.target.files?.[0] && importCSV(e.target.files[0])} /></label>
        </div>
      </Section>
      <Section title="Kindle highlights" icon="quote" sub="Every highlight from a Kindle's My Clippings.txt, matched to the books on your shelf." actions={<button type="button" className="btn sm" onClick={() => setKindle(true)}><Icon name="upload" size={14} />Import My Clippings.txt…</button>}>
        {kindle && <KindleImport onClose={() => setKindle(false)} />}
      </Section>
      <Section title="Missing covers" icon="image" sub="Checks Open Library by ISBN for books without a cover, one every few seconds. What it does not find is asked of Pocket AI (Settings → AI): it names the edition's ISBNs or the original title, and only covers that really download are saved."
        actions={<button type="button" className="btn sm" onClick={() => pollCovers(true)} disabled={covers?.running}><Icon name="refresh" size={14} />Find missing covers</button>}>
        {covers && <p className="muted small" style={{ margin: 0 }}>{covers.running ? `Looking up… ${covers.done}/${covers.total}` : covers.busy ? 'Another cover lookup is running; try again in a few minutes' : `Done: ${covers.found} of ${covers.total} found`}</p>}
      </Section>
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const tiles: [string, number | undefined, string][] = [['Books', counts.books, 'book'], ['Reads', counts.reads, 'check-circle'], ['Quotes', counts.quotes, 'quote'], ['Accounts', counts.users, 'users']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</span>],
    ['Covers', `${String(data.covers)} files · ${(Number(data.cover_bytes) / 1024 / 1024).toFixed(1)} MB`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Bookshelf</h3>
          <p className="muted small">A small reading log: what you read, what you thought of it, the passages worth keeping, and a plan for what to read next. Book details and covers come from Open Library and Google Books. One binary, one SQLite file, one volume — part of the Pocket apps.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.filter(([, n]) => n !== undefined).map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{n ?? 0}</span><span className="set-stat-l">{label === 'Accounts' ? 'Accounts on this box' : `On your shelf · ${label.toLowerCase()}`}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => window.dispatchEvent(new CustomEvent('pb:shortcuts'))}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-bookshelf" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-bookshelf/releases" target="_blank" rel="noreferrer"><Icon name="file" size={15} />Release notes</a>
      </div>
    </div>
  )
}

/** Ordered list of preferred languages with add / remove / move up. */
function LanguagePicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const list = value.split(',').map(s => s.trim()).filter(Boolean)
  const set = (l: string[]) => onChange(l.join(','))
  const up = (i: number) => { const l = [...list]; [l[i - 1], l[i]] = [l[i], l[i - 1]]; set(l) }
  return (
    <div className="lang-picker">
      {list.length === 0 && <span className="muted small">None: results in catalogue order.</span>}
      {list.map((c, i) => (
        <span key={c} className="chip active lang-chip">
          <b>{i + 1}.</b> {langName(c)}
          {i > 0 && <button type="button" onClick={() => up(i)} title="Prefer this one more" aria-label={`Move ${langName(c)} up`}><Icon name="arrow-up" size={13} /></button>}
          <button type="button" onClick={() => set(list.filter(x => x !== c))} title="Remove" aria-label={`Remove ${langName(c)}`}><Icon name="x" size={13} /></button>
        </span>
      ))}
      {list.length < 3 && (
        <Select className="lang-add" value="" onChange={v => v && set([...list, v])} ariaLabel="Add a language" placeholder="+ Add language…"
          options={Object.entries(LANGUAGES).filter(([c]) => !list.includes(c)).map(([c, n]) => ({ value: c, label: n }))} />
      )}
    </div>
  )
}

/** Templates for "My thoughts": named sets of section headings. */
function Templates() {
  const { f, setF, save, busy } = useSettingsForm()
  if (!f) return <Loading />
  const list = f.thought_templates || []
  const set = (l: ThoughtTemplate[]) => setF({ ...f, thought_templates: l })
  const upd = (i: number, t: Partial<ThoughtTemplate>) => set(list.map((x, j) => j === i ? { ...x, ...t } : x))
  return (
    <div className="stack loose">
      <Section title="Thoughts templates" actions={<button type="button" className="btn sm" onClick={() => set([...list, { name: 'New template', body: '## 🧠 My thoughts\n\n## 🥰 Who would enjoy it?\n' }])}><Icon name="plus" size={14} />Add template</button>}
        sub={<>When a book has no thoughts yet, its page offers these as a starting point. Write one heading per line: <code>## Heading</code> for a section, <code>### Sub-heading</code> under it; anything under a heading (like <code>1.</code> <code>2.</code> <code>3.</code>) is kept as a prompt. Sections you leave empty are hidden when the thoughts are shown.</>}>
        {list.length === 0 && <p className="muted">No templates. Add one above.</p>}
        <div className="grid" style={{ gap: 14 }}>
          {list.map((t, i) => (
            <div key={i} className="template-edit">
              <div className="row">
                <input value={t.name} onChange={e => upd(i, { name: e.target.value })} placeholder="Name (e.g. Fiction)" style={{ maxWidth: 280 }} aria-label="Template name" />
                <span className="spacer" />
                {i > 0 && <button type="button" className="btn ghost sm icon-only" onClick={() => { const l = [...list]; [l[i - 1], l[i]] = [l[i], l[i - 1]]; set(l) }} title="Move up" aria-label="Move up"><Icon name="arrow-up" size={15} /></button>}
                <button type="button" className="btn ghost sm danger-text" onClick={() => set(list.filter((_, j) => j !== i))}><Icon name="trash" size={14} />Delete</button>
              </div>
              <MarkdownEditor rows={Math.min(16, Math.max(6, t.body.split('\n').length + 1))} value={t.body} onChange={v => upd(i, { body: v })} />
            </div>
          ))}
        </div>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={save} disabled={busy}>Save templates</button></div>
    </div>
  )
}

interface AISettings { ai_provider: string; ai_pocket_model: string; ai_configured: boolean; ai_status: { selected: string; detail: string; ready: boolean }; core?: { configured: boolean; registered: boolean }; key_set: boolean; key_hint: string; key_env: boolean; model: string; default_model: string; web_search: boolean }
interface CharacterStatus { writer: 'app' | 'external'; running: boolean; paused: boolean; waiting: number; written: number; failed: number; last_error: string; last_run?: string }

/** AI: your own OpenAI key lets the app write the characters of your books itself. */
function AITab() {
  const { toast } = useToast()
  const { data, setData } = useAsync(() => api.get<AISettings>(`${V1}/settings/ai`), [])
  const status = useAsync(() => api.get<CharacterStatus>(`${V1}/characters/status`), [])
  const [key, setKey] = useState('')
  const [provider, setProvider] = useState('auto')
  const [pocketModel, setPocketModel] = useState('')
  const [model, setModel] = useState('')
  const [web, setWeb] = useState(true)
  const [models, setModels] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  // Pocket AI only works through Pocket Core: without it a saved "pocket" shows as Automatic (own key)
  const coreOn = !!data?.core?.configured && !!data?.core?.registered
  useEffect(() => { if (data) { setModel(data.model); setWeb(data.web_search); setProvider(data.ai_provider === 'pocket' && !(data.core?.configured && data.core?.registered) ? 'auto' : data.ai_provider || 'auto'); setPocketModel(data.ai_pocket_model || '') } }, [data])
  // the model list comes from the account behind the saved key
  useEffect(() => { if (provider === 'openai' && (data?.key_set || data?.key_env)) api.post<{ models: string[] }>(`${V1}/settings/ai/test`, {}).then(r => setModels(r.models)).catch(() => {}) }, [provider, data?.key_set, data?.key_env])
  // follow a run while it is going
  useEffect(() => {
    if (!status.data?.running) return
    const t = setInterval(() => status.reload(), 4000)
    return () => clearInterval(t)
  }, [status.data?.running]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!data) return <Loading />
  const hasKey = data.key_set || data.key_env
  const test = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ models: string[] }>(`${V1}/settings/ai/test`, { key })
      setModels(r.models)
      if (!model) setModel(r.models.includes(data.default_model) ? data.default_model : r.models.find(m => m.startsWith('gpt-5')) || r.models[0] || '')
      toast(`The key works — ${r.models.length} models available`)
    } catch (e) { toast((e as Error).message.replace(/^HTTP \d+:\s*/, ''), 'err') } finally { setBusy(false) }
  }
  const save = async (extra: Record<string, unknown> = {}) => {
    setBusy(true)
    try {
      const r = await api.put<AISettings>(`${V1}/settings/ai`, { key: key || undefined, model, web_search: web, ai_provider: provider, ai_pocket_model: pocketModel, ...extra })
      setData(r); setKey(''); toast(extra.clear_key ? 'Key removed' : 'Saved'); setTimeout(status.reload, 1500)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const run = async () => { try { await api.post(`${V1}/characters/run`, {}); toast('Writing what is missing…'); setTimeout(status.reload, 1500) } catch (e) { toast((e as Error).message, 'err') } }
  const st = status.data
  return (
    <div className="stack loose">
      <Section title="AI provider" icon="sparkles" sub="Writes short, spoiler-free character notes in your preferred languages after you add a book. Automatic prefers Pocket AI when connected through Core; standalone installs use your own OpenAI key.">
        <SettingRow title="Text generation" help="Pocket AI uses your connected Claude account.">
          <Select ariaLabel="Text generation provider" value={provider} onChange={setProvider} options={[{ value: 'auto', label: 'Automatic — prefer Pocket AI' }, { value: 'pocket', label: 'Pocket AI', hint: coreOn ? undefined : 'needs Pocket Core', disabled: !coreOn }, { value: 'openai', label: 'OpenAI' }]} />
        </SettingRow>
        {!coreOn && <NeedsCore what={data.ai_provider === 'pocket' ? 'Pocket AI was picked, but it needs Pocket Core, which this server is not connected to.' : 'Pocket AI needs Pocket Core.'} />}
        <p className="muted small" role="status">Saved selection: {data.ai_status?.detail}</p>
        {provider !== 'openai' && coreOn && <PocketModelSelect value={pocketModel} onChange={setPocketModel} />}
        {provider === 'openai' && <>
          <SettingRow stack title="API key" help={<>From <a href="https://platform.openai.com/api-keys" target="_blank" rel="noreferrer">platform.openai.com/api-keys</a>. Stored in the app's database, for your account only. {data.key_env && !data.key_set && 'A key is set through OPENAI_API_KEY.'}</>}>
            <input type="password" value={key} onChange={e => setKey(e.target.value.trim())} placeholder={data.key_set ? `saved (${data.key_hint}) — type to replace` : data.key_env ? 'using OPENAI_API_KEY' : 'sk-…'} autoComplete="off" spellCheck={false} aria-label="OpenAI API key" />
          </SettingRow>
          <SettingRow title="Model" help={`Bigger models know more books; smaller ones are cheaper. Default: ${data.default_model}.`}>
            {models.length > 0
              ? <Select value={model} onChange={setModel} options={models.map(m => ({ value: m, label: m }))} placeholder={data.default_model} clearLabel="Default" ariaLabel="Model" />
              : <input value={model} onChange={e => setModel(e.target.value.trim())} placeholder={data.default_model} style={{ width: 200 }} aria-label="Model" />}
          </SettingRow>
          <SettingRow title="Let it look books up on the web" help="More accurate for translations and lesser-known books. OpenAI charges a small fee per search.">
            <Switch checked={web} onChange={setWeb} label="Let it look books up on the web" />
          </SettingRow>
        </>}
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {provider === 'openai' && <button type="button" className="btn" onClick={test} disabled={busy || !(key || hasKey)}>{busy ? 'Checking…' : 'Test key'}</button>}
        {provider === 'openai' && data.key_set && <button type="button" className="btn ghost danger-text" onClick={() => save({ clear_key: true, key: undefined })} disabled={busy}>Remove key</button>}
      </div>
      <Section title="Characters" icon="users" sub={<>The <Icon name="refresh" size={12} className="inline-icon" /> button on a book page writes that book's characters again.</>}
        actions={st?.writer === 'app' ? <button type="button" className="btn sm" onClick={run} disabled={st.running}><Icon name="sparkles" size={14} />{st.running ? 'Writing…' : 'Write missing now'}</button> : undefined}>
        {st ? (
          <dl className="set-dl">
            <div><dt>Written by</dt><dd>{st.writer === 'app' ? <>{data.ai_status?.selected === 'pocket' ? 'Pocket AI' : `OpenAI · ${data.model || data.default_model}`}</> : <span className="muted">the external writer (scripts/characters.py), if you run one</span>}</dd></div>
            <div><dt>Waiting</dt><dd>{st.waiting === 0 ? 'nothing — every book has them' : `${st.waiting} book${st.waiting === 1 ? '' : 's'}`}{st.running && <span className="badge cat" style={{ marginLeft: 8 }}>writing…</span>}</dd></div>
            {st.writer === 'app' && <div><dt>Written</dt><dd>{st.written} since the app started{st.failed > 0 && <span className="muted"> · {st.failed} failed, retried in 2 hours</span>}</dd></div>}
            {st.last_error && <div><dt>Last error</dt><dd className="danger-text small">{st.last_error}{st.paused && <div className="muted">Paused until the key or model changes, or you press “Write missing now”.</div>}</dd></div>}
          </dl>
        ) : <Loading />}
      </Section>
    </div>
  )
}

/** Daily highlights: when and where the morning review goes. */
function DailyTab() {
  const { toast } = useToast()
  const { data, setData } = useAsync(() => api.get<DailySettings>(`${V1}/daily/settings`), [])
  const [busy, setBusy] = useState(false)
  if (!data) return <Loading />
  const f = data
  const set = (p: Partial<DailySettings>) => setData({ ...f, ...p })
  const save = async () => {
    setBusy(true)
    try { setData(await api.put<DailySettings>(`${V1}/daily/settings`, f)); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const test = async () => {
    setBusy(true)
    try { await api.put(`${V1}/daily/settings`, f); await api.post(`${V1}/daily/send`); toast('Sent — check your phone') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const hints: Record<string, string> = {
    apprise: 'Apprise API route, e.g. http://192.168.1.10:8000/notify/bookshelf-daily',
    ntfy: 'ntfy topic, e.g. https://ntfy.example.com/reading',
    webhook: 'Any URL that accepts a JSON POST {title, body, url, date, quotes}',
  }
  return (
    <div className="stack loose">
      <Section title="To your phone" icon="bell" sub={<>Every morning a few of your highlights come back: the ones you have not seen for longest, favourites more often, never two from the same book. They are on the <b>Daily</b> page anyway; here you choose whether they are also sent to your phone.</>}>
        <SettingRow title="Send them to my phone every day"><Switch checked={f.enabled} onChange={v => set({ enabled: v })} label="Send them to my phone every day" /></SettingRow>
        <SettingRow title="How many"><Segmented size="sm" value={f.count} onChange={v => set({ count: v })} options={[3, 4, 5, 6, 7].map(n => ({ value: n, label: String(n) }))} label="How many" /></SettingRow>
        <SettingRow title="At"><input type="time" value={f.time} onChange={e => set({ time: e.target.value })} className="set-time" aria-label="Time" /></SettingRow>
        <SettingRow title="Send through"><Segmented size="sm" value={f.channel} onChange={v => set({ channel: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'webhook', label: 'Webhook' }]} label="Send through" /></SettingRow>
        <SettingRow stack title="Address" help={hints[f.channel]}><input value={f.url} onChange={e => set({ url: e.target.value.trim() })} placeholder="http://…" spellCheck={false} aria-label="Address" /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={save} disabled={busy}>Save changes</button><button type="button" className="btn" onClick={test} disabled={busy || !f.url}><Icon name="mail" size={15} />Send today’s now</button></div>
      <Section title="How it picks">
        <ul className="muted small" style={{ paddingLeft: 18, margin: 0, display: 'grid', gap: 4 }}>
          <li>Highlights never shown come first, then the longest unseen; one shown in the last week rests.</li>
          <li>Favourites count double.</li>
          <li>At most one highlight per book each day.</li>
          <li>Hidden highlights never come back (they stay in Quotes).</li>
          <li>Today’s set stays the same all day; <b>Shuffle</b> on the Daily page picks a new one.</li>
        </ul>
      </Section>
    </div>
  )
}
