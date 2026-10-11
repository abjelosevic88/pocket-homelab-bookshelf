import { useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import type { Author } from '../lib/types'
import { Loading, Modal, useAsync, useToast } from './ui'
import { AuthorPhoto } from './AuthorBits'

interface PhotoOption { url: string; thumb: string; source: string; label: string }

/** Change an author's photo: pick one of the suggested pictures, upload a file or paste an address. */
export function AuthorPhotoPicker({ author, onClose, onChanged }: { author: Author; onClose: () => void; onChanged: () => void }) {
  const { toast } = useToast()
  const { data, loading } = useAsync(() => api.get<{ photos: PhotoOption[] }>(`${V1}/authors/${author.id}/photos`), [author.id])
  const [busy, setBusy] = useState('')
  const [url, setUrl] = useState('')
  const [broken, setBroken] = useState<Set<string>>(new Set())

  const use = async (u: string) => {
    setBusy(u)
    try { await api.post(`${V1}/authors/${author.id}/photo`, { url: u }); toast('Photo changed'); onChanged(); onClose() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const fromFile = async (f: File) => {
    setBusy('file')
    try { await upload(`${V1}/authors/${author.id}/photo`, 'photo', f); toast('Photo uploaded'); onChanged(); onClose() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const remove = async () => { await api.del(`${V1}/authors/${author.id}/photo`); toast('Photo removed'); onChanged(); onClose() }

  const photos = (data?.photos || []).filter(p => !broken.has(p.thumb))
  return (
    <Modal title={`Photo of ${author.name}`} onClose={onClose} size="lg">
      <div className="cover-picker-head">
        <AuthorPhoto author={author} size="lg" />
        <div className="grid" style={{ gap: 10, flex: 1 }}>
          <div className="row wrap">
            <label className="btn primary">{busy === 'file' ? 'Uploading…' : 'Upload a picture…'}<input type="file" accept="image/*" hidden onChange={e => e.target.files?.[0] && fromFile(e.target.files[0])} /></label>
            {author.photo && <button className="btn ghost danger-text" onClick={remove}>Remove photo</button>}
          </div>
          <form className="row" onSubmit={e => { e.preventDefault(); if (url) use(url) }}>
            <input value={url} onChange={e => setUrl(e.target.value.trim())} placeholder="…or paste an image address (https://…)" />
            <button className="btn" disabled={!url || busy !== ''}>Use</button>
          </form>
        </div>
      </div>
      <h3 className="mt mb-sm">Suggested pictures</h3>
      {loading ? <Loading /> : photos.length === 0 ? <p className="muted">Nothing found{author.ol_key ? '' : ' (the author is not linked to Open Library — use “Wrong person?” on their page)'}. Upload one or paste an address above.</p> : (
        <>
          <div className="photo-groups">
            {Array.from(new Set(photos.map(p => p.label))).map(label => (
              <section key={label}>
                <div className="photo-group">{label}</div>
                <div className="photo-grid">
                  {photos.filter(p => p.label === label).map(p => (
                    <button key={p.thumb} type="button" className={`photo-option ${busy === p.url ? 'busy' : ''}`} onClick={() => use(p.url)} disabled={busy !== ''} title={p.label}>
                      <img src={p.thumb} alt="" loading="lazy" referrerPolicy="no-referrer"
                        onLoad={e => { if (e.currentTarget.naturalWidth < 20) setBroken(s => new Set(s).add(p.thumb)) }}
                        onError={() => setBroken(s => new Set(s).add(p.thumb))} />
                      {busy === p.url && <span className="spinner sm" />}
                    </button>
                  ))}
                </div>
              </section>
            ))}
          </div>
          <p className="muted small mt">From Open Library, Wikidata and Wikipedia — portraits first. The chosen picture is downloaded and kept with your data.</p>
        </>
      )}
    </Modal>
  )
}
