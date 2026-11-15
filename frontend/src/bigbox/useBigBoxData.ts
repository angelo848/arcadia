// Read-only data of the Big Box [BBX-004, BBX-007, BBX-017, BBX-027..BBX-029]. Never writes anything.
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { FilterSpec, SortKey } from '../lib/filters'
import { currentSession, currentSessionKey } from '../lib/games'
import type { SessionView } from '../lib/games'
import { libraryKeys } from '../lib/library'
import type { GameDetailView, GameListView, GameView } from '../lib/library'
import { fetchMedia, mediaKey } from '../lib/metadata'
import type { MediaItemView } from '../lib/metadata'

export const PAGE_SIZE = 200

type Binding = {
  ListGames?: (a: { filter: FilterSpec; sort: SortKey; offset: number; limit: number }) => Promise<GameListView>
  GetGame?: (id: number) => Promise<GameDetailView>
}
type Win = { go?: { main?: { App?: Binding } }; runtime?: { EventsOn?: (n: string, cb: (...d: unknown[]) => void) => (() => void) | void } }

function binding(): Binding {
  return (window as unknown as Win).go?.main?.App ?? {}
}

export type TabGames = {
  games: GameView[]
  loaded: number
  total: number
  isPending: boolean
  isFetchingNextPage: boolean
  error: Error | null
  fetchNextPage: () => void
}

// useTabGames pages the games of a tab (platform filter only, by title) in pages of PAGE_SIZE [BBX-007].
export function useTabGames(platformId: number | null): TabGames {
  const q = useInfiniteQuery({
    queryKey: ['bigbox', 'games', platformId],
    queryFn: async ({ pageParam }) => {
      const fn = binding().ListGames
      if (!fn) throw new Error('ListGames indisponível')
      const filter: FilterSpec = platformId === null ? { version: 1 } : { version: 1, platformIds: [platformId] }
      return fn({ filter, sort: 'title', offset: pageParam, limit: PAGE_SIZE })
    },
    initialPageParam: 0,
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + p.games.length, 0)
      return loaded < last.total && last.games.length > 0 ? loaded : undefined
    },
    retry: false,
    refetchOnWindowFocus: false,
  })
  const games = q.data?.pages.flatMap((p) => p.games) ?? []
  const total = q.data?.pages[q.data.pages.length - 1]?.total ?? 0
  const busy = useRef(false)
  busy.current = q.isFetchingNextPage || q.isFetching
  const { hasNextPage, fetchNextPage: next } = q
  const fetchNextPage = useCallback(() => {
    if (!hasNextPage || busy.current) return
    busy.current = true
    void next({ cancelRefetch: false })
  }, [hasNextPage, next])
  return {
    games,
    loaded: games.length,
    total,
    isPending: q.isPending,
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
