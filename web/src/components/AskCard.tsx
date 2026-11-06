import { useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { Book } from '../lib/types'
import { Card, useToast } from './ui'
import { Icon } from './icons'
import { Select } from './Select'

// "Ask about this book": ready questions that suit where the book stands (and
// your own), each starting a Pocket AI chat about the book, which knows the
// book, your page (so nothing past it gets spoiled), your thoughts and quotes.
// The book's chats are listed here and open in a panel beside the page, with
// Pocket AI inside it. Greyed out with the reason where the link is off;
// nothing at all where there is no Pocket AI.

export interface BookChat { id: string; title: string; status: string; preview: string; model: string; url: string; created_at: string; updated_at: string }
interface ChatsBox { available: boolean; missing?: boolean; reason?: string; chats: BookChat[]; starters: { label: string; text: string }[]; model: string }

const MODELS = [{ value: 'sonnet', label: 'Sonnet', hint: 'balanced' }, { value: 'opus', label: 'Opus', hint: 'deepest' }, { value: 'haiku', label: 'Haiku', hint: 'fastest' }]
const STATUS: Record<string, string> = { running: 'Answering…', waiting: 'Needs you', error: 'Stopped with an error', idle: '' }

/** The chat's name without the book's ("Dune · Who is Stilgar?" → "Who is Stilgar?"). */
const chatName = (c: BookChat, book: Book) => c.title.startsWith(book.title + ' · ') ? c.title.slice(book.title.length + 3) : c.title

function ago(iso: string) {
  const t = Date.parse(iso.includes('T') || iso.includes('Z') ? iso : iso.replace(' ', 'T') + 'Z')
  if (!Number.isFinite(t)) return ''
  const m = Math.round((Date.now() - t) / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m} min ago`
  const h = Math.round(m / 60)
  if (h < 24) return `${h} h ago`
  const d = Math.round(h / 24)
  if (d < 14) return `${d} d ago`
  return new Date(t).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}

export function AskCard({ book }: { book: Book }) {
  const { toast } = useToast()
  const [box, setBox] = useState<ChatsBox | null>(null)
  const [text, setText] = useState('')
  const [starter, setStarter] = useState<{ label: string; text: string } | null>(null)
  const [model, setModel] = useState('')
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState<BookChat | null>(null)
  const [all, setAll] = useState(false)
  const input = useRef<HTMLTextAreaElement>(null)

  const load = () => api.get<ChatsBox>(`${V1}/books/${book.id}/chats`).then(setBox).catch(() => {})
  useEffect(() => { setBox(null); setOpen(null); setText(''); load() }, [book.id]) // eslint-disable-line react-hooks/exhaustive-deps
  // while a chat answers, its state here keeps up; otherwise now and then
  const live = box?.chats.some(c => c.status === 'running' || c.status === 'waiting')
  useEffect(() => {
    if (!box?.available) return
    const t = window.setInterval(() => { if (!document.hidden) load() }, live ? 4000 : 30000)
    return () => window.clearInterval(t)
  }, [box?.available, live, book.id]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!box || box.missing) return null
  const pick = (st: { label: string; text: string }) => { const t = st.text; setStarter(st); setText(t); requestAnimationFrame(() => { const el = input.current; if (el) { el.focus(); el.setSelectionRange(t.length, t.length) } }) }
  const ask = async (q = text, st = starter) => {
    if (!q.trim() || busy) return
    setBusy(true)
    try {
      const name = st && st.text === q ? st.label : undefined
      const chat = await api.post<BookChat>(`${V1}/books/${book.id}/chats`, { text: q, name, model: model || box.model })
      setText(''); setStarter(null)
      setBox(b => b && { ...b, chats: [chat, ...b.chats.filter(c => c.id !== chat.id)] })
      setOpen(chat)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const shown = all ? box.chats : box.chats.slice(0, 5)
  const title = <><Icon name="sparkles" size={16} />Ask about this book</>

  if (!box.available) return (
    <Card className="mt ask-card off" title={title}>
      <p className="muted small icon-text"><Icon name="lock" size={14} />{box.reason}</p>
    </Card>
  )
  return (
    <Card className="mt ask-card" title={title} sub={book.status === 'reading' ? 'Pocket AI knows your page and keeps what comes after it to itself.' : book.status === 'want' ? 'Pocket AI knows you haven’t started it, so it won’t spoil it.' : 'Pocket AI knows the book, your thoughts and your quotes.'}>
      {box.starters.length > 0 && (
        <div className="ask-starters">
          {box.starters.map(s => (
            <button key={s.label} type="button" className="chip" title={s.text} onClick={() => pick(s)} onDoubleClick={() => ask(s.text, s)}>{s.label}</button>
          ))}
        </div>
      )}
      <form className="ask-box" onSubmit={e => { e.preventDefault(); ask() }}>
        <textarea ref={input} rows={text.length > 90 ? 3 : 2} value={text} onChange={e => setText(e.target.value)} placeholder={`Ask anything about ${book.title}…`}
          onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey || !e.shiftKey) && !e.nativeEvent.isComposing) { e.preventDefault(); ask() } }} />
        <div className="ask-box-bar">
          <Select value={model || box.model} onChange={setModel} options={MODELS} ariaLabel="Model" className="ask-model" />
          <span className="muted small grow">Enter to ask · Shift+Enter for a new line</span>
          <button className="btn primary sm" disabled={!text.trim() || busy}>{busy ? 'Starting…' : <><Icon name="sparkles" size={14} />Ask</>}</button>
        </div>
      </form>
      {box.chats.length > 0 && <>
        <h3 className="ask-sub">Conversations <span className="badge">{box.chats.length}</span></h3>
        <ul className="mini-list ask-chats">
          {shown.map(c => (
            <li key={c.id} className={open?.id === c.id ? 'on' : ''}>
              <button type="button" className="ask-chat" onClick={() => setOpen(c)}>
                <span className={`ask-dot ${c.status}`} aria-hidden />
                <span className="grow">
                  <b>{chatName(c, book)}</b>
                  {c.preview && <span className="muted small clamp2">{c.preview}</span>}
                </span>
                <span className="muted small nowrap">{STATUS[c.status] || ago(c.updated_at)}</span>
              </button>
            </li>
          ))}
        </ul>
        {box.chats.length > 5 && <button type="button" className="btn ghost sm mt-sm" onClick={() => setAll(a => !a)}>{all ? 'Show fewer' : `Show all ${box.chats.length}`}</button>}
      </>}
      {open && <ChatPanel chat={open} name={chatName(open, book)} book={book} onClose={() => { setOpen(null); load() }}
        onStatus={(id, status) => setBox(b => b && { ...b, chats: b.chats.map(c => c.id === id && status ? { ...c, status } : c) })} />}
    </Card>
  )
}

/** Pocket AI's conversation beside the page (full screen on a phone). */
function ChatPanel({ chat, name, book, onClose, onStatus }: { chat: BookChat; name: string; book: Book; onClose: () => void; onStatus: (id: string, status: string) => void }) {
  const frame = useRef<HTMLIFrameElement>(null)
  const src = chat.url + (chat.url.includes('?') ? '&' : '?') + 'embed=1&links=1'
  const origin = (() => { try { return new URL(chat.url).origin } catch { return '' } })()
  useEffect(() => {
    const h = (e: MessageEvent) => {
      if (e.source !== frame.current?.contentWindow || e.origin !== origin || e.data?.type !== 'pocket-ai') return
      if (typeof e.data.link === 'string') {
        const u = new URL(e.data.link)
        if (u.origin === location.origin) location.assign(u.href); else window.open(u.href, '_blank', 'noopener')
        return
      }
      if (e.data.thread_id === chat.id && typeof e.data.status === 'string') onStatus(chat.id, e.data.status)
    }
    window.addEventListener('message', h)
    const k = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop')) onClose() }
    window.addEventListener('keydown', k)
    document.documentElement.classList.add('ask-panel-open')
    return () => { window.removeEventListener('message', h); window.removeEventListener('keydown', k); document.documentElement.classList.remove('ask-panel-open') }
  }, [chat.id, origin]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <aside className="ask-panel" aria-label={`Pocket AI: ${name}`}>
      <header className="ask-panel-head">
        <span className="grow ask-panel-title"><span className="muted small">{book.title}</span><b>{name}</b></span>
        <a className="btn ghost sm icon" href={chat.url} target="_blank" rel="noreferrer" title="Open in Pocket AI" aria-label="Open in Pocket AI"><Icon name="external" size={15} /></a>
        <button type="button" className="btn ghost sm icon" onClick={onClose} title="Close (Esc)" aria-label="Close"><Icon name="x" size={16} /></button>
      </header>
      <iframe ref={frame} key={chat.id} src={src} title={`Pocket AI: ${name}`} allow="clipboard-write; microphone" />
    </aside>
  )
}
