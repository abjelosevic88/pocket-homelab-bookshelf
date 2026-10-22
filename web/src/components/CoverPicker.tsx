import { useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import type { Book } from '../lib/types'
import { langName } from '../lib/languages'
import { Field, Loading, Modal, useAsync, useToast } from './ui'
import { Cover } from './Cover'
import { Icon } from './icons'

interface CoverOption { url: string; thumb: string; source: string; label: string; language?: string }

/** Change a book's cover: pick one of its editions' covers, upload a file or paste an address. */
export function CoverPicker({ book, onClose, onChanged }: { book: Book; onClose: () => void; onChanged: (b: Book) => void }) {
  const { toast } = useToast()
  const { data, loading } = useAsync(() => api.get<{ covers: CoverOption[] }>(`${V1}/books/${book.id}/covers`), [book.id])
  const [busy, setBusy] = useState('')
  const [url, setUrl] = useState('')
  const [broken, setBroken] = useState<Set<string>>(new Set())
  const [aiState, setAiState] = useState<'' | 'busy' | 'done'>('')
  const [aiCovers, setAiCovers] = useState<CoverOption[]>([])
  const [aiNote, setAiNote] = useState('')

  const askAI = async () => {
    setAiState('busy'); setAiNote('')
    try {
      const r = await api.get<{ covers: CoverOption[]; hints: { original_title: string; isbns: string[] } }>(`${V1}/books/${book.id}/covers/ai`)
      setAiCovers(r.covers)
      const facts = [r.hints.original_title && r.hints.original_title !== book.title && `original: “${r.hints.original_title}”`, r.hints.isbns.length > 0 && `${r.hints.isbns.length} ISBN${r.hints.isbns.length > 1 ? 's' : ''}`].filter(Boolean).join(' · ')
      setAiNote(r.covers.length ? `${r.covers.length} covers checked and ready${facts ? ` — ${facts}` : ''}.` : `Pocket AI found nothing that downloads${facts ? ` (${facts})` : ''}. Upload one or paste an address above.`)
      setAiState('done')
    } catch (e) { setAiNote((e as Error).message); setAiState('done') }
  }

  const use = async (u: string) => {
    setBusy(u)
    try { const b = await api.post<Book>(`${V1}/books/${book.id}/cover`, { url: u }); toast('Cover changed'); onChanged(b); onClose() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const fromFile = async (f: File) => {
    setBusy('file')
    try { const b = await upload<Book>(`${V1}/books/${book.id}/cover`, 'cover', f); toast('Cover uploaded'); onChanged(b); onClose() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const remove = async () => { await api.del(`${V1}/books/${book.id}/cover`); toast('Cover removed'); onChanged({ ...book, cover: '' }); onClose() }

  const covers = (data?.covers || []).filter(c => !broken.has(c.thumb))
  return (
    <Modal title="Change cover" onClose={onClose} size="lg">
      <div className="cover-picker-head">
        <Cover book={book} size="sm" />
        <div className="grid" style={{ gap: 10, flex: 1 }}>
          <div className="row wrap">
            <label className="btn primary">{busy === 'file' ? 'Uploading…' : 'Upload a picture…'}<input type="file" accept="image/*" hidden onChange={e => e.target.files?.[0] && fromFile(e.target.files[0])} /></label>
            {book.cover && <button type="button" className="btn ghost danger-text" onClick={remove}>Remove cover</button>}
          </div>
          <form className="row" onSubmit={e => { e.preventDefault(); if (url) use(url) }}>
            <input value={url} onChange={e => setUrl(e.target.value.trim())} placeholder="…or paste an image address (https://…)" />
            <button type="submit" className="btn" disabled={!url || busy !== ''}>Use</button>
          </form>
        </div>
      </div>
      <div className="ai-covers mt">
        <div className="row wrap">
          <div className="grow">
            <h3>Find with Pocket AI</h3>
            <p className="muted small">For translations and local editions the catalogues miss. Pocket AI names the edition, then each cover is downloaded to check it before it is shown. Takes up to a minute.</p>
          </div>
          <button type="button" className="btn" onClick={askAI} disabled={aiState === 'busy' || busy !== ''}><Icon name="sparkles" size={14} />{aiState === 'busy' ? 'Searching…' : aiState === 'done' ? 'Search again' : 'Find covers'}</button>
        </div>
        {aiState === 'busy' && <Loading label="Asking Pocket AI and checking covers…" />}
        {aiNote && aiState === 'done' && <p className={`small ${aiCovers.length ? 'muted' : 'danger-text'}`}>{aiNote}</p>}
        {aiCovers.length > 0 && (
          <div className="cover-grid mt-sm">
            {aiCovers.map(c => (
              <button key={c.url} type="button" className={`cover-option ${busy === c.url ? 'busy' : ''}`} onClick={() => use(c.url)} disabled={busy !== ''} title={c.label}>
                <img src={c.thumb} alt="" loading="lazy" referrerPolicy="no-referrer" />
                <span className="cap">{c.language ? <b>{langName(c.language)}</b> : null}{c.language && c.label ? ' · ' : ''}{c.label}</span>
                {busy === c.url && <span className="spinner sm" />}
              </button>
            ))}
          </div>
        )}
      </div>
      <h3 className="mt mb-sm">Covers of other editions</h3>
      {loading ? <Loading /> : covers.length === 0 ? <p className="muted">No other covers found. Upload one or paste an address above.</p> : (
        <>
          <div className="cover-grid">
            {covers.map(c => (
              <button key={c.thumb} type="button" className={`cover-option ${busy === c.url ? 'busy' : ''}`} onClick={() => use(c.url)} disabled={busy !== ''} title={[c.language && langName(c.language), c.label].filter(Boolean).join(' · ')}>
                <img src={c.thumb} alt="" loading="lazy" referrerPolicy="no-referrer"
                  onLoad={e => { const i = e.currentTarget; if (i.naturalWidth < 20) setBroken(s => new Set(s).add(c.thumb)) }}
                  onError={() => setBroken(s => new Set(s).add(c.thumb))} />
                <span className="cap">{c.language ? <b>{langName(c.language)}</b> : null}{c.language && c.label ? ' · ' : ''}{c.label}</span>
                {busy === c.url && <span className="spinner sm" />}
              </button>
            ))}
          </div>
          <p className="muted small mt">{covers.length} covers from Open Library{covers.some(c => c.source === 'google') ? ' and Google Books' : ''}; your preferred languages first. The chosen one is downloaded and kept with your data.</p>
        </>
      )}
    </Modal>
  )
}
