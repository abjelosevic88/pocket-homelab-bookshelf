import { useState } from 'react'
import { api, ApiError, V1 } from '../lib/api'
import type { Book } from '../lib/types'
import { MenuButton, Spinner, useAsync, useToast } from './ui'
import { Icon } from './icons'

// "Get it": ask Pocket Requests (through Pocket Core) for an ebook or an
// audiobook of a book on the shelves, and see whether it is already on the
// server or on its way. Without the link the buttons stay, greyed out, with
// the reason.

type Kind = 'ebook' | 'audiobook'
interface Req { id: number; status: string; state: string; progress: number; url?: string }
interface Fmt { have: { url: string } | null; request: Req | null }
export interface GetItInfo { available: boolean; reason?: string; found: boolean; book_key?: string; ebook: Fmt; audiobook: Fmt; app_url?: string }

const KIND_LABEL: Record<Kind, string> = { ebook: 'Ebook', audiobook: 'Audiobook' }
const KIND_ICON: Record<Kind, string> = { ebook: 'tablet', audiobook: 'headphones' }

const STATE_LABEL: Record<string, string> = {
  waiting: 'Waiting for approval', sent: 'Asked for', unreleased: 'Not out yet', searching: 'Looking for a copy',
  downloading: 'Downloading', importing: 'Arriving', attention: 'Needs a look', partial: 'Partly here',
  available: 'Here', removed: 'Removed', failed: 'Couldn’t be asked for',
}

function stateText(r: Req): string {
  if (r.status === 'pending') return 'Waiting for approval'
  if (r.status === 'declined') return 'Declined'
  let s = STATE_LABEL[r.state] || r.state
  if (r.state === 'downloading' && r.progress > 0 && r.progress < 1) s += ` · ${Math.round(r.progress * 100)}%`
  return s
}

/** ask posts the request; it returns the request's page (if any) and a message. */
async function ask(bookId: number, kind: Kind): Promise<{ ok: boolean; msg: string; url?: string }> {
  try {
    const r = await api.post<Req>(`${V1}/books/${bookId}/requests`, { kind })
    return { ok: true, msg: `${KIND_LABEL[kind]} asked for in Pocket Requests`, url: r.url }
  } catch (e) {
    const d = e instanceof ApiError ? e.data : {}
    return { ok: false, msg: (e as Error).message, url: (d.url as string) || undefined }
  }
}

function openAction(url?: string) {
  return url ? { label: 'Open', run: () => { window.open(url, '_blank', 'noopener') } } : undefined
}

/** The "Get it" box on the book page. */
export function GetItBox({ book }: { book: Book }) {
  const { toast } = useToast()
  const { data, loading, setData, reload } = useAsync(() => api.get<GetItInfo>(`${V1}/books/${book.id}/requests`), [book.id])
  const [busy, setBusy] = useState<Kind | null>(null)
  const go = async (kind: Kind) => {
    setBusy(kind)
    const r = await ask(book.id, kind)
    setBusy(null)
    toast(r.msg, r.ok ? 'ok' : 'err', openAction(r.url))
    try { setData(await api.get<GetItInfo>(`${V1}/books/${book.id}/requests?fresh=1`)) } catch { reload() }
  }
  const off = !!data && !data.available
  return (
    <div className="get-it" aria-busy={loading}>
      <div className="get-it-head">
        <span className="get-it-title">Get it</span>
        {data?.app_url && <a className="muted small icon-text" href={data.app_url} target="_blank" rel="noreferrer">Pocket Requests<Icon name="external" size={11} /></a>}
        {loading && !data && <Spinner size={12} />}
      </div>
      {(['ebook', 'audiobook'] as Kind[]).map(kind => {
        const f = data?.[kind]
        const req = f?.request && f.request.status !== 'declined' && f.request.status !== 'cancelled' && f.request.state !== 'removed' ? f.request : null
        return (
          <div key={kind} className="get-it-row">
            <span className="get-it-kind"><Icon name={KIND_ICON[kind]} size={14} />{KIND_LABEL[kind]}</span>
            {f?.have ? (
              <a className="btn sm" href={f.have.url} target="_blank" rel="noreferrer"><Icon name="check" size={13} />Here · open</a>
            ) : req ? (
              req.url ? <a className="get-it-state" href={req.url} target="_blank" rel="noreferrer">{stateText(req)}<Icon name="external" size={11} /></a>
                : <span className="get-it-state">{stateText(req)}</span>
            ) : (
              <button type="button" className="btn sm" disabled={!data || off || busy !== null} onClick={() => go(kind)}
                title={off ? data?.reason : `Ask for the ${kind} in Pocket Requests`}>
                {busy === kind ? <Spinner size={12} /> : <Icon name="download" size={13} />}Get {kind}
              </button>
            )}
          </div>
        )
      })}
      {off && <p className="core-hint">{data?.reason}</p>}
    </div>
  )
}

/** A compact "Get" menu for a row in the reading plan. */
export function GetItMenu({ book }: { book: Book }) {
  const { toast } = useToast()
  const go = async (kind: Kind) => {
    const r = await ask(book.id, kind)
    toast(r.msg, r.ok ? 'ok' : 'err', openAction(r.url))
  }
  return (
    <MenuButton label="Get it from Pocket Requests" className="btn ghost sm icon-only" align="right"
      items={[{ heading: 'Ask Pocket Requests for' }, { label: 'The ebook', icon: 'tablet', onSelect: () => go('ebook') }, { label: 'The audiobook', icon: 'headphones', onSelect: () => go('audiobook') }]}>
      <Icon name="download" size={15} />
    </MenuButton>
  )
}
