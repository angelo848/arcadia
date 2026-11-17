import { useEffect, useRef, useState } from 'react'
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

  // Keep the highlighted thumbnail visible when it moves because the list changed [IMG-009].
  const last = useRef<{ id: number; index: number } | null>(null)
  useEffect(() => {
    const id = items[index]?.id
    const prev = last.current
    if (id !== undefined) {
      if (prev && prev.id === id && prev.index !== index) {
        refs.current[index]?.scrollIntoView?.({ inline: 'nearest', block: 'nearest' })
      }
      last.current = { id, index }
    }
  })

  const select = (i: number) => {
    onSelect(i)
    const el = refs.current[i]
    if (el) {
      el.focus()
      el.scrollIntoView?.({ inline: 'nearest', block: 'nearest' })
    }
  }

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    const target = (e.target as HTMLElement).closest?.('button')
    const found = target ? refs.current.indexOf(target as HTMLButtonElement) : -1
    const from = found >= 0 ? found : index
    let next = from
    if (e.key === 'ArrowRight') next = from + 1
    else if (e.key === 'ArrowLeft') next = from - 1
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = n - 1
    else return
    if (next < 0 || next >= n || next === from) return
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
            aria-label={`${label}, ${i + 1} de ${n}`}
            aria-current={current ? 'true' : undefined}
            tabIndex={current ? 0 : -1}
            onClick={() => {
              if (!current) select(i)
            }}
            className={`h-[54px] w-24 shrink-0 overflow-hidden rounded bg-raised ${current ? 'ring-2 ring-accent' : ''}`}
          >
            {it.kind === 'video' && !it.thumbUrl ? (
              <span className="relative block h-full w-full">
                {/* No stored thumbnail: the browser draws a frame of the video itself. */}
                <video
                  src={`${it.url}#t=1`}
                  preload="metadata"
                  muted
                  playsInline
                  tabIndex={-1}
                  className="pointer-events-none h-full w-full object-cover"
                />
                <span aria-hidden="true" className="absolute inset-0 flex items-center justify-center text-xl text-fg">
                  ▶
                </span>
              </span>
            ) : (
              <img
                src={src}
                alt=""
                onError={() => setFailed((s) => new Set(s).add(it.id))}
                className="h-full w-full object-cover"
              />
            )}
          </button>
        )
      })}
    </div>
  )
}
