import React from 'react'

// A small Markdown renderer for thoughts and notes, built from React elements
// (no raw HTML, so nothing in the text can inject markup). It covers what
// reading notes use: headings, paragraphs, **bold**, *italic*, ==highlight==,
// ~~strike~~, `code`, links, nested lists, block quotes, code blocks, rules,
// and Obsidian callouts — "> [!tip] Title" and Admonition's ```ad-tip blocks.

const CALLOUTS: Record<string, { icon: string; cls: string }> = {
  note: { icon: '✎', cls: 'blue' }, info: { icon: 'ℹ', cls: 'blue' }, todo: { icon: '☐', cls: 'blue' },
  tip: { icon: '💡', cls: 'teal' }, hint: { icon: '💡', cls: 'teal' }, important: { icon: '❗', cls: 'purple' },
  success: { icon: '✓', cls: 'green' }, check: { icon: '✓', cls: 'green' }, done: { icon: '✓', cls: 'green' },
  question: { icon: '?', cls: 'amber' }, help: { icon: '?', cls: 'amber' }, faq: { icon: '?', cls: 'amber' },
  warning: { icon: '⚠', cls: 'amber' }, caution: { icon: '⚠', cls: 'amber' }, attention: { icon: '⚠', cls: 'amber' },
  failure: { icon: '✕', cls: 'red' }, fail: { icon: '✕', cls: 'red' }, missing: { icon: '✕', cls: 'red' },
  danger: { icon: '⚡', cls: 'red' }, error: { icon: '⚡', cls: 'red' }, bug: { icon: '🐞', cls: 'red' },
  example: { icon: '☰', cls: 'purple' }, quote: { icon: '❝', cls: 'gray' }, cite: { icon: '❝', cls: 'gray' },
  summary: { icon: '📋', cls: 'teal' }, abstract: { icon: '📋', cls: 'teal' }, tldr: { icon: '📋', cls: 'teal' },
}

// ---------- inline ----------

