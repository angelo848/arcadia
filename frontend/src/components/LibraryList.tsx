import { useEffect, useLayoutEffect, useRef, type FocusEvent, type JSX, type KeyboardEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { LibraryGrid } from './LibraryGrid'
import { liveOf } from './GameCard'
import { formatLocalDateTime } from '../lib/formatDate'
import { formatPlayTime } from '../lib/games'
import { isNavKey, nextIndex } from '../lib/gridNav'

// Row metrics in px. The list is one column of rows [UIX-020].
const ROW_HEIGHT = 64
const OVERSCAN_ROWS = 4
// The next page is asked for when the last rendered row is past loaded - 10 [UI-036].
const PREFETCH_AHEAD = 10

// LibraryList shows the same page as LibraryGrid, one game per row: title, platform, play
// time, last played and the session mark, with no buttons [UIX-020]. Rows are virtualized, so
// only the rows of the visible window are in the DOM. The prefetch rule is the grid's [UI-036].
export function LibraryList({
  games,
  total,
  current,
  selectedId,
  onNeedMore,
  onSelect,
  onLaunch,
  keyboard,
}: Parameters<typeof LibraryGrid>[0]): JSX.Element {
  const scrollRef = useRef<HTMLDivElement>(null)
  const askedFor = useRef<number | null>(null)
  const scrolledToSelected = useRef(false)

  const loaded = games.length
  const virtualizer = useVirtualizer({
    count: loaded,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: OVERSCAN_ROWS,
    // The window size comes from the same box the grid uses (getBoundingClientRect),
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

  // On mount, the game in focus is brought into view once, when it is loaded [UIX-021].
  useLayoutEffect(() => {
    if (scrolledToSelected.current) {
      return
    }
    scrolledToSelected.current = true
    if (selectedId === null) {
      return
    }
    const index = games.findIndex((g) => g.id === selectedId)
    const el = scrollRef.current
    if (index >= 0 && el !== null) {
      // Scrolled directly: the virtualizer clamps scrollToIndex to the scroll height, which a
      // fresh element may not have yet. The virtualizer follows the scroll event.
      const top = index * ROW_HEIGHT - (el.getBoundingClientRect().height - ROW_HEIGHT) / 2
      el.scrollTo?.({ top: Math.max(0, top) })
    }
  }, [selectedId, games])

  const rows = virtualizer.getVirtualItems()

  const lastRow = rows.at(-1)?.index ?? -1

  // Prefetch [UI-036]: one call per set of loaded games, and only while more exist.
  useEffect(() => {
    if (lastRow < 0 || loaded >= total) {
      return
    }
    if (lastRow <= loaded - PREFETCH_AHEAD) {
      return
    }
    if (askedFor.current === loaded) {
      return
    }
    askedFor.current = loaded
    onNeedMore()
  }, [lastRow, loaded, total, onNeedMore])

  const selectedIndex = selectedId === null ? -1 : games.findIndex((g) => g.id === selectedId)
  // The one row that is a Tab stop: the game in focus when loaded, else the first [UIX-019].
  const stopIndex = selectedIndex >= 0 ? selectedIndex : 0
  const stopRendered = rows.some((r) => r.index === stopIndex)

  // Scrolls a row into the window (or centers it) [UIX-019].
  const scrollToIndex = (index: number, center: boolean) => {
    const el = scrollRef.current
    if (el === null) {
      return
    }
    const height = el.getBoundingClientRect().height
    const top = index * ROW_HEIGHT
    if (center) {
      el.scrollTo?.({ top: Math.max(0, top - (height - ROW_HEIGHT) / 2) })
    } else if (top < el.scrollTop) {
      el.scrollTo?.({ top })
    } else if (top + ROW_HEIGHT > el.scrollTop + height) {
      el.scrollTo?.({ top: top + ROW_HEIGHT - height })
    }
  }

  // The row that must receive DOM focus once it is rendered (it may be outside the window now).
  const pendingFocus = useRef<number | null>(null)
  const focusPending = () => {
    const id = pendingFocus.current
    if (id === null) {
      return
    }
    const row = scrollRef.current?.querySelector<HTMLElement>(`[data-game-id="${id}"]`)
    if (row) {
      pendingFocus.current = null
      row.focus({ preventScroll: true })
    }
  }
  useLayoutEffect(focusPending)

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!keyboard) {
      return
    }
    const rowEl = (e.target as HTMLElement).closest<HTMLElement>('[data-game-id]')
    const focusedId = rowEl === null ? null : Number(rowEl.dataset.gameId)
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
    const to = nextIndex(from, e.key, 1, loaded)
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

  // The container is a Tab stop only while the focusable row is not in the DOM; it hands the
  // focus to the row after scrolling to it.
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
          const g = games[row.index]
          if (g === undefined) {
            return null
          }
          const live = liveOf(current, g.id)
          const selected = g.id === selectedId
          const tabbable = row.index === stopIndex
          return (
            <div
              key={row.key}
              role="option"
              aria-selected={selected}
              data-game-id={g.id}
              data-bigbox-item
              tabIndex={tabbable ? 0 : -1}
              onClick={() => onSelect(g.id)}
              onDoubleClick={() => onLaunch(g.id)}
              className={`absolute left-0 top-0 flex w-full cursor-pointer items-center gap-4 border-b border-line px-4 ${
                selected ? 'bg-raised' : 'hover:bg-hover'
              }`}
              style={{ height: ROW_HEIGHT, transform: `translateY(${row.start}px)` }}
            >
              <span className="min-w-0 flex-1 truncate font-medium text-fg">{g.title}</span>{' '}
              <span className="w-40 shrink-0 truncate text-sm text-muted">{g.platform}</span>{' '}
              <span className="w-28 shrink-0 text-sm text-muted">{formatPlayTime(g.totalPlaySeconds)}</span>{' '}
              <span className="w-36 shrink-0 text-sm text-muted">{formatLocalDateTime(g.lastPlayedAt)}</span>{' '}
              <span className="w-24 shrink-0 text-sm font-medium">
                {live === 'running' && <span className="text-running">Em execução</span>}
                {live === 'paused' && <span className="text-paused">Pausado</span>}
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
