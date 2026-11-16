import { useEffect, useLayoutEffect, useRef, useState, type FocusEvent, type JSX, type KeyboardEvent, type MouseEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { GameCard, liveOf } from './GameCard'
import { GRID_GAP, GRID_INSET, columnsFor, usableWidth, isNavKey, nextIndex } from '../lib/gridNav'
import type { SessionView } from '../lib/games'
import type { GameView } from '../lib/library'
import type { CardImage } from '../lib/metadata'

const OVERSCAN_ROWS = 2
// The next page is asked for when the last rendered card is past loaded - 10 [UI-036].
const PREFETCH_AHEAD = 10

// LibraryGrid shows the loaded games as a grid of cards, virtualized so that only
// the rows of the visible window are in the DOM [UIX-016]. Scrolling changes the
// window without touching the network. When the last rendered card passes
// loaded - 10, onNeedMore is called once for this set of loaded games [UI-036].
export function LibraryGrid({
  games,
  images,
  total,
  current,
  selectedId,
  onNeedMore,
  onSelect,
  onLaunch,
  keyboard,
}: {
  // keyboard enables arrows/Home/End/Enter.
  keyboard: boolean
  games: GameView[]
  // images maps a game id to its card image [AJU-015]; a game without one shows the placeholder.
  images?: Record<number, CardImage>
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
      const m = columnsFor(usableWidth(el.getBoundingClientRect().width))
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

  // On mount, the game in focus is brought into view once, when it is among the loaded ones [UIX-021].
  // Declared before the virtualizer so that its mount write reads the scroll position set here;
  // the metrics are read from the element because the state is not measured yet.
  const centerSelected = () => {
    if (selectedId === null) {
      return
    }
    const index = games.findIndex((g) => g.id === selectedId)
    const el = scrollRef.current
    if (index >= 0 && el !== null) {
      // The row is centered in the window. The element is scrolled directly (the virtualizer
      // clamps scrollToIndex to the scroll height, which a fresh element may not have yet)
      // and the virtualizer follows the scroll event.
      const box = el.getBoundingClientRect()
      const m = columnsFor(usableWidth(box.width))
      const top = GRID_INSET + Math.floor(index / m.cols) * m.rowHeight - (box.height - m.rowHeight) / 2
      el.scrollTo?.({ top: Math.max(0, top) })
    }
  }
  useLayoutEffect(() => {
    if (scrolledToSelected.current) {
      return
    }
    scrolledToSelected.current = true
    centerSelected()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, games])
  // Once measured, the position is set again: the virtualizer's own mount write may have run after.
  const centeredAfterMeasure = useRef(false)
  useLayoutEffect(() => {
    if (!measured || centeredAfterMeasure.current) {
      return
    }
    centeredAfterMeasure.current = true
    centerSelected()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [measured])

  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan: OVERSCAN_ROWS,
    paddingStart: GRID_INSET,
    paddingEnd: GRID_INSET,
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
    const rowTop = GRID_INSET + Math.floor(index / cols) * rowHeight
    if (center) {
      el.scrollTo?.({ top: Math.max(0, rowTop - (height - rowHeight) / 2) })
    } else if (rowTop <= GRID_INSET || rowTop - GRID_INSET < el.scrollTop) {
      el.scrollTo?.({ top: Math.max(0, rowTop - GRID_INSET) })
    } else if (rowTop + rowHeight + GRID_INSET > el.scrollTop + height) {
      el.scrollTo?.({ top: rowTop + rowHeight + GRID_INSET - height })
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
        if (!e.repeat) {
          onLaunch(focusedId)
        }
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
  // A press on the container itself is a press on its scrollbar (the cards cover the rest). The focus it gives
  // must not scroll: jumping to the game in focus would fight the drag or the click on the track [UIX-019].
  const mouseFocus = useRef(false)
  const onContainerMouseDown = (e: MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) {
      mouseFocus.current = true
    }
  }
  const onContainerBlur = (e: FocusEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) {
      mouseFocus.current = false
    }
  }

  const onContainerFocus = (e: FocusEvent<HTMLDivElement>) => {
    if (!keyboard || e.target !== e.currentTarget || loaded === 0) {
      return
    }
    if (mouseFocus.current) {
      mouseFocus.current = false
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
      onMouseDown={onContainerMouseDown}
      onBlur={onContainerBlur}
      className="h-full min-h-0 overflow-y-auto focus:outline-none focus-visible:outline-2 [scrollbar-gutter:stable]"
    >
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {rows.map((row) => {
          const first = row.index * cols
          const cards = games.slice(first, Math.min(first + cols, loaded))
          return (
            <div
              key={row.key}
              className="absolute top-0 grid content-start"
              style={{
                height: rowHeight,
                left: GRID_INSET,
                width: `calc(100% - ${2 * GRID_INSET}px)`,
                transform: `translateY(${GRID_INSET + row.index * rowHeight}px)`,
                gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
                columnGap: GRID_GAP,
              }}
            >
              {cards.map((g, i) => (
                <GameCard
                  key={g.id}
                  game={g}
                  live={liveOf(current, g.id)}
                  image={images?.[g.id]}
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
