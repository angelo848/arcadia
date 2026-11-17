import { useEffect, useMemo, useState } from 'react'
import type { JSX } from 'react'
import { mediaKindLabels, orderMedia } from '../lib/metadata'
import type { MediaItemView } from '../lib/metadata'
import { coverPlaceholderUrl } from './CoverImage'
import { MediaLightbox } from './MediaLightbox'
import { MediaThumbStrip } from './MediaThumbStrip'

// MediaCarousel shows one featured image (covers first) plus a thumbnail strip
// [AJU-009..013, UIX-026, UIX-042, UIX-043].
export function MediaCarousel({ items, resetKey }: { items?: MediaItemView[]; resetKey: number }): JSX.Element {
  const ordered = useMemo(() => orderMedia(items ?? []), [items])
  const n = ordered.length
  const [highlightId, setHighlightId] = useState<number | undefined>(undefined)
  const [seenReset, setSeenReset] = useState(resetKey)
  const [failed, setFailed] = useState(false)
  const [zoomed, setZoomed] = useState(false)

  // Pin the highlight by id so new images never move it [IMG-009].
  let hid = seenReset === resetKey ? highlightId : undefined
  if (n > 0 && (hid === undefined || !ordered.some((m) => m.id === hid))) hid = ordered[0].id
  if (seenReset !== resetKey) setSeenReset(resetKey)
  if (hid !== highlightId) setHighlightId(hid)

  useEffect(() => {
    setFailed(false)
  }, [hid, resetKey])

  const safe = Math.max(
    0,
    ordered.findIndex((m) => m.id === hid),
  )
  const setIndex = (i: number) => {
    const m = ordered[i]
    if (m) setHighlightId(m.id)
  }
  const current = n > 0 ? ordered[safe] : undefined
  const src = current && !failed ? current.url : coverPlaceholderUrl

  return (
    <section
      role="region"
      aria-label="Imagens"
      aria-roledescription="carrossel"
      className="flex flex-col gap-2 rounded border border-line bg-surface p-2"
    >
      <div
        title={current ? 'Ampliar' : undefined}
        onClick={() => current && setZoomed(true)}
        className={`flex aspect-video w-full items-center justify-center overflow-hidden rounded bg-raised ${current ? 'cursor-zoom-in' : ''}`}
      >
        <img src={src} alt="" onError={() => setFailed(true)} className="h-full w-full object-contain" />
      </div>
      {current && (
        <div className="flex items-center justify-between gap-2 text-sm text-muted">
          <span>{mediaKindLabels[current.kind]}</span>
          <span aria-live="polite">{`${safe + 1} de ${n}`}</span>
        </div>
      )}
      {n >= 2 && <MediaThumbStrip key={resetKey} items={ordered} index={safe} onSelect={setIndex} />}
      {zoomed && n > 0 && (
        <MediaLightbox items={ordered} index={safe} onIndex={setIndex} onClose={() => setZoomed(false)} />
      )}
    </section>
  )
}
