// The app's own chat, as in Pocket Projects: a round button in the bottom-right
// corner opens Pocket AI in a window that floats over the page, or sits beside it
// as a sidebar (the button in its head switches; the sidebar can be resized).
// It starts as a general chat about Pocket Bookshelf (Pocket AI ?embed=1&ask=bookshelf),
// and a book's chats ("Ask about this book") open in it too. "@" in the conversation
// adds the page the person is looking at, or the text selected on it.
// Desktop only (a phone gets the book's chat full screen). The frame stays mounted
// while hidden, so a running answer goes on.
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { Icon } from './icons'
import { useMediaQuery } from './ui'

export const CHAT_DESKTOP = '(min-width: 768px) and (hover: hover) and (pointer: fine)'
const DOCKED = 'pb-app-chat-docked'
const URL_KEY = 'pb-app-chat-url'
const TITLE_KEY = 'pb-app-chat-title'
const WIDTH = 'pb-app-chat-w'
const SHOWN = 'pb-app-chat-open' // '0' = hidden while in the sidebar (a reload keeps it hidden)
const APP = 'bookshelf'
const APP_NAME = 'Pocket Bookshelf'
const KEYS = 'mod+i'
const clampW = (w: number) => Math.round(Math.max(340, Math.min(w, window.innerWidth - 480, 900)))

/** What a book chat is called in the window's head. */
export interface ChatTitle { title: string; sub?: string }

/** Opens one of Pocket AI's chats (a book's) in the app's chat window. */
export function openAppChat(threadId: string, title: ChatTitle) {
  window.dispatchEvent(new CustomEvent('pb:app-chat-open', { detail: { threadId, title } }))
}

let aiURL: Promise<string> | null = null
/** Pocket AI's public address ('' = none here). */
function pocketAI(): Promise<string> {
  aiURL ??= api.get<{ url: string }>(`${V1}/ai`).then(r => (r.url || '').replace(/\/+$/, '')).catch(() => { aiURL = null; return '' })
  return aiURL
}

/** The frame's address: embedded, links come back here, and a new chat in it is this app's. */
function frameURL(ai: string, path: string, extra: Record<string, string> = {}) {
  const u = new URL(ai + path)
  u.searchParams.set('embed', '1')
  u.searchParams.set('tabs', '1') // "@" asks this page for its items
  u.searchParams.set('links', '1')
  u.searchParams.set('chat_key', KEYS)
  u.searchParams.set('ask', APP)
  u.searchParams.set('app_url', location.origin + '/')
  u.searchParams.set('app_name', APP_NAME)
  for (const [k, v] of Object.entries(extra)) u.searchParams.set(k, v)
  return u.href
}
const startURL = (ai: string, extra: Record<string, string> = {}) => frameURL(ai, '/', extra)
const threadURL = (ai: string, id: string) => frameURL(ai, `/t/${encodeURIComponent(id)}`)

/** The conversation last open, if it is on this Pocket AI. */
const savedURL = (ai: string) => {
  const u = localStorage.getItem(URL_KEY) || ''
  return u.startsWith(ai + '/') ? u : ''
}
const savedTitle = (): ChatTitle | null => { try { return JSON.parse(localStorage.getItem(TITLE_KEY) || 'null') } catch { return null } }