const INLINE = /(`[^`\n]+`)|(\*\*[^*\n]+?\*\*|__[^_\n]+?__)|(==[^=\n]+?==)|(~~[^~\n]+?~~)|(\[\[[^\]\n]+\]\])|(\[[^\]\n]+\]\((?:https?:\/\/|mailto:)[^)\s]+\))|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])|((?<![\w*])\*(?!\s)[^*\n]+?\*(?![\w*])|(?<![\w_])_(?!\s)[^_\n]+?_(?![\w_]))/

export function Inline({ text }: { text: string }): React.ReactElement {
  const out: React.ReactNode[] = []
  let rest = text
  let k = 0
  while (rest) {
    const m = INLINE.exec(rest)
    if (!m) { out.push(rest); break }
    if (m.index > 0) out.push(rest.slice(0, m.index))
    const t = m[0]
    if (m[1]) out.push(<code key={k++}>{t.slice(1, -1)}</code>)
    else if (m[2]) out.push(<strong key={k++}><Inline text={t.slice(2, -2)} /></strong>)
    else if (m[3]) out.push(<mark key={k++}><Inline text={t.slice(2, -2)} /></mark>)
    else if (m[4]) out.push(<del key={k++}><Inline text={t.slice(2, -2)} /></del>)
    else if (m[5]) { // [[Note|shown]] or [[folder/Note#heading]] → its name
      const inner = t.slice(2, -2)
      out.push(inner.includes('|') ? inner.split('|').pop() : inner.split('#')[0].split('/').pop())
    } else if (m[6]) {
      const mm = t.match(/^\[([^\]]+)\]\(([^)]+)\)$/)!
      out.push(<a key={k++} href={mm[2]} target="_blank" rel="noreferrer"><Inline text={mm[1]} /></a>)
    } else if (m[7]) out.push(<a key={k++} href={t} target="_blank" rel="noreferrer">{t.replace(/^https?:\/\/(www\.)?/, '').slice(0, 60)}</a>)
    else if (m[8]) out.push(<em key={k++}><Inline text={t.slice(1, -1)} /></em>)
    rest = rest.slice(m.index + t.length)
  }
  return <>{out}</>
}

// ---------- blocks ----------

type Block =
  | { t: 'h'; level: number; text: string }
  | { t: 'p'; lines: string[] }
  | { t: 'code'; lang: string; text: string }
  | { t: 'callout'; kind: string; title: string; fold: '' | '+' | '-'; body: string }
  | { t: 'quote'; body: string }
  | { t: 'list'; items: ListItem[]; ordered: boolean; start: number }
  | { t: 'hr' }

interface ListItem { text: string; children: string[] }

const LIST_RE = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/
const indentOf = (s: string) => s.replace(/\t/g, '    ').length

function parseBlocks(src: string): Block[] {
  const lines = src.replace(/\r\n/g, '\n').split('\n')
  const blocks: Block[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (!line.trim()) { i++; continue }
    // fenced code / admonition
    const fence = line.match(/^\s*(```+|~~~+)\s*([\w-]*)(.*)$/)
    if (fence) {
      const close = fence[1]
      const body: string[] = []
      i++
      while (i < lines.length && !lines[i].trim().startsWith(close)) body.push(lines[i++])
      i++
      const lang = fence[2].toLowerCase()
      if (lang.startsWith('ad-')) {
        let title = ''
        const rest = body.filter(l => { const m = l.match(/^title:\s*(.*)$/); if (m) { title = m[1]; return false } return !/^(collapse|icon|color):/.test(l) })
        blocks.push({ t: 'callout', kind: lang.slice(3), title, fold: '', body: rest.join('\n') })
      } else if (lang !== 'dataview' && lang !== 'dataviewjs') {
        blocks.push({ t: 'code', lang, text: body.join('\n') })
      }
      continue
    }
    const h = line.match(/^(#{1,6})\s+(.*)$/)
    if (h) { blocks.push({ t: 'h', level: h[1].length, text: h[2] }); i++; continue }
    if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) { blocks.push({ t: 'hr' }); i++; continue }
    if (/^\s*>/.test(line)) {
      const body: string[] = []
      while (i < lines.length && /^\s*>/.test(lines[i])) body.push(lines[i++].replace(/^\s*>\s?/, ''))
      const c = body[0]?.match(/^\[!(\w+)\]([+-]?)\s*(.*)$/)
      if (c) blocks.push({ t: 'callout', kind: c[1].toLowerCase(), fold: c[2] as '' | '+' | '-', title: c[3], body: body.slice(1).join('\n') })
      else blocks.push({ t: 'quote', body: body.join('\n') })
      continue
    }
    const li = line.match(LIST_RE)
    if (li) {
      const base = indentOf(li[1])
      const ordered = /\d/.test(li[2])
      const items: ListItem[] = []
      while (i < lines.length) {
        const l = lines[i]
        if (!l.trim()) { // a blank line ends the list unless it continues indented
          const next = lines[i + 1]
          if (next && (LIST_RE.test(next) && indentOf(next.match(LIST_RE)![1]) >= base || indentOf(next.match(/^\s*/)![0]) > base && next.trim())) { i++; continue }
          break
        }
        const m = l.match(LIST_RE)
        const ind = indentOf(l.match(/^\s*/)![0])
        if (m && ind <= base + 1) { items.push({ text: m[3], children: [] }); i++; continue }
        if (items.length && ind > base) { items[items.length - 1].children.push(l); i++; continue }
        break
      }
      blocks.push({ t: 'list', items, ordered, start: ordered ? parseInt(li[2]) || 1 : 1 })
      continue
    }
    // paragraph: until a blank line or another block starts
    const para: string[] = []
    while (i < lines.length && lines[i].trim() && !/^\s*(```|~~~|#{1,6}\s|>)/.test(lines[i]) && !(LIST_RE.test(lines[i]) && para.length === 0)) {
      if (para.length && LIST_RE.test(lines[i])) break
      para.push(lines[i++])
    }
    if (para.length) blocks.push({ t: 'p', lines: para })
    else i++
  }
  return blocks
}

function dedent(lines: string[]): string {
  const ind = Math.min(...lines.filter(l => l.trim()).map(l => indentOf(l.match(/^\s*/)![0])))
  return lines.map(l => l.replace(/\t/g, '    ').slice(Number.isFinite(ind) ? ind : 0)).join('\n')
}

function Callout({ kind, title, fold, body }: { kind: string; title: string; fold: string; body: string }) {
  const c = CALLOUTS[kind] || CALLOUTS.note
  const head = <><span className="co-icon">{c.icon}</span><span className="co-title"><Inline text={title || kind.charAt(0).toUpperCase() + kind.slice(1)} /></span></>
  const inner = body.trim() ? <div className="co-body"><Markdown text={body} /></div> : null
  if (fold) return <details className={`callout-box ${c.cls}`} open={fold === '+'}><summary className="co-head">{head}</summary>{inner}</details>
  return <div className={`callout-box ${c.cls}`}><div className="co-head">{head}</div>{inner}</div>
}

export function Markdown({ text }: { text: string }) {
  const blocks = parseBlocks(text)
  return <div className="md">{blocks.map((b, i) => {
    switch (b.t) {
      case 'h': return React.createElement(`h${Math.min(6, b.level + 2)}`, { key: i, className: 'md-h' }, <Inline text={b.text} />)
      case 'p': return <p key={i}>{b.lines.map((l, j) => <React.Fragment key={j}>{j > 0 && <br />}<Inline text={l.trim()} /></React.Fragment>)}</p>
      case 'code': return <pre key={i} className="md-code"><code>{b.text}</code></pre>
      case 'hr': return <hr key={i} />
      case 'quote': return <blockquote key={i}><Markdown text={b.body} /></blockquote>
      case 'callout': return <Callout key={i} kind={b.kind} title={b.title} fold={b.fold} body={b.body} />
      case 'list': {
        const items = b.items.map((it, j) => <li key={j}><Inline text={it.text} />{it.children.length > 0 && <Markdown text={dedent(it.children)} />}</li>)
        return b.ordered ? <ol key={i} start={b.start}>{items}</ol> : <ul key={i}>{items}</ul>
      }
    }
  })}</div>
}
