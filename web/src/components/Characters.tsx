import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { langName } from '../lib/languages'
import { Card, Segmented, useAsync, useToast } from './ui'
import { Icon } from './icons'

interface Character { name: string; role: 'main' | 'supporting'; description: string }
interface CharacterSet { status: 'ok' | 'none' | 'unknown'; characters: Character[]; source: string; updated_at: string }
interface Data { langs: string[]; sets: Record<string, CharacterSet>; writer?: 'app' | 'external' }

// Role labels in the language being read; others fall back to English.
const ROLE: Record<string, Record<string, string>> = {
  en: { main: 'Main', supporting: 'Supporting' },
  sr: { main: 'Glavni', supporting: 'Sporedni' }, hr: { main: 'Glavni', supporting: 'Sporedni' }, bs: { main: 'Glavni', supporting: 'Sporedni' },
}
const LANG_KEY = 'pb.characterLang'
const SHOWN = 8

/** Characters of a book with a short spoiler-free note on each, in every
 *  preferred language (written by the character writer, see scripts/characters.py). */
export function CharactersCard({ bookId }: { bookId: number }) {
  const { toast } = useToast()
  const { data, setData, reload } = useAsync(() => api.get<Data>(`${V1}/books/${bookId}/characters`), [bookId])
  const [lang, setLang] = useState(() => localStorage.getItem(LANG_KEY) || '')
  const [all, setAll] = useState(false)
  // While the app is writing them (it has an OpenAI key), look again every few seconds.
  const pending = !!data && data.writer === 'app' && data.langs.some(l => !data.sets[l])
  useEffect(() => {
    if (!pending) return
    const t = setInterval(reload, 5000)
    const stop = setTimeout(() => clearInterval(t), 5 * 60_000)
    return () => { clearInterval(t); clearTimeout(stop) }
  }, [pending]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!data) return null
  const langs = data.langs.filter(l => data.sets[l]?.status === 'ok' && data.sets[l].characters.length > 0)
  const current = langs.includes(lang) ? lang : langs[0]
  const set = current ? data.sets[current] : Object.values(data.sets)[0]
  const pick = (l: string) => { setLang(l); localStorage.setItem(LANG_KEY, l) }
  const again = async () => {
    try { await api.del(`${V1}/books/${bookId}/characters`); setData({ ...data, sets: {} }); toast(data.writer === 'app' ? 'Writing the characters again…' : 'Characters will be written again shortly') } catch (e) { toast((e as Error).message, 'err') }
  }

  if (!set) {
    return <Card className="mt" title="Characters"><p className="muted small">{data.writer === 'app' ? 'Being written — they appear here in a minute or two.' : 'Being written — they appear here within about half an hour.'}</p></Card>
  }
  if (set.status === 'none') return null // non-fiction: nothing to show
  if (set.status !== 'ok' || !current) {
    return (
      <Card className="mt" title="Characters" actions={<button type="button" className="btn ghost sm" onClick={again}>Try again</button>}>
        <p className="muted small">This book is not known well enough to list its characters reliably.</p>
      </Card>
    )
  }
  const chars = [...set.characters].sort((a, b) => (a.role === 'main' ? 0 : 1) - (b.role === 'main' ? 0 : 1))
  const shown = all ? chars : chars.slice(0, SHOWN)
  const roles = ROLE[current] || ROLE.en
  return (
    <Card className="mt" title={`Characters (${chars.length})`} actions={<>
      {langs.length > 1 && <Segmented size="sm" value={current} onChange={pick} options={langs.map(l => ({ value: l, label: l.toUpperCase(), title: langName(l) }))} />}
      <button type="button" className="btn ghost sm icon-only" onClick={again} title="Write the characters again" aria-label="Write the characters again"><Icon name="refresh" size={14} /></button>
    </>}>
      <ul className="characters">
        {shown.map(c => (
          <li key={c.name}>
            <div className="ch-head"><b>{c.name}</b>{c.role === 'main' && <span className="badge cat">{roles.main}</span>}</div>
            <p>{c.description}</p>
          </li>
        ))}
      </ul>
      {chars.length > SHOWN && <button type="button" className="btn ghost sm mt-sm" onClick={() => setAll(!all)}>{all ? 'Show fewer' : `Show all ${chars.length}`}</button>}
      <p className="muted small mt-sm">Spoiler-free · written by AI{set.source ? ` (${set.source})` : ''} — may contain mistakes.</p>
    </Card>
  )
}
