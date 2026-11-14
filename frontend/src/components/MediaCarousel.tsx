import { useEffect, useMemo, useState } from 'react'
import type { JSX } from 'react'
import { mediaKindLabels, orderMedia } from '../lib/metadata'
import type { MediaItemView } from '../lib/metadata'
import { coverPlaceholderUrl } from './CoverImage'
import { MediaThumbStrip } from './MediaThumbStrip'

// MediaCarousel shows one featured image (covers first) plus a thumbnail strip
// [AJU-009..013, UIX-026, UIX-042, UIX-043].
export function MediaCarousel({ items, resetKey }: { items?: MediaItemView[]; resetKey: number }): JSX.Element {
  const ordered = useMemo(() => orderMedia(items ?? []), [items])
  const n = ordered.length
  const [index, setIndex] = useState(0)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setIndex(0)
  }, [resetKey, items])

  useEffect(() => {
    setFailed(false)
  }, [index, resetKey, items])

  const safe = n > 0 ? Math.min(index, n - 1) : 0
  const current = n > 0 ? ordered[safe] : undefined
  const src = current && !failed ? current.url : coverPlaceholderUrl

  return (
    <section
      role="region"
      aria-label="Imagens"
      aria-roledescription="carrossel"
      className="flex flex-col gap-2 rounded border border-line bg-surface p-2"
    >
      <div className="flex aspect-video w-full items-center justify-center overflow-hidden rounded bg-raised">
        <img src={src} alt="" onError={() => setFailed(true)} className="h-full w-full object-contain" />
      </div>
      {current && (
        <div className="flex items-center justify-between gap-2 text-sm text-muted">
          <span>{mediaKindLabels[current.kind]}</span>
          <span aria-live="polite">{`${safe + 1} de ${n}`}</span>
        </div>
      )}
      {n >= 2 && <MediaThumbStrip key={resetKey} items={ordered} index={safe} onSelect={setIndex} />}
    </section>
  )
}
