// Read-only data of the Big Box [BBX-004, BBX-007, BBX-017, BBX-027..BBX-029]. Never writes anything.
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { STRIP_MAX } from './viewModel'
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

// useTabGames reads the games of a tab (platform filter only, by title) page by page [BBX-007, BBX-038].
export function useTabGames(platformId: number | null, capacity: number): TabGames {
  const qc = useQueryClient()
  const limitRef = useRef(pageSize(capacity))
  limitRef.current = pageSize(capacity)
  const q = useInfiniteQuery({
    queryKey: ['bigbox', 'games', platformId],
    queryFn: async ({ pageParam }) => {
      const fn = binding().ListGames
      if (!fn) throw new Error('ListGames indisponível')
      const filter: FilterSpec = platformId === null ? { version: 1 } : { version: 1, platformIds: [platformId] }
      return fn({ filter, sort: 'title', offset: pageParam, limit: limitRef.current })
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
      void qc.cancelQueries({ queryKey: ['bigbox', 'games', platformId], exact: true })
    }
  }, [qc, platformId])

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
