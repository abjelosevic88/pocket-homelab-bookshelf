import { useState } from 'react'
import { api, ApiError, V1 } from '../lib/api'
import type { Book } from '../lib/types'
import { langName } from '../lib/languages'
import { MenuButton, Spinner, useAsync, useToast, type MenuItem } from './ui'
import { Icon } from './icons'

// "Get it": ask Pocket Requests (through Pocket Core) for an ebook or an
// audiobook of a book on the shelves, and see whether it is already on the
// server or on its way. Without the link the buttons stay, greyed out, with
// the reason.

type Kind = 'ebook' | 'audiobook'
interface Req { id: number; status: string; state: string; progress: number; url?: string }
interface Fmt { have: { url: string } | null; request: Req | null }
export interface Ask { id: number; kind: Kind; language: string; status: string; state: string; progress: number; url?: string }
export interface GetItInfo { available: boolean; reason?: string; found: boolean; book_key?: string; ebook: Fmt; audiobook: Fmt; app_url?: string; languages?: string[]; asked?: Ask[] }

const KIND_LABEL: Record<Kind, string> = { ebook: 'Ebook', audiobook: 'Audiobook' }
export const KIND_ICON: Record<Kind, string> = { ebook: 'tablet', audiobook: 'headphones' }

const STATE_LABEL: Record<string, string> = {
  waiting: 'Waiting for approval', sent: 'Asked for', unreleased: 'Not out yet', searching: 'Looking for a copy',
  downloading: 'Downloading', importing: 'Arriving', attention: 'Needs a look', partial: 'Partly here',
  available: 'Here', removed: 'Removed', failed: 'Couldn’t be asked for',
}

export function stateText(r: { status: string; state: string; progress: number }): string {
  if (r.status === 'pending') return 'Waiting for approval'
  if (r.status === 'declined') return 'Declined'
  let s = STATE_LABEL[r.state] || r.state
  if (r.state === 'downloading' && r.progress > 0 && r.progress < 1) s += ` · ${Math.round(r.progress * 100)}%`
  return s
}

export const liveAsk = (a: { status: string; state: string }) => a.status !== 'declined' && a.status !== 'cancelled' && a.state !== 'removed'

/** what = "the ebook" / "the Serbian ebook" */
function what(kind: Kind, language?: string) { return `the ${language ? langName(language) + ' ' : ''}${kind}` }

/** postGet sends a "Get it" request to one of Bookshelf's proxies; it returns the request's page (if any) and a message. */
export async function postGet(path: string, body: Record<string, unknown>): Promise<{ ok: boolean; msg: string; url?: string }> {
  const kind = body.kind as Kind
  try {
    const r = await api.post<Req>(path, body)
    return { ok: true, msg: `Asked Pocket Requests for ${what(kind, body.language as string)}`, url: r.url }
  } catch (e) {
    const d = e instanceof ApiError ? e.data : {}
    return { ok: false, msg: (e as Error).message, url: (d.url as string) || undefined }
  }
}

export function openAction(url?: string) {
  return url ? { label: 'Open', run: () => { window.open(url, '_blank', 'noopener') } } : undefined
}

