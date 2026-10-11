import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Markdown } from './Markdown'

// A textarea for Markdown with a "/" command menu (headings, lists, quotes,
// callouts …), Ctrl/Cmd+B / I for bold and italic, and a Preview toggle.

interface Cmd { id: string; label: string; hint: string; keys: string; insert: string; cursor?: number }

// insert: text placed instead of "/query"; "|" marks where the cursor goes.
const CALLOUT_TYPES: [string, string, string][] = [
  ['info', 'ℹ', 'Info'], ['tip', '💡', 'Tip'], ['important', '❗', 'Important'], ['warning', '⚠', 'Warning'],
  ['success', '✓', 'Success'], ['danger', '⚡', 'Danger'], ['note', '✎', 'Note'], ['summary', '📋', 'Summary'], ['question', '?', 'Question'], ['quote', '❝', 'Quote'],
]
const COMMANDS: Cmd[] = [
  { id: 'h2', label: 'Heading', hint: '## ', keys: 'heading h2 title naslov', insert: '## |' },
  { id: 'h3', label: 'Sub-heading', hint: '### ', keys: 'subheading h3 podnaslov', insert: '### |' },
  { id: 'ul', label: 'Bullet list', hint: '- ', keys: 'bullet list ul lista', insert: '- |' },
  { id: 'ol', label: 'Numbered list', hint: '1. ', keys: 'numbered ordered list ol', insert: '1. |' },
  { id: 'three', label: 'Three points', hint: '1. 2. 3.', keys: 'three 3 points tri', insert: '1. |\n2. \n3. ' },
  { id: 'quote', label: 'Quote', hint: '> ', keys: 'quote blockquote citat', insert: '> |' },
  ...CALLOUT_TYPES.map(([t, icon, name]) => ({ id: 'co-' + t, label: `${icon} ${name} callout`, hint: `> [!${t}]`, keys: `callout box ${t} ${name.toLowerCase()}`, insert: `> [!${t}] ${name}\n> |` })),
  { id: 'fold', label: 'Foldable callout', hint: '> [!tip]-', keys: 'fold collapse details callout', insert: '> [!tip]- |\n> ' },
  { id: 'bold', label: 'Bold', hint: '**text**', keys: 'bold strong podebljano', insert: '**|**' },
  { id: 'italic', label: 'Italic', hint: '*text*', keys: 'italic emphasis kurziv', insert: '*|*' },
  { id: 'mark', label: 'Highlight', hint: '==text==', keys: 'highlight mark', insert: '==|==' },
  { id: 'link', label: 'Link', hint: '[text](https://…)', keys: 'link url', insert: '[|](https://)' },
  { id: 'code', label: 'Code block', hint: '```', keys: 'code block pre', insert: '```\n|\n```' },
  { id: 'hr', label: 'Divider', hint: '---', keys: 'divider hr rule line', insert: '---\n|' },
]

/** Pixel position of the caret inside a textarea (mirror-div technique). */
function caretXY(ta: HTMLTextAreaElement, pos: number): { x: number; y: number } {
  const div = document.createElement('div')
  const cs = getComputedStyle(ta)
  for (const p of ['fontFamily', 'fontSize', 'fontWeight', 'lineHeight', 'letterSpacing', 'padding', 'border', 'boxSizing', 'whiteSpace', 'wordWrap', 'tabSize'] as const) (div.style as unknown as Record<string, string>)[p] = (cs as unknown as Record<string, string>)[p]
  div.style.position = 'absolute'; div.style.visibility = 'hidden'; div.style.whiteSpace = 'pre-wrap'; div.style.width = ta.clientWidth + 'px'
  div.textContent = ta.value.slice(0, pos)
  const span = document.createElement('span'); span.textContent = '​'; div.appendChild(span)
  document.body.appendChild(div)
  const xy = { x: span.offsetLeft, y: span.offsetTop - ta.scrollTop + parseFloat(cs.lineHeight || '20') }
  document.body.removeChild(div)
  return xy
}

