import { useEffect, useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Settings as SettingsT, ThoughtTemplate, User } from '../lib/types'
import { Card, Confirm, Field, Loading, Modal, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'
import { Avatar } from '../components/people'
import { KindleImport } from '../components/KindleImport'
import { MarkdownEditor } from '../components/MarkdownEditor'
import { LANGUAGES, langName } from '../lib/languages'
import { Select } from '../components/Select'

type Tab = 'general' | 'metadata' | 'templates' | 'users' | 'profile' | 'api' | 'data' | 'about'

export default function Settings() {
  const [tab, setTab] = useState<Tab>('general')
  return (
    <>
      <PageHeader title="Settings" />
      <Tabs tabs={[{ id: 'general', label: 'General' }, { id: 'metadata', label: 'Metadata' }, { id: 'templates', label: 'Templates' }, { id: 'users', label: 'Users' }, { id: 'profile', label: 'Profile' }, { id: 'api', label: 'API tokens' }, { id: 'data', label: 'Backup & data' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
      <div className="mt">
        {tab === 'general' && <General />}
        {tab === 'metadata' && <Metadata />}
        {tab === 'templates' && <Templates />}
        {tab === 'users' && <Users />}
        {tab === 'profile' && <Profile />}
        {tab === 'api' && <Tokens />}
        {tab === 'data' && <Data />}
        {tab === 'about' && <About />}
      </div>
    </>
  )
}

function useSettingsForm() {
  const { settings, setSettings } = useApp()
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
  return { f, setF, save, busy }
}

function General() {
  const { f, setF, save, busy } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <Card title="Preferences">
      <div className="grid" style={{ gap: 14, maxWidth: 560 }}>
        <Field label="App title" help="Shown in the sidebar and browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} /></Field>
        <Field label="Yearly reading goal" help="Books per year. 0 = no goal. Shown on Now, Plan and Stats."><input type="number" min={0} max={1000} value={f.yearly_goal} onChange={e => setF({ ...f, yearly_goal: Math.max(0, Number(e.target.value) || 0) })} style={{ maxWidth: 120 }} /></Field>
        <Field group label="Open on"><Segmented value={f.landing} onChange={v => setF({ ...f, landing: v })} options={[{ value: 'now', label: 'Now' }, { value: 'library', label: 'Library' }, { value: 'planner', label: 'Plan' }]} /></Field>
        <div><button className="btn primary" onClick={save} disabled={busy}>Save</button></div>
      </div>
    </Card>
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
    <div className="two-col">
      <Card title="Book search">
        <div className="grid" style={{ gap: 14 }}>
          <p className="muted">When you add a book, its details and cover come from free public catalogues. Covers are downloaded and kept with your data, so nothing is loaded from outside when you browse.</p>
          <Field group label="Search in" help="The default for the add-book search; you can switch per search.">
            <Segmented value={f.search_source} onChange={v => setF({ ...f, search_source: v })} options={[{ value: 'all', label: 'All' }, { value: 'openlibrary', label: 'Open Library' }, { value: 'google', label: 'Google Books' }]} />
          </Field>
          <Field group label="Preferred languages" help="Search results and authors' bibliographies show books available in these languages first, as that edition (title and cover). The first one wins when a book exists in several.">
            <LanguagePicker value={f.languages} onChange={v => setF({ ...f, languages: v })} />
          </Field>
          <Field label="Google Books API key" help={<>Free: Google Cloud console → enable “Books API” → Credentials → API key. Without a key Google refuses anonymous searches. {f.google_books_env && !f.google_books_key && 'A key is set through GOOGLE_BOOKS_API_KEY.'}</>}>
            <input value={f.google_books_key} onChange={e => setF({ ...f, google_books_key: e.target.value.trim() })} placeholder={f.google_books_env ? 'using GOOGLE_BOOKS_API_KEY' : 'AIza…'} autoComplete="off" spellCheck={false} />
          </Field>
          <div className="row"><button className="btn primary" onClick={save} disabled={busy}>Save</button><button className="btn" onClick={test} disabled={testing || !(f.google_books_key || f.google_books_env)}>{testing ? 'Testing…' : 'Test Google Books'}</button></div>
        </div>
      </Card>
      <Card title="Sources">
        <table className="table compact">
          <tbody>
            <tr><th>Open Library</th><td><span className="badge cat">always on</span></td><td className="muted small">openlibrary.org — no key, community catalogue, good for older and non-English books.</td></tr>
            <tr><th>Google Books</th><td>{f.sources.includes('google') ? <span className="badge cat">on</span> : <span className="badge">needs key</span>}</td><td className="muted small">Broad coverage of recent editions, page counts and descriptions.</td></tr>
          </tbody>
        </table>
        <p className="muted small mt">With “All”, both are searched at once and the same book (same ISBN) is shown once. ISBN searches pick that exact edition.</p>
      </Card>
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
    <Card title="Users" actions={isAdmin && <button className="btn primary sm" onClick={() => setEdit({ role: 'member' })}>+ Add user</button>}>
      <p className="muted">Everyone signs in with their own account and keeps their own shelf: books, ratings, thoughts, quotes and plan are private to each person. Admins manage users and the shared settings.</p>
      <table className="table compact">
        <thead><tr><th /><th>Name</th><th>Email</th><th>Role</th><th /></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td style={{ width: 32 }}><Avatar user={u} /></td>
              <td>{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge" style={{ marginLeft: 6 }}>you</span>}</td>
              <td className="muted">{u.email}</td>
              <td><span className={`badge ${u.role === 'admin' ? 'cat' : ''}`}>{u.role}</span></td>
              <td className="right">{isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their whole shelf: books, thoughts, quotes and reading history." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Card>
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
    <Modal title={user.id ? 'Edit user' : 'Add user'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters. Tell them to change it under Settings → Profile."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add user'}</button></div>
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
    <Card title="Profile">
      <form onSubmit={save} className="grid" style={{ gap: 14, maxWidth: 480 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        {status?.auth_mode === 'local' && <>
          <hr />
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters. Leave empty to keep the current one."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </>}
        <div><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Card>
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
    <Card title="API tokens">
      <p className="muted">Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you and sees your shelf. Good for Homepage widgets (<code>GET /api/v1/dashboard</code>), logging pages from an iOS Shortcut (<code>POST /api/v1/books/{'{id}'}/progress</code>) or a random quote somewhere (<code>GET /api/v1/quotes?random=1</code>). See <code>docs/api.md</code>.</p>
      <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button className="btn primary">Create</button></form>
      {fresh && <div className="callout success mb"><b>Copy it now — it is shown once.</b><pre className="mono" style={{ margin: '8px 0 0' }}>{fresh}</pre></div>}
      {data && data.length > 0 ? (
        <table className="table compact">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
          <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? t.last_used_at.slice(0, 16).replace('T', ' ') : 'never'}</td><td className="muted">{t.created_at.slice(0, 10)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
        </table>
      ) : <p className="muted">No tokens yet.</p>}
      {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </Card>
  )
}

function Data() {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const [covers, setCovers] = useState<{ running: boolean; done: number; found: number; total: number } | null>(null)
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
    <div className="two-col">
      <Card title="Backup">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Covers live next to it in <code>covers/</code>. Restore by stopping the app and replacing <code>pocket-bookshelf.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>
      <Card title="Export & import">
        <p className="muted">JSON is complete (books, thoughts, reading history, quotes) and can be imported again. CSV is one row per book for spreadsheets.</p>
        <div className="row mb wrap"><a className="btn" href={`${V1}/export.json`}>Export JSON</a><a className="btn" href={`${V1}/export.csv`}>Export CSV</a></div>
        <div className="row wrap">
          <label className="btn ghost">{busy ? 'Importing…' : 'Import JSON…'}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
          <label className="btn ghost">{busy ? 'Importing…' : 'Import Goodreads / StoryGraph CSV…'}<input type="file" accept=".csv,text/csv" hidden onChange={e => e.target.files?.[0] && importCSV(e.target.files[0])} /></label>
        </div>
        <div className="row wrap mt"><button className="btn ghost" onClick={() => setKindle(true)}>Import Kindle highlights (My Clippings.txt)…</button></div>
        {kindle && <KindleImport onClose={() => setKindle(false)} />}
        <p className="muted small mt">Goodreads: My Books → Import and export → Export library. StoryGraph: Manage account → Export. Ratings, reviews, read dates and shelves come along; books already on your shelf are skipped. Covers are looked up afterwards by ISBN.</p>
        <hr className="mt mb" />
        <div className="row wrap"><button className="btn" onClick={() => pollCovers(true)} disabled={covers?.running}>Find missing covers</button>
          {covers && <span className="muted small">{covers.running ? `Looking up… ${covers.done}/${covers.total}` : `Done: ${covers.found} of ${covers.total} found`}</span>}</div>
        <p className="muted small mt">Checks Open Library by ISBN for books without a cover, one every few seconds.</p>
      </Card>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <Card title="About">
      <p><b>Pocket Bookshelf</b> is a small reading log: what you read, what you thought of it, the passages worth keeping, and a plan for what to read next. Book details and covers come from Open Library and Google Books. One binary, one SQLite file, one volume.</p>
      <p className="muted small">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-bookshelf" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-bookshelf</a>. Sibling projects: Pocket Habits, Pocket Inventory and Pocket Invoicing.</p>
      {data && (
        <table className="table compact mt">
          <tbody>
            <tr><th>Version</th><td className="mono">{String(data.version)}</td></tr>
            <tr><th>Runtime</th><td className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</td></tr>
            <tr><th>Auth mode</th><td className="mono">{String(data.auth_mode)}</td></tr>
            <tr><th>Database</th><td className="mono">{String(data.db_path)} · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</td></tr>
            <tr><th>Covers</th><td className="mono">{String(data.covers)} files · {(Number(data.cover_bytes) / 1024 / 1024).toFixed(1)} MB</td></tr>
            <tr><th>Timezone</th><td className="mono">{String(data.timezone)}</td></tr>
            <tr><th>Uptime</th><td className="mono">{String(data.uptime)}</td></tr>
            <tr><th>Rows</th><td className="mono">{counts.books} books · {counts.reads} reads · {counts.quotes} quotes · {counts.users} users</td></tr>
          </tbody>
        </table>
      )}
    </Card>
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
          {i > 0 && <button type="button" onClick={() => up(i)} title="Prefer this one more" aria-label={`Move ${langName(c)} up`}>↑</button>}
          <button type="button" onClick={() => set(list.filter(x => x !== c))} title="Remove" aria-label={`Remove ${langName(c)}`}>✕</button>
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
    <div className="grid" style={{ gap: 16 }}>
      <Card title="Thoughts templates" actions={<button className="btn sm" onClick={() => set([...list, { name: 'New template', body: '## 🧠 My thoughts\n\n## 🥰 Who would enjoy it?\n' }])}>+ Add template</button>}>
        <p className="muted">When a book has no thoughts yet, its page offers these as a starting point. Write one heading per line: <code>## Heading</code> for a section, <code>### Sub-heading</code> under it; anything under a heading (like <code>1.</code> <code>2.</code> <code>3.</code>) is kept as a prompt. Sections you leave empty are hidden when the thoughts are shown.</p>
        {list.length === 0 && <p className="muted">No templates. Add one above.</p>}
        <div className="grid" style={{ gap: 14 }}>
          {list.map((t, i) => (
            <div key={i} className="template-edit">
              <div className="row">
                <input value={t.name} onChange={e => upd(i, { name: e.target.value })} placeholder="Name (e.g. Fiction)" style={{ maxWidth: 280 }} aria-label="Template name" />
                <span className="spacer" />
                {i > 0 && <button className="btn ghost sm" onClick={() => { const l = [...list]; [l[i - 1], l[i]] = [l[i], l[i - 1]]; set(l) }} title="Move up">↑</button>}
                <button className="btn ghost sm danger-text" onClick={() => set(list.filter((_, j) => j !== i))}>Delete</button>
              </div>
              <MarkdownEditor rows={Math.min(16, Math.max(6, t.body.split('\n').length + 1))} value={t.body} onChange={v => upd(i, { body: v })} />
            </div>
          ))}
        </div>
        <div className="mt"><button className="btn primary" onClick={save} disabled={busy}>Save templates</button></div>
      </Card>
    </div>
  )
}
