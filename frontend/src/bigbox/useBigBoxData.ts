// Read-only data of the Big Box [BBX-004, BBX-007, BBX-017, BBX-027..BBX-029]. Never writes anything.
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { GRID_MAX_NODES, STRIP_MAX, effectiveRows } from './viewModel'
import type { BigBoxTab } from './viewModel'
import type { FilterSpec, SortKey } from '../lib/filters'
import { currentSession, currentSessionKey } from '../lib/games'
import type { SessionView } from '../lib/games'
import { libraryKeys } from '../lib/library'
import type { GameDetailView, GameListView, GameView } from '../lib/library'
import { fetchMedia, mediaKey } from '../lib/metadata'
import type { MediaItemView } from '../lib/metadata'


type Binding = {
  ListGames?: (a: { filter: FilterSpec; sort: SortKey; offset: number; limit: number }) => Promise<GameListView>
  GetGame?: (id: number) => Promise<GameDetailView>
}
type Win = { go?: { main?: { App?: Binding } }; runtime?: { EventsOn?: (n: string, cb: (...d: unknown[]) => void) => (() => void) | void } }

function binding(): Binding {
  return (window as unknown as Win).go?.main?.App ?? {}
}

// A tab is read in pages of V + PAGE_EXTRA games, ahead of the focus [BBX-007, BBX-038].
export const PAGE_EXTRA = 3
export const PAGE_MAX = 500

const clampCapacity = (capacity: number) => Math.min(Math.max(capacity, 1), STRIP_MAX - 1)

// pageSize is the `limit` of each ListGames call for the capacity of the moment.
export function pageSize(capacity: number): number {
  return Math.min(clampCapacity(capacity) + PAGE_EXTRA, PAGE_MAX)
}

// gridPageSize is the `limit` of each ListGames call on a grid theme [BBX-050, BBX-062].
export function gridPageSize(columns: number, rows: number, maxNodes: number = GRID_MAX_NODES): number {
  return Math.min((effectiveRows(rows, columns, maxNodes) + 1 + PAGE_EXTRA) * columns, PAGE_MAX)
}

// shouldPrefetchGrid: fewer games read than the focus row, the window and PAGE_EXTRA more rows need [BBX-050].
export function shouldPrefetchGrid(
  loaded: number,
  total: number,
  focus: number,
  columns: number,
  rows: number,
  exhausted: boolean,
  maxNodes: number = GRID_MAX_NODES,
): boolean {
  const need = Math.min(total, (Math.floor(focus / columns) + effectiveRows(rows, columns, maxNodes) + 1 + PAGE_EXTRA) * columns)
  return !exhausted && loaded < total && loaded < need
}

// shouldPrefetch tells whether the next page must be requested: fewer than V + 3 games read from the focus on.
export function shouldPrefetch(loaded: number, total: number, focus: number, capacity: number, exhausted: boolean): boolean {
  return !exhausted && loaded < total && loaded - focus < clampCapacity(capacity) + PAGE_EXTRA
}

export type TabGames = {
  games: GameView[]
  total: number
  loaded: number
  exhausted: boolean
  isPending: boolean
  isFetchingNextPage: boolean
  error: Error | null
  fetchNextPage: () => void
}

export type TabPaging = { kind: 'strip'; capacity: number } | { kind: 'grid'; columns: number; rows: number; maxNodes?: number }

function tabFilter(tab: BigBoxTab): FilterSpec {
  const f = tab.filter
  const filter: FilterSpec = { version: 1 }
  if (f.platformId !== null) filter.platformIds = [f.platformId]
  if (f.favorite) filter.favorite = true
  if (f.status === 'completed') filter.statuses = ['completed']
  return filter
}