/** What the person is looking at, for "@": the top bar's title and the page's text. */
function pageTitle() {
  const parts = [...document.querySelectorAll('.topbar .crumb, .topbar .ellipsis, .topbar-title')].map(e => e.textContent?.trim()).filter(Boolean)
  return parts.join(' › ') || document.title || APP_NAME
}
const safeName = (s: string) => (s.replace(/[\\/:*?"<>|\n\r›]+/g, ' ').replace(/\s+/g, ' ').trim() || 'Page').slice(0, 100)

function pageFile(): File {
  const title = pageTitle()
  const main = document.querySelector<HTMLElement>('main.content')
  const text = (main?.innerText || '').replace(/\n{3,}/g, '\n\n').trim().slice(0, 60000)
  const body = `# ${title}\n\n${APP_NAME} page: ${location.href}\n\nWhat the page shows (its visible text):\n\n${text || '(nothing readable)'}\n`
  return new File([body], `${safeName(title)}.md`, { type: 'text/markdown' })
}

function comboOf(e: KeyboardEvent) {
  const mod = navigator.platform.toLowerCase().includes('mac') ? e.metaKey : e.ctrlKey
  return `${mod ? 'mod+' : ''}${e.altKey ? 'alt+' : ''}${e.shiftKey ? 'shift+' : ''}${e.key.toLowerCase()}`
}
const keyLabel = navigator.platform.toLowerCase().includes('mac') ? '⌘I' : 'Ctrl+I'

export function AppChat() {
  const desktop = useMediaQuery(CHAT_DESKTOP)
  const [ai, setAI] = useState('')
  const [open, setOpen] = useState(false)
  const [docked, setDocked] = useState(() => localStorage.getItem(DOCKED) === '1')
  const [src, setSrc] = useState('') // the frame's address; set when it (re)starts, not on every move inside it
  const [threadId, setThreadId] = useState('')
  const [status, setStatus] = useState('')
  const [head, setHead] = useState<(ChatTitle & { id: string }) | null>(null) // a book chat's name while it is the one open
  const [w, setW] = useState(() => clampW(Number(localStorage.getItem(WIDTH)) || 440))
  const [drag, setDrag] = useState(false)
  const frame = useRef<HTMLIFrameElement>(null)
  const selection = useRef('')
  const navigate = useNavigate()
  const go = useRef(navigate)
  go.current = navigate

  useEffect(() => { if (desktop) pocketAI().then(setAI) }, [desktop])
  // docked last time: the sidebar comes back with the conversation it had
  useEffect(() => {
    if (!ai || !docked || src || localStorage.getItem(SHOWN) === '0') return
    const saved = savedURL(ai)
    setSrc(saved || startURL(ai))
    if (saved) { const t = savedTitle(); if (t) setHead({ ...t, id: saved.match(/\/t\/([^/?]+)/)?.[1] || '' }) }
    setOpen(true)
  }, [ai]) // eslint-disable-line react-hooks/exhaustive-deps

  // text selected on the page before the person clicks into the conversation
  useEffect(() => {
    const h = () => {
      const s = window.getSelection()?.toString().trim() || ''
      if (s) selection.current = s.slice(0, 20000)
      else if (document.activeElement !== frame.current) selection.current = ''
    }
    document.addEventListener('selectionchange', h)
    return () => document.removeEventListener('selectionchange', h)
  }, [])

  useEffect(() => {
    if (!ai) return
    const origin = new URL(ai).origin
    const h = (e: MessageEvent) => {
      if (e.source !== frame.current?.contentWindow || e.origin !== origin || e.data?.type !== 'pocket-ai') return
      const reply = (body: Record<string, unknown>) => frame.current?.contentWindow?.postMessage({ type: 'pocket-ai-context', req: e.data.req, ...body }, origin)
      if (e.data.want === 'context') {
        const items = [{ id: 'page', kind: 'page', title: pageTitle(), sub: 'The page you are looking at' }]
        const sel = window.getSelection()?.toString().trim() || selection.current
        if (sel) items.push({ id: 'selection', kind: 'page', title: 'Selected text', sub: sel.slice(0, 60) })
        reply({ label: 'this page', items })
        return
      }
      if (e.data.want === 'context-item') {
        if (e.data.id === 'page') reply({ file: pageFile() })
        else if (e.data.id === 'selection') reply({ text: '\n> ' + (window.getSelection()?.toString().trim() || selection.current).split('\n').join('\n> ') + '\n' })
        else reply({ error: 'That is not on the page any more' })
        return
      }
      if (e.data.open?.thread_id) { setHead(null); setSrc(threadURL(ai, String(e.data.open.thread_id))); return }
      if (e.data.key === 'toggle-chat') { toggle.current(); return } // the key typed in the conversation
      if (typeof e.data.link === 'string') {
        const u = new URL(e.data.link)
        if (u.origin === location.origin) go.current(u.pathname + u.search + u.hash); else window.open(u.href, '_blank', 'noopener')
        return
      }
      // where the frame is: remembered, so a reload comes back to the same conversation
      const id = e.data.thread_id ? String(e.data.thread_id) : ''
      setThreadId(id)
      setStatus(String(e.data.status || ''))
      setHead(h => h && h.id !== id ? null : h)
      if (id) localStorage.setItem(URL_KEY, threadURL(ai, id))
    }
    window.addEventListener('message', h)
    return () => window.removeEventListener('message', h)
  }, [ai])

  // a book's chat ("Ask about this book") opens here
  useEffect(() => {
    if (!ai) return
    const h = (e: Event) => {
      const { threadId: id, title } = (e as CustomEvent<{ threadId: string; title: ChatTitle }>).detail
      setHead({ ...title, id })
      localStorage.setItem(TITLE_KEY, JSON.stringify(title))
      localStorage.setItem(URL_KEY, threadURL(ai, id))
      setThreadId(id)
      setStatus('')
      setSrc(threadURL(ai, id) + '&n=' + Date.now().toString(36))
      setOpen(true)
      localStorage.removeItem(SHOWN)
    }
    window.addEventListener('pb:app-chat-open', h)
    return () => window.removeEventListener('pb:app-chat-open', h)
  }, [ai])

  useEffect(() => {
    if (!open || docked) return
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop')) setOpen(false) }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [open, docked])

  // ⌘I opens and closes it from anywhere on the page but a text editor (italic there)
  const toggle = useRef(() => {})
  useEffect(() => {
    if (!desktop || !ai) return
    const key = (e: KeyboardEvent) => {
      if (comboOf(e) !== KEYS) return
      if ((e.target as HTMLElement | null)?.closest?.('[contenteditable="true"], .ProseMirror, .md-editor')) return
      e.preventDefault(); toggle.current()
    }
    const ev = () => toggle.current()
    window.addEventListener('keydown', key)
    window.addEventListener('pb:app-chat', ev)
    return () => { window.removeEventListener('keydown', key); window.removeEventListener('pb:app-chat', ev) }
  }, [desktop, ai])

  if (!desktop || !ai) return null

  const show = () => {
    if (!src) { const saved = savedURL(ai); setSrc(saved || startURL(ai)); if (saved) { const t = savedTitle(); if (t) setHead({ ...t, id: saved.match(/\/t\/([^/?]+)/)?.[1] || '' }) } }
    setOpen(true); localStorage.removeItem(SHOWN); window.setTimeout(() => frame.current?.focus(), 60)
  }
  const hide = () => { setOpen(false); if (docked) localStorage.setItem(SHOWN, '0') } // keeps its shape for next time
  toggle.current = () => { if (open) hide(); else show() }
  const fresh = (extra: Record<string, string> = {}) => {
    localStorage.removeItem(URL_KEY); localStorage.removeItem(TITLE_KEY)
    setThreadId(''); setStatus(''); setHead(null)
    setSrc(startURL(ai, { ...extra, n: Date.now().toString(36) }))
  }
  const newChat = () => fresh()
  // every earlier chat about Bookshelf, in the frame (Pocket AI lists them, with a search)
  const history = () => fresh({ history: '1' })
  const dock = (on: boolean) => { setDocked(on); localStorage.setItem(DOCKED, on ? '1' : '0') }
  const close = () => { setOpen(false); if (docked) dock(false) }
  const resize = (e: React.PointerEvent) => {
    e.preventDefault()
    const x0 = e.clientX, w0 = w
    setDrag(true)
    const mv = (ev: PointerEvent) => setW(clampW(w0 + x0 - ev.clientX))
    const up = (ev: PointerEvent) => {
      setDrag(false)
      localStorage.setItem(WIDTH, String(clampW(w0 + x0 - ev.clientX)))
      window.removeEventListener('pointermove', mv)
      window.removeEventListener('pointerup', up)
    }
    window.addEventListener('pointermove', mv)
    window.addEventListener('pointerup', up)
  }
  const busy = status === 'running' || status === 'waiting'
  const title = head && head.id === threadId ? head : null

  return (
    <>
      {/* one element in both shapes: moving the frame would reload the conversation */}
      {src && (
        <aside className={`app-chat ${docked ? 'docked' : 'floating'} ${open ? '' : 'closed'} ${drag ? 'dragging' : ''}`}
          style={docked ? { width: w } : undefined} aria-label={title ? `Chat: ${title.title}` : `Chat about ${APP_NAME}`} aria-hidden={!open || undefined}>
          {docked && <div className="app-chat-grip" onPointerDown={resize} role="separator" aria-orientation="vertical" aria-label="Resize" />}
          <div className="app-chat-head">
            <Icon name="sparkles" size={15} className="app-chat-ico" />
            {title
              ? <span className="app-chat-title two"><span className="ellipsis muted small">{title.sub}</span><span className="ellipsis">{title.title}</span></span>
              : <span className="app-chat-title ellipsis">Ask {APP_NAME}</span>}
            <button type="button" className="btn ghost icon-only sm" onClick={history} title="Earlier chats" aria-label="Earlier chats"><Icon name="clock" size={15} /></button>
            <button type="button" className="btn ghost icon-only sm" onClick={newChat} title="New chat" aria-label="New chat"><Icon name="plus" size={16} /></button>
            <button type="button" className="btn ghost icon-only sm" onClick={() => dock(!docked)} title={docked ? 'Float over the page' : 'Open in the sidebar'} aria-label={docked ? 'Float over the page' : 'Open in the sidebar'}>
              <Icon name={docked ? 'message' : 'sidebar'} size={15} />
            </button>
            <a className="btn ghost icon-only sm" href={threadId ? `${ai}/t/${encodeURIComponent(threadId)}` : `${ai}/`} target="_blank" rel="noreferrer" title="Open in Pocket AI" aria-label="Open in Pocket AI"><Icon name="external" size={15} /></a>
            <button type="button" className="btn ghost icon-only sm" onClick={close} title={docked ? 'Close the sidebar' : `Hide (Esc, ${keyLabel})`} aria-label="Close"><Icon name={docked ? 'x' : 'chevron-down'} size={16} /></button>
          </div>
          <iframe ref={frame} key={src} src={src} title={title ? `Chat: ${title.title}` : `Chat about ${APP_NAME}`} allow="clipboard-read; clipboard-write; camera; microphone" />
        </aside>
      )}
      {!open && (
        <button type="button" className={`app-chat-fab ${busy ? status : ''}`} onClick={show} title={`Ask about ${APP_NAME} (${keyLabel})`} aria-label={`Ask about ${APP_NAME}`}>
          <Icon name="sparkles" size={22} />
          {busy && <span className="app-chat-fab-dot" />}
        </button>
      )}
    </>
  )
}
