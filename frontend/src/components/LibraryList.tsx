import { useEffect, useRef, type JSX } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { LibraryGrid } from './LibraryGrid'
import { liveOf, type BusyMode } from './GameCard'
import { formatPlayTime } from '../lib/games'

// Row metrics in px. The list is one column of rows [UI-030].
const ROW_HEIGHT = 80
const OVERSCAN_ROWS = 4
// The next page is asked for when the last rendered row is past loaded - 40 [UI-036].
const PREFETCH_AHEAD = 40

const buttonClass =
  'shrink-0 rounded px-4 py-1.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400'

// LibraryList shows the same page as LibraryGrid, one game per row, with the same data
// and the same buttons [UI-030, UI-024, UI-031]. Rows are virtualized, so only the
// rows of the visible window are in the DOM. The prefetch rule is the grid's [UI-036].
export function LibraryList({
  games,
  total,
  current,
  pending,
  onNeedMore,
  onOpen,
  onPlay,
  onPlayUntracked,
}: Parameters<typeof LibraryGrid>[0]): JSX.Element {
  const scrollRef = useRef<HTMLDivElement>(null)
  const askedFor = useRef<number | null>(null)

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

  return (
    <div ref={scrollRef} className="h-full min-h-0 overflow-y-auto">
      <ul className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {rows.map((row) => {
          const g = games[row.index]
          if (g === undefined) {
            return null
          }
          const live = liveOf(current, g.id)
          const busy: BusyMode = pending !== null && pending.id === g.id ? pending.mode : null
          const blocked = busy !== null
          return (
            <li
              key={row.key}
              className="absolute left-0 top-0 flex w-full items-center justify-between gap-4 border-b border-neutral-800 px-6"
              style={{ height: ROW_HEIGHT, transform: `translateY(${row.start}px)` }}
            >
              <div
                role="link"
                tabIndex={0}
                onClick={() => onOpen(g.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    onOpen(g.id)
                  }
                }}
                className="flex min-w-0 cursor-pointer flex-col"
              >
                <span className="truncate font-medium text-neutral-100">{g.title}</span>
                <span className="truncate text-sm text-neutral-400">{g.platform}</span>
                <span className="flex items-center gap-2 text-sm text-neutral-400">
                  <span>{formatPlayTime(g.totalPlaySeconds)}</span>
                  {live === 'running' && <span className="font-medium text-emerald-400">Em execução</span>}
                  {live === 'paused' && <span className="font-medium text-sky-400">Pausado</span>}
                </span>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <button
                  type="button"
                  disabled={blocked}
                  onClick={() => onPlay(g.id)}
                  className={`${buttonClass} bg-emerald-600 hover:bg-emerald-500`}
                >
                  {busy === 'tracked' ? 'Iniciando…' : 'Jogar'}
                </button>
                <button
                  type="button"
                  disabled={blocked}
                  onClick={() => onPlayUntracked(g.id)}
                  className={`${buttonClass} bg-neutral-800 text-neutral-100 hover:bg-neutral-700`}
                >
                  {busy === 'untracked' ? 'Iniciando…' : 'Sem contar tempo'}
                </button>
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