// useTabGames reads the games of a tab (its filter, in the order of the theme) page by page [BBX-007, BBX-038, BBX-050, BBX-055].
export function useTabGames(tab: BigBoxTab, sort: SortKey, paging: TabPaging): TabGames {
  const qc = useQueryClient()
  const limit = paging.kind === 'grid' ? gridPageSize(paging.columns, paging.rows, paging.maxNodes) : pageSize(paging.capacity)
  const limitRef = useRef(limit)
  limitRef.current = limit
  const tabKey = tab.key
  const filterRef = useRef(tab)
  filterRef.current = tab
  const q = useInfiniteQuery({
    queryKey: ['bigbox', 'games', tabKey, sort],
    queryFn: async ({ pageParam }) => {
      const fn = binding().ListGames
      if (!fn) throw new Error('ListGames indisponível')
      return fn({ filter: tabFilter(filterRef.current), sort, offset: pageParam, limit: limitRef.current })
    },
    initialPageParam: 0,
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + p.games.length, 0)
      return loaded < last.total && last.games.length > 0 ? loaded : undefined
    },
    staleTime: Infinity,
    retry: false,
    refetchOnWindowFocus: false,
  })

  // Leaving a tab discards what is still in flight for it; pages already read stay cached.
  useEffect(() => {
    return () => {
      void qc.cancelQueries({ queryKey: ['bigbox', 'games', tabKey, sort], exact: true })
    }
  }, [qc, tabKey, sort])

  const pages = q.data?.pages
  const games = useMemo(() => pages?.flatMap((p) => p.games) ?? [], [pages])
  const total = pages?.[0]?.total ?? 0
  const exhausted = !!pages && pages.length > 0 && pages[pages.length - 1].games.length === 0
  const busy = useRef(false)
  busy.current = q.isFetching
  const { hasNextPage, fetchNextPage: next } = q
  const fetchNextPage = useCallback(() => {
    if (!hasNextPage || busy.current) return
    busy.current = true
    void next({ cancelRefetch: false })
  }, [hasNextPage, next])
  return {
    games,
    total,
    loaded: games.length,
    exhausted,
    isPending: q.isPending && !q.isError,
    isFetchingNextPage: q.isFetchingNextPage,
    error: q.error ?? null,
    fetchNextPage,
  }
}

export type FocusedData = {
  detail: GameDetailView | undefined
  media: MediaItemView[]
  session: SessionView | null
  description: string
  genres: string | null
}

// useFocusedData loads detail, media and session of the focused game; the session ending rereads the game [BBX-017, BBX-028].
export function useFocusedData(gameId: number | null): FocusedData {
  const qc = useQueryClient()
  const id = gameId ?? 0
  const detail = useQuery({
    queryKey: libraryKeys.game(id),
    enabled: gameId !== null,
    queryFn: () => {
      const fn = binding().GetGame
      if (!fn) throw new Error('GetGame indisponível')
      return fn(id)
    },
    retry: false,
    refetchOnWindowFocus: false,
  })
  const media = useQuery({
    queryKey: mediaKey(id),
    enabled: gameId !== null,
    queryFn: () => fetchMedia(id),
    retry: false,
  })
  const initial = useQuery({ queryKey: currentSessionKey, queryFn: currentSession, retry: false, refetchOnWindowFocus: false })
  const [live, setLive] = useState<SessionView | null | undefined>(undefined)

  useEffect(() => {
    const on = (window as unknown as Win).runtime?.EventsOn
    if (!on) return undefined
    const off = on('session:changed', (...d: unknown[]) => {
      const s = d[0] as SessionView | undefined
      if (!s || typeof s !== 'object') return
      if (s.state === 'ended' || s.state === 'discarded') {
        setLive(null)
        void qc.invalidateQueries({ queryKey: libraryKeys.game(s.gameId), exact: true })
      } else {
        setLive(s)
      }
    })
    return () => {
      if (typeof off === 'function') off()
    }
  }, [qc])

  const session = live !== undefined ? live : (initial.data ?? null)
  return {
    detail: detail.data,
    media: media.data ?? [],
    session,
    description: detail.data?.description ?? '',
    genres: detail.data && detail.data.genres.length > 0 ? detail.data.genres.join(', ') : null,
  }
}
