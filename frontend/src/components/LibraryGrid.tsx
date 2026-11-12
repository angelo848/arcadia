import { useEffect, useLayoutEffect, useRef, useState, type JSX } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { GameCard, liveOf } from './GameCard'
import type { SessionView } from '../lib/games'
import type { GameView } from '../lib/library'
import type { PendingLaunch } from '../lib/pendingLaunch'

// Card and grid metrics in px. A row holds as many cards as fit the width.
const CARD_MIN_WIDTH = 220
const CARD_HEIGHT = 180
const GAP = 16
const ROW_HEIGHT = CARD_HEIGHT + GAP
const OVERSCAN_ROWS = 2
// The next page is asked for when the last rendered card is past loaded - 10 [UI-036].
const PREFETCH_AHEAD = 10

// columnsFor returns how many cards fit a row of the given width (at least one).
function columnsFor(width: number): number {
  return Math.max(1, Math.floor((width + GAP) / (CARD_MIN_WIDTH + GAP)))
}

// LibraryGrid shows the loaded games as a grid of cards, virtualized so that only
// the rows of the visible window are in the DOM [UI-029]. Scrolling changes the
// window without touching the network. When the last rendered card passes
// loaded - 40, onNeedMore is called once for this set of loaded games [UI-036].
export function LibraryGrid({
  games,
  total,
  current,
  pending,
  onNeedMore,
  onOpen,
  onPlay,
  onPlayUntracked,
}: {
  games: GameView[]
  total: number
  current: SessionView | null
  pending: PendingLaunch
  onNeedMore: () => void
  onOpen: (id: number) => void
  onPlay: (id: number) => void
  onPlayUntracked: (id: number) => void
}): JSX.Element {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [cols, setCols] = useState(1)
  const askedFor = useRef<number | null>(null)

  // The number of columns follows the width of the scroll element.
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el === null) {
      return undefined
    }
    const measure = () => setCols(columnsFor(el.getBoundingClientRect().width))
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => {
      observer.disconnect()
    }
  }, [])

  const loaded = games.length
  const rowCount = Math.ceil(loaded / cols)
  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: OVERSCAN_ROWS,
    // The window size comes from the same box as the column count (getBoundingClientRect),
    // not from clientWidth/clientHeight, which the default reads.
    observeElementRect: (instance, cb) => {
      const el = instance.scrollElement
      if (el === null) {
        return undefined
      }
      const read = () => {
        const box = el.getBoundingClientRect()
        cb({ width: Math.round(box.width), height: Math.round(box.height) })
      }
      read()
      const observer = new ResizeObserver(read)
      observer.observe(el)
      return () => {
        observer.disconnect()
      }
    },
  })
  const rows = virtualizer.getVirtualItems()

  const lastRow = rows.at(-1)?.index ?? -1
  const lastCard = Math.min(loaded - 1, (lastRow + 1) * cols - 1)

  // Prefetch [UI-036]: one call per set of loaded games, and only while more exist.
  useEffect(() => {
    if (lastRow < 0 || loaded >= total) {
      return
    }
    if (lastCard <= loaded - PREFETCH_AHEAD) {
      return
    }
    if (askedFor.current === loaded) {
      return
    }
    askedFor.current = loaded
    onNeedMore()
  }, [lastRow, lastCard, loaded, total, onNeedMore])

  return (
    <div ref={scrollRef} className="h-full min-h-0 overflow-y-auto">
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {rows.map((row) => {
          const first = row.index * cols
          const cards = games.slice(first, Math.min(first + cols, loaded))
          return (
            <div
              key={row.key}
              className="absolute left-0 top-0 grid w-full"
              style={{
                height: ROW_HEIGHT,
                transform: `translateY(${row.start}px)`,
                gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
                gridTemplateRows: `${CARD_HEIGHT}px`,
                columnGap: GAP,
              }}
            >
              {cards.map((g) => (
                <GameCard
                  key={g.id}
                  game={g}
                  live={liveOf(current, g.id)}
                  busy={pending !== null && pending.id === g.id ? pending.mode : null}
                  onOpen={onOpen}
                  onPlay={onPlay}
                  onPlayUntracked={onPlayUntracked}
                />
              ))}
            </div>
          )
        })}
      </div>
    </div>
  )
}