export function MarkdownEditor({ value, onChange, rows = 8, placeholder, className = '', autoFocus }: { value: string; onChange: (v: string) => void; rows?: number; placeholder?: string; className?: string; autoFocus?: boolean }) {
  const ta = useRef<HTMLTextAreaElement>(null)
  const [menu, setMenu] = useState<{ start: number; query: string; x: number; y: number } | null>(null)
  const [hl, setHl] = useState(0)
  const [preview, setPreview] = useState(false)
  const pending = useRef<number | null>(null)

  const matches = useMemo(() => {
    if (!menu) return []
    const q = menu.query.toLowerCase()
    return COMMANDS.filter(c => !q || c.label.toLowerCase().includes(q) || c.keys.includes(q))
  }, [menu])
  useEffect(() => { setHl(0) }, [menu?.query])
  useLayoutEffect(() => { // restore the caret after an insert
    if (pending.current !== null && ta.current) { ta.current.focus(); ta.current.setSelectionRange(pending.current, pending.current); pending.current = null }
  })

  // Open the menu when "/" starts a word; follow the query while it is typed.
  const detect = (v: string, pos: number) => {
    const before = v.slice(0, pos)
    const m = before.match(/(^|\s)\/([\w-]*)$/)
    if (!m || m[2].length > 20) { setMenu(null); return }
    const start = pos - m[2].length - 1
    const xy = ta.current ? caretXY(ta.current, start) : { x: 0, y: 0 }
    setMenu({ start, query: m[2], ...xy })
  }

  const apply = (c: Cmd) => {
    if (!menu || !ta.current) return
    const end = ta.current.selectionStart
    let ins = c.insert
    // Line-level blocks belong at the start of a line.
    const lineStart = value.lastIndexOf('\n', menu.start - 1) + 1
    const block = /^(#|-|1\.|>|```|---)/.test(ins)
    if (block && value.slice(lineStart, menu.start).trim()) ins = '\n' + ins
    const at = ins.indexOf('|')
    ins = ins.replace('|', '')
    onChange(value.slice(0, menu.start) + ins + value.slice(end))
    pending.current = menu.start + at
    setMenu(null)
  }

  const wrap = (mark: string) => {
    const t = ta.current; if (!t) return
    const { selectionStart: a, selectionEnd: b } = t
    const sel = value.slice(a, b)
    onChange(value.slice(0, a) + mark + sel + mark + value.slice(b))
    pending.current = sel ? b + mark.length * 2 : a + mark.length
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (menu && matches.length) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setHl(h => (h + 1) % matches.length); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); setHl(h => (h - 1 + matches.length) % matches.length); return }
      if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); apply(matches[hl]); return }
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); setMenu(null); return }
    }
    if ((e.metaKey || e.ctrlKey) && (e.key === 'b' || e.key === 'i')) { e.preventDefault(); wrap(e.key === 'b' ? '**' : '*'); return }
    // Enter inside a list or callout continues it; on an empty item it ends it.
    if (e.key === 'Enter' && !e.shiftKey && ta.current) {
      const pos = ta.current.selectionStart
      const ls = value.lastIndexOf('\n', pos - 1) + 1
      const line = value.slice(ls, pos)
      const m = line.match(/^(\s*)([-*+]|\d+[.)]|>)(\s+)(.*)$/)
      if (m) {
        e.preventDefault()
        if (!m[4].trim() && m[2] !== '>') { onChange(value.slice(0, ls) + value.slice(pos)); pending.current = ls; return }
        const next = /\d/.test(m[2]) ? `${parseInt(m[2]) + 1}${m[2].slice(-1)}` : m[2]
        const ins = '\n' + m[1] + next + m[3]
        onChange(value.slice(0, pos) + ins + value.slice(pos)); pending.current = pos + ins.length
      }
    }
  }

  return (
    <div className={`md-editor ${className}`}>
      <div className="md-tabs">
        <button type="button" className={!preview ? 'active' : ''} onClick={() => setPreview(false)}>Write</button>
        <button type="button" className={preview ? 'active' : ''} onClick={() => setPreview(true)}>Preview</button>
        <span className="spacer" />
        <span className="muted small">Type <kbd>/</kbd> for headings, lists, callouts…</span>
      </div>
      {preview ? <div className="md-preview">{value.trim() ? <Markdown text={value} /> : <p className="muted">Nothing to preview yet.</p>}</div> : (
        <div className="md-input">
          <textarea ref={ta} rows={rows} value={value} placeholder={placeholder} autoFocus={autoFocus} className={className}
            onChange={e => { onChange(e.target.value); detect(e.target.value, e.target.selectionStart) }}
            onKeyDown={onKeyDown} onClick={e => detect(value, (e.target as HTMLTextAreaElement).selectionStart)} onBlur={() => setTimeout(() => setMenu(null), 150)} />
          {menu && matches.length > 0 && (
            <div className="slash-menu" style={{ left: Math.min(menu.x, (ta.current?.clientWidth || 300) - 260), top: menu.y + 4 }} role="listbox">
              {matches.map((c, i) => (
                <div key={c.id} role="option" aria-selected={i === hl} className={`slash-opt ${i === hl ? 'hl' : ''}`} onMouseEnter={() => setHl(i)} onMouseDown={e => { e.preventDefault(); apply(c) }}>
                  <span>{c.label}</span><span className="muted small mono">{c.hint}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
