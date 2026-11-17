import { useEffect, useRef } from 'react'
import type { JSX, KeyboardEvent, MouseEvent, WheelEvent } from 'react'
import { mediaKindLabels } from '../lib/metadata'
import type { MediaItemView } from '../lib/metadata'

// Minimum gap between two steps of the mouse wheel, so one notch moves one image.
const WHEEL_STEP_MS = 120

// MediaLightbox shows the images of a game large and centered. The mouse wheel, the arrow keys and the
// buttons move through them; Esc or a click outside the image closes it.
export function MediaLightbox({
  items,
  index,
  onIndex,
  onClose,
}: {
  items: MediaItemView[]
  index: number
  onIndex: (i: number) => void
  onClose: () => void
}): JSX.Element {
  const n = items.length
  const box = useRef<HTMLDivElement>(null)
  const lastWheel = useRef(0)
  useEffect(() => {
    box.current?.focus()
  }, [])

  const go = (i: number) => {
    if (i >= 0 && i < n && i !== index) {
      onIndex(i)
    }
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    switch (e.key) {
      case 'Escape':
        e.preventDefault()
        e.stopPropagation()
        onClose()
        break
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault()
        go(index + 1)
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault()
        go(index - 1)
        break
    }
  }

  const onWheel = (e: WheelEvent<HTMLDivElement>) => {
    const now = Date.now()
    if (e.deltaY === 0 || now - lastWheel.current < WHEEL_STEP_MS) {
      return
    }
    lastWheel.current = now
    go(index + (e.deltaY > 0 ? 1 : -1))
  }

  const stop = (e: MouseEvent) => e.stopPropagation()
  const current = items[index]
  const arrow =
    'absolute top-1/2 -translate-y-1/2 rounded-full bg-overlay px-3 py-2 text-xl text-fg hover:bg-hover disabled:opacity-30'

  return (
    <div
      ref={box}
      role="dialog"
      aria-modal="true"
      aria-label="Imagens ampliadas"
      tabIndex={-1}
      onKeyDown={onKeyDown}
      onWheel={onWheel}
      onClick={onClose}
      className="fixed inset-0 z-[60] flex flex-col items-center justify-center gap-3 bg-overlay p-6 focus:outline-none focus-visible:outline-2"
    >
      {current &&
        (current.kind === 'video' ? (
          <video
            key={current.id}
            src={current.url}
            controls
            autoPlay
            loop
            playsInline
            onClick={stop}
            className="min-h-0 max-h-[calc(100vh-7rem)] max-w-[calc(100vw-9rem)] rounded object-contain shadow-2xl"
          />
        ) : (
          <img
            src={current.url}
            alt=""
            onClick={stop}
            className="min-h-0 max-h-[calc(100vh-7rem)] max-w-[calc(100vw-9rem)] rounded object-contain shadow-2xl"
          />
        ))}
      {current && (
        <div className="flex items-center gap-3 text-sm text-fg" onClick={stop}>
          <span>{mediaKindLabels[current.kind]}</span>
          <span aria-live="polite">{`${index + 1} de ${n}`}</span>
        </div>
      )}
      {n > 1 && (
        <>
          <button
            type="button"
            aria-label="Imagem anterior"
            disabled={index === 0}
            onClick={(e) => {
              stop(e)
              go(index - 1)
            }}
            className={`${arrow} left-4`}
          >
            ‹
          </button>
          <button
            type="button"
            aria-label="Próxima imagem"
            disabled={index === n - 1}
            onClick={(e) => {
              stop(e)
              go(index + 1)
            }}
            className={`${arrow} right-4`}
          >
            ›
          </button>
        </>
      )}
    </div>
  )
}
