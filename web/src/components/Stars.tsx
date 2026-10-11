import { useState } from 'react'

/** Five stars in half steps. With onChange it is an input: click the left or
 *  right half of a star; clicking the current value clears it. */
export function Stars({ value, onChange, size = '' }: { value: number; onChange?: (v: number) => void; size?: '' | 'sm' | 'lg' }) {
  const [hover, setHover] = useState<number | null>(null)
  const shown = hover ?? value
  const label = value ? `${value} of 5 stars` : 'Not rated'
  return (
    <span className={`stars ${size} ${onChange ? 'edit' : ''}`} onMouseLeave={() => setHover(null)} title={label} aria-label={label} role={onChange ? 'group' : 'img'}>
      {[1, 2, 3, 4, 5].map(i => {
        const fill = shown >= i ? 100 : shown >= i - 0.5 ? 50 : 0
        return (
          <span key={i} className="star">
            <span className="bg">★</span>
            <span className="fg" style={{ width: `${fill}%` }}>★</span>
            {onChange && [i - 0.5, i].map((v, k) => (
              <button key={k} type="button" className={`half ${k ? 'r' : 'l'}`} aria-label={`${v} stars`} onMouseEnter={() => setHover(v)} onFocus={() => setHover(v)} onBlur={() => setHover(null)}
                onClick={() => { setHover(null); onChange(v === value ? 0 : v) }} />
            ))}
          </span>
        )
      })}
      {onChange && <span className="stars-val">{shown ? shown.toFixed(1) : '–'}</span>}
    </span>
  )
}
