import { useEffect, useMemo, useState } from 'react'
import type { JSX, KeyboardEvent } from 'react'
import { mediaKindLabels, orderMedia } from '../lib/metadata'
import type { MediaItemView } from '../lib/metadata'
import { coverPlaceholderUrl } from './CoverImage'

// MediaCarousel shows one image of the game at a time, covers first [UIX-026], with
// keyboard-reachable buttons and arrow keys [UIX-042, UIX-043].
export function MediaCarousel({ items, resetKey }: { items?: MediaItemView[]; resetKey: number }): JSX.Element {
  const ordered = useMemo(() => orderMedia(items ?? []), [items])
  const n = ordered.length
  const [index, setIndex] = useState(0)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setIndex(0)
  }, [resetKey])

  useEffect(() => {
    setFailed(false)
  }, [index, resetKey, items])

  const move = (delta: number) => setIndex((i) => (n === 0 ? 0 : (i + delta + n) % n))
  const current = n > 0 ? ordered[Math.min(index, n - 1)] : undefined
  const src = current && !failed ? current.url : coverPlaceholderUrl

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (n < 2) return
    if (e.key === 'ArrowRight') {
      e.preventDefault()
      move(1)
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault()
      move(-1)
    }
  }

  const btn = 'rounded bg-raised px-3 py-1 text-fg hover:bg-hover'

  return (
    <section
      role="region"
      aria-label="Imagens"
      aria-roledescription="carrossel"
      onKeyDown={onKeyDown}
      className="flex flex-col gap-2 rounded border border-line bg-surface p-2"
    >
      <div className="flex h-64 items-center justify-center overflow-hidden rounded bg-raised">
        <img src={src} alt="" onError={() => setFailed(true)} className="h-full w-full object-contain" />
      </div>
      {current && (
        <div className="flex items-center justify-between gap-2 text-sm text-muted">
          {n > 1 && (
            <button type="button" className={btn} aria-label="Imagem anterior" onClick={() => move(-1)}>
              ‹
            </button>
          )}
          <span>{mediaKindLabels[current.kind]}</span>
          <span aria-live="polite">{`${Math.min(index, n - 1) + 1} de ${n}`}</span>
          {n > 1 && (
            <button type="button" className={btn} aria-label="Próxima imagem" onClick={() => move(1)}>
              ›
            </button>
          )}
        </div>
      )}
    </section>
  )
}
