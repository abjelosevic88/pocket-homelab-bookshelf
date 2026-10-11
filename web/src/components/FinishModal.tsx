import { useState } from 'react'
import { api, V1 } from '../lib/api'
import type { Book } from '../lib/types'
import { todayISO } from '../lib/format'
import { Field, Modal, useToast } from './ui'
import { Stars } from './Stars'
import { Cover } from './Cover'

/** Finish (or give up on) a book: date, rating, a few words while it is fresh. */
export function FinishModal({ book, abandon = false, onClose, onDone }: { book: Book; abandon?: boolean; onClose: () => void; onDone: (b: Book) => void }) {
  const { toast } = useToast()
  const [date, setDate] = useState(todayISO())
  const [rating, setRating] = useState(book.rating)
  const [review, setReview] = useState(book.review)
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const b = await api.post<Book>(`${V1}/books/${book.id}/status`, { status: abandon ? 'abandoned' : 'read', date, rating, review })
      toast(abandon ? 'Moved to abandoned' : `Finished “${book.title}” 🎉`)
      onDone(b)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={abandon ? 'Stop reading' : 'Finished!'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <div className="row" style={{ gap: 12, alignItems: 'flex-start' }}>
          <Cover book={book} size="xs" />
          <div><b>{book.title}</b><div className="muted small">{book.authors}</div></div>
        </div>
        <div className="two-col tight">
          <Field label={abandon ? 'Stopped on' : 'Finished on'}><input type="date" value={date} max={todayISO()} onChange={e => setDate(e.target.value)} required /></Field>
          <Field group label="Rating"><Stars value={rating} onChange={setRating} /></Field>
        </div>
        <Field label={abandon ? 'Why did you stop?' : 'Your thoughts'} help="Optional — you can always come back to it on the book's page.">
          <textarea rows={6} value={review} onChange={e => setReview(e.target.value)} placeholder={abandon ? 'Not for me because…' : 'What stayed with you? Would you recommend it?'} autoFocus />
        </Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{abandon ? 'Abandon' : 'Save'}</button></div>
      </form>
    </Modal>
  )
}
