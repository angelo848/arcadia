import { useRef, useState } from 'react'
import type { JSX, KeyboardEvent } from 'react'
import { GRID_INSET } from '../lib/gridNav'
import { mediaKindLabels } from '../lib/metadata'
import type { MediaItemView } from '../lib/metadata'
import { coverPlaceholderUrl } from './CoverImage'

// MediaThumbStrip is the row of thumbnails under the carousel image [AJU-010..012].
export function MediaThumbStrip({
  items,
  index,
  onSelect,
}: {
  items: MediaItemView[]
  index: number
  onSelect: (i: number) => void
}): JSX.Element {
  const n = items.length
  const refs = useRef<Array<HTMLButtonElement | null>>([])
  const [failed, setFailed] = useState<Set<number>>(() => new Set())

  const select = (i: number) => {
    onSelect(i)
    const el = refs.current[i]
    if (el) {
      el.focus()
      el.scrollIntoView?.({ inline: 'nearest', block: 'nearest' })
    }
  }

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    let next = index
    if (e.key === 'ArrowRight') next = index + 1
    else if (e.key === 'ArrowLeft') next = index - 1
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = n - 1
    else return
    if (next < 0 || next >= n || next === index) return
    e.preventDefault()
    select(next)
  }

  return (
    <div
      role="group"
      aria-label="Miniaturas"
      onKeyDown={onKeyDown}
      className="flex gap-2 overflow-x-auto"
      style={{ padding: GRID_INSET }}
    >
      {items.map((it, i) => {
        const label = mediaKindLabels[it.kind]
        const current = i === index
        const src = failed.has(it.id) ? coverPlaceholderUrl : it.thumbUrl || it.url
        return (
          <button
            key={it.id}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            data-bigbox-item
            aria-label={`${label}, ${i + 1} de ${n}`}
            aria-current={current ? 'true' : undefined}
            tabIndex={current ? 0 : -1}
            onClick={() => {
              if (!current) select(i)
            }}
            className={`h-[54px] w-24 shrink-0 overflow-hidden rounded bg-raised ${current ? 'ring-2 ring-accent' : ''}`}
          >
            <img
              src={src}
              alt=""
              onError={() => setFailed((s) => new Set(s).add(it.id))}
              className="h-full w-full object-cover"
            />
          </button>
        )
      })}
    </div>
  )
}