/** The "Get it" box on the book page. */
export function GetItBox({ book }: { book: Book }) {
  const { toast } = useToast()
  const { data, loading, setData, reload } = useAsync(() => api.get<GetItInfo>(`${V1}/books/${book.id}/requests`), [book.id])
  const [busy, setBusy] = useState<Kind | null>(null)
  const go = async (kind: Kind, language = '') => {
    setBusy(kind)
    const r = await postGet(`${V1}/books/${book.id}/requests`, { kind, language })
    setBusy(null)
    toast(r.msg, r.ok ? 'ok' : 'err', openAction(r.url))
    try { setData(await api.get<GetItInfo>(`${V1}/books/${book.id}/requests?fresh=1`)) } catch { reload() }
  }
  const off = !!data && !data.available
  const langs = data?.languages || []
  return (
    <div className="get-it" aria-busy={loading}>
      <div className="get-it-head">
        <span className="get-it-title">Get it</span>
        {data?.app_url && <a className="muted small icon-text" href={data.app_url} target="_blank" rel="noreferrer">Pocket Requests<Icon name="external" size={11} /></a>}
        {loading && !data && <Spinner size={12} />}
      </div>
      {(['ebook', 'audiobook'] as Kind[]).map(kind => {
        const f = data?.[kind]
        const asks = (data?.asked || []).filter(a => a.kind === kind && liveAsk(a))
        const fallback = f?.request && liveAsk(f.request) && !asks.length ? f.request : null
        const label = asks.length ? 'Another language' : `Get ${kind}`
        const ready = !!data && !off && busy === null
        const button = ready && langs.length > 1 ? (
          <MenuButton label={`Ask for the ${kind} in Pocket Requests`} className="btn sm" align="right"
            items={[{ heading: `The ${kind} in` }, { label: 'Any language I read', onSelect: () => go(kind) }, ...langs.map(l => ({ label: langName(l), onSelect: () => go(kind, l) }))]}>
            <Icon name="download" size={13} />{label}<Icon name="chevron-down" size={12} />
          </MenuButton>
        ) : (
          <button type="button" className="btn sm" disabled={!ready} onClick={() => go(kind)} title={off ? data?.reason : `Ask for the ${kind} in Pocket Requests`}>
            {busy === kind ? <Spinner size={12} /> : <Icon name="download" size={13} />}{label}
          </button>
        )
        return (
          <div key={kind} className="get-it-block">
            <div className="get-it-row">
              <span className="get-it-kind"><Icon name={KIND_ICON[kind]} size={14} />{KIND_LABEL[kind]}</span>
              {f?.have ? (
                <a className="btn sm" href={f.have.url} target="_blank" rel="noreferrer"><Icon name="check" size={13} />Here · open</a>
              ) : fallback ? (
                fallback.url ? <a className="get-it-state" href={fallback.url} target="_blank" rel="noreferrer">{stateText(fallback)}<Icon name="external" size={11} /></a>
                  : <span className="get-it-state">{stateText(fallback)}</span>
              ) : asks.length && off ? null : button}
            </div>
            {!f?.have && asks.map(a => (
              <div key={a.id} className="get-it-ask">
                <span className="muted">{a.language ? langName(a.language) : 'Any language'}</span>
                {a.url ? <a className="get-it-state" href={a.url} target="_blank" rel="noreferrer">{stateText(a)}<Icon name="external" size={11} /></a> : <span className="get-it-state">{stateText(a)}</span>}
              </div>
            ))}
          </div>
        )
      })}
      {off && <p className="core-hint">{data?.reason}</p>}
    </div>
  )
}

/** A compact "Get" menu: the ebook (in any language or one of the box's) or the audiobook. */
export function GetMenu({ onGet, languages = [], label = 'Get it from Pocket Requests', className = 'btn ghost sm icon-only', children, disabled }:
  { onGet: (kind: Kind, language: string) => void; languages?: string[]; label?: string; className?: string; children?: React.ReactNode; disabled?: boolean }) {
  if (disabled) return <button type="button" className={className} disabled title={label}>{children || <Icon name="download" size={15} />}</button>
  const items: MenuItem[] = [{ heading: 'Ask Pocket Requests for' }, { label: 'The ebook', icon: KIND_ICON.ebook, onSelect: () => onGet('ebook', '') }]
  if (languages.length > 1) for (const l of languages) items.push({ label: `The ebook in ${langName(l)}`, onSelect: () => onGet('ebook', l) })
  items.push({ label: 'The audiobook', icon: KIND_ICON.audiobook, onSelect: () => onGet('audiobook', '') })
  return <MenuButton label={label} className={className} align="right" items={items}>{children || <Icon name="download" size={15} />}</MenuButton>
}

/** A compact "Get" menu for a row in the reading plan. */
export function GetItMenu({ book }: { book: Book }) {
  const { toast } = useToast()
  const go = async (kind: Kind, language: string) => {
    const r = await postGet(`${V1}/books/${book.id}/requests`, { kind, language })
    toast(r.msg, r.ok ? 'ok' : 'err', openAction(r.url))
  }
  return <GetMenu onGet={go} />
}

// ---- "+ Want": also get it ----

export interface WantGetIt { offer?: boolean; asked?: { kind: Kind; ok: boolean; message: string; url?: string }[] }

/** wantFollowUp says what happened after a book went on Want to read: what
 * was asked of Pocket Requests (the person's setting), or "Also get it?". */
export function wantFollowUp(toast: ReturnType<typeof useToast>['toast'], book: Book, g?: WantGetIt) {
  if (!g) return
  if (g.asked?.length) {
    const ok = g.asked.filter(a => a.ok)
    const bad = g.asked.filter(a => !a.ok)
    if (ok.length) toast(`Pocket Requests: the ${ok.map(a => a.kind).join(' and the ')} — asked for`, 'ok', openAction(ok[0].url))
    if (bad.length) toast(`Couldn’t ask for the ${bad[0].kind}: ${bad[0].message}`, 'err')
    return
  }
  if (g.offer) {
    const ask = async (kind: Kind) => {
      const r = await postGet(`${V1}/books/${book.id}/requests`, { kind })
      toast(r.msg, r.ok ? 'ok' : 'err', openAction(r.url))
    }
    toast(
      <span className="also-get">Also get it?
        <button type="button" className="btn sm also-get-btn" onClick={() => ask('ebook')}><Icon name="tablet" size={12} />Ebook</button>
        <button type="button" className="btn sm also-get-btn" onClick={() => ask('audiobook')}><Icon name="headphones" size={12} />Audiobook</button>
      </span>, 'ok', { label: 'Not now', run: () => {} })
  }
}
