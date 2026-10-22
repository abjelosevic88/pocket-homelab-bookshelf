import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { langName } from '../lib/languages'
import { Card, Segmented, useAsync, useToast } from './ui'
import { Icon } from './icons'

interface Summary { status: 'ok' | 'unknown'; text: string; source: string; updated_at: string }
interface Data { langs: string[]; summaries: Record<string, Summary>; writer: boolean }

const LANG_KEY = 'pb.characterLang' // the same language choice as the characters

/** "About the book": a short spoiler-free summary written by AI first, then the catalogue's description. */
export function AboutCard({ bookId, description }: { bookId: number; description: string }) {
  const { toast } = useToast()
  const { data, setData, reload } = useAsync(() => api.get<Data>(`${V1}/books/${bookId}/summary`), [bookId])
  const [lang, setLang] = useState(() => localStorage.getItem(LANG_KEY) || '')
  const pending = !!data && data.writer && data.langs.some(l => !data.summaries[l])
  // While it is being written, look again now and then.
  useEffect(() => {
    if (!pending) return
    const t = setInterval(reload, 15000)
    const stop = setTimeout(() => clearInterval(t), 10 * 60_000)
    return () => { clearInterval(t); clearTimeout(stop) }
  }, [pending]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!data) return description ? <Card title="About the book" className="mt"><Description text={description} /></Card> : null

  const langs = data.langs.filter(l => data.summaries[l]?.status === 'ok' && data.summaries[l].text)
  const current = langs.includes(lang) ? lang : langs[0]
  const sum = current ? data.summaries[current] : undefined
  const pick = (l: string) => { setLang(l); localStorage.setItem(LANG_KEY, l) }
  const again = async () => {
    try { await api.del(`${V1}/books/${bookId}/summary`); setData({ ...data, summaries: {} }); toast('Writing the summary again…') } catch (e) { toast((e as Error).message, 'err') }
  }
  if (!sum && !description && !pending) return null
  return (
    <Card title="About the book" className="mt" actions={(sum || pending) && <>
      {langs.length > 1 && <Segmented size="sm" value={current} onChange={pick} options={langs.map(l => ({ value: l, label: l.toUpperCase(), title: langName(l) }))} />}
      {sum && <button type="button" className="btn ghost sm icon-only" onClick={again} title="Write the summary again" aria-label="Write the summary again"><Icon name="refresh" size={14} /></button>}
    </>}>
      {sum ? (
        <div className="about-summary">
          <p>{sum.text}</p>
          <p className="muted small">Spoiler-free · written by AI{sum.source ? ` (${sum.source})` : ''} — may contain mistakes.</p>
        </div>
      ) : pending && <p className="muted small about-summary">A short spoiler-free summary is being written — it appears here shortly.</p>}
      {description && <Description text={description} />}
    </Card>
  )
}

function Description({ text }: { text: string }) {
  const [open, setOpen] = useState(text.length < 600)
  return (
    <div className={`description ${open ? '' : 'clamped'}`}>
      {text}
      {!open && <button type="button" className="btn ghost sm more-btn" onClick={() => setOpen(true)}>Read more</button>}
    </div>
  )
}
