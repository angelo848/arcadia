import { useEffect, useLayoutEffect, useRef, useState, type FocusEvent, type JSX, type KeyboardEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { GameCard, liveOf } from './GameCard'
import { GRID_GAP, columnsFor, isNavKey, nextIndex } from '../lib/gridNav'
import type { SessionView } from '../lib/games'
import type { GameView } from '../lib/library'

const OVERSCAN_ROWS = 2
// The next page is asked for when the last rendered card is past loaded - 10 [UI-036].
const PREFETCH_AHEAD = 10

// LibraryGrid shows the loaded games as a grid of cards, virtualized so that only
// the rows of the visible window are in the DOM [UIX-016]. Scrolling changes the
// window without touching the network. When the last rendered card passes
// loaded - 10, onNeedMore is called once for this set of loaded games [UI-036].
export function LibraryGrid({
  games,
  covers,
  total,
  current,
  selectedId,
  onNeedMore,
  onSelect,
  onLaunch,
  keyboard,
}: {
  // keyboard enables arrows/Home/End/Enter; false while Big Box owns the keys [UIX-041].
  keyboard: boolean
  games: GameView[]
  // covers maps a game id to its cover thumbnail URL (MID-006); a game without one shows the placeholder.
  covers?: Record<number, string>
  total: number
  current: SessionView | null
  // selectedId is the game in focus of the details panel; null when none [UIX-017].
  selectedId: number | null
  onNeedMore: () => void
  onSelect: (id: number) => void
  onLaunch: (id: number) => void
}): JSX.Element {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [metrics, setMetrics] = useState(() => ({ ...columnsFor(0), measured: false }))
  const askedFor = useRef<number | null>(null)
  const scrolledToSelected = useRef(false)
  const { cols, rowHeight, measured } = metrics

  // The columns and the row height follow the width of the scroll element.
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el === null) {
      return undefined
    }
    const measure = () => {
      const m = columnsFor(el.getBoundingClientRect().width)
      setMetrics((old) =>
        old.measured && old.cols === m.cols && old.rowHeight === m.rowHeight
          ? old
          : { ...m, measured: true },
      )
    }
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
    estimateSize: () => rowHeight,
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

  // A new row height invalidates the sizes the virtualizer measured.
  useEffect(() => {
    virtualizer.measure()
  }, [rowHeight, virtualizer])

  // On mount, the game in focus is brought into view once, when it is among the loaded ones [UIX-021].
  useLayoutEffect(() => {
    if (!measured || scrolledToSelected.current) {
      return
    }
    scrolledToSelected.current = true
    if (selectedId === null) {
      return
    }
    const index = games.findIndex((g) => g.id === selectedId)
    const el = scrollRef.current
    if (index >= 0 && el !== null) {
      // The row is centered in the window. The element is scrolled directly (the virtualizer
      // clamps scrollToIndex to the scroll height, which a fresh element may not have yet)
      // and the virtualizer follows the scroll event.
      const top = Math.floor(index / cols) * rowHeight - (el.getBoundingClientRect().height - rowHeight) / 2
      el.scrollTo?.({ top: Math.max(0, top) })
    }
  }, [measured, selectedId, games, cols, rowHeight])

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

  const selectedIndex = selectedId === null ? -1 : games.findIndex((g) => g.id === selectedId)
  const selectedLoaded = selectedIndex >= 0
  // The one card that is a Tab stop: the game in focus when loaded, else the first [UIX-019].
  const stopIndex = selectedLoaded ? selectedIndex : 0
  const stopRendered = rows.some((r) => r.index === Math.floor(stopIndex / cols))

  // Scrolls the row of a game into the window (or centers it) [UIX-019].
  const scrollToIndex = (index: number, center: boolean) => {
    const el = scrollRef.current
    if (el === null) {
      return
    }
    const height = el.getBoundingClientRect().height
    const top = Math.floor(index / cols) * rowHeight
    if (center) {
      el.scrollTo?.({ top: Math.max(0, top - (height - rowHeight) / 2) })
    } else if (top < el.scrollTop) {
      el.scrollTo?.({ top })
    } else if (top + rowHeight > el.scrollTop + height) {
      el.scrollTo?.({ top: top + rowHeight - height })
    }
  }

  // The card that must receive DOM focus once it is rendered (it may be outside the window now).
  const pendingFocus = useRef<number | null>(null)
  const focusPending = () => {
    const id = pendingFocus.current
    if (id === null) {
      return
    }
    const card = scrollRef.current?.querySelector<HTMLElement>(`[data-game-id="${id}"]`)
    if (card) {
      pendingFocus.current = null
      card.focus({ preventScroll: true })
    }
  }
  useLayoutEffect(focusPending)

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!keyboard) {
      return
    }
    const cardEl = (e.target as HTMLElement).closest<HTMLElement>('[data-game-id]')
    const focusedId = cardEl === null ? null : Number(cardEl.dataset.gameId)
    if (e.key === 'Enter') {
      if (focusedId !== null) {
        e.preventDefault()
        onLaunch(focusedId)
      }
      return
    }
    if (!isNavKey(e.key) || loaded === 0) {
      return
    }
    e.preventDefault()
    const from = focusedId === null ? stopIndex : games.findIndex((g) => g.id === focusedId)
    if (from < 0) {
      return
    }
    const to = nextIndex(from, e.key, cols, loaded)
    const target = games[to]
    if (to === from || target === undefined) {
      return
    }
    pendingFocus.current = target.id
    onSelect(target.id)
    scrollToIndex(to, false)
    focusPending()
    if (to >= loaded - PREFETCH_AHEAD && loaded < total && askedFor.current !== loaded) {
      askedFor.current = loaded
      onNeedMore()
    }
  }

  // The container is a Tab stop only while the focusable card is not in the DOM; it hands the
  // focus to the card after scrolling to it.
  const onContainerFocus = (e: FocusEvent<HTMLDivElement>) => {
    if (!keyboard || e.target !== e.currentTarget || loaded === 0) {
      return
    }
    const target = games[stopIndex]
    if (target === undefined) {
      return
    }
    pendingFocus.current = target.id
    scrollToIndex(stopIndex, true)
    focusPending()
  }

  return (
    <div
      ref={scrollRef}
      role="listbox"
      aria-label="Jogos"
      tabIndex={keyboard && loaded > 0 && !stopRendered ? 0 : -1}
      onKeyDown={onKeyDown}
      onFocus={onContainerFocus}
      className="h-full min-h-0 overflow-y-auto focus:outline-none focus-visible:outline-2"
    >
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {rows.map((row) => {
          const first = row.index * cols
          const cards = games.slice(first, Math.min(first + cols, loaded))
          return (
            <div
              key={row.key}
              className="absolute left-0 top-0 grid w-full content-start"
              style={{
                height: rowHeight,
                transform: `translateY(${row.start}px)`,
                gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
                columnGap: GRID_GAP,
              }}
            >
              {cards.map((g, i) => (
                <GameCard
                  key={g.id}
                  game={g}
                  live={liveOf(current, g.id)}
                  coverUrl={covers?.[g.id]}
                  selected={g.id === selectedId}
                  tabbable={first + i === stopIndex}
                  onSelect={onSelect}
                  onLaunch={onLaunch}
                />
              ))}
            </div>
          )
        })}
      </div>
    </div>
  )
}
