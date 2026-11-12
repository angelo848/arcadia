import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { InfiniteData, UseInfiniteQueryResult, UseMutationResult, UseQueryResult } from '@tanstack/react-query'
import type { FilterSpec, GameStatus, LibraryQuery, SortKey } from './filters'
import type { SessionState } from './games'

// Types mirror the IPC contract (docs/specs/phase-5-library/contracts/ipc.openrpc.json) by JSON field name.

export type GameView = {
  id: number
  platformId: number
  title: string
  platform: string
  path: string
  status: GameStatus
  favorite: boolean
  year: number | null
  totalPlaySeconds: number
  playCount: number
  firstPlayedAt: string
  lastPlayedAt: string
}

export type GameDetailView = GameView & {
  description: string
  completedAt: string | null
  developers: string[]
  publishers: string[]
  genres: string[]
  playModes: PlayModeName[]
}

export type PlayModeName = 'single' | 'multi_local' | 'multi_online' | 'coop_local' | 'coop_online'

export type GameListView = { games: GameView[]; total: number; offset: number }

export type SessionEndReason = 'exited' | 'agent-shutdown' | 'recovered'

export type SessionItemView = {
  sessionId: number
  gameId: number
  state: SessionState
  startedAt: string
  endedAt: string | null
  activeSeconds: number
  endReason: SessionEndReason | null
}

export type SessionListView = { sessions: SessionItemView[]; total: number; offset: number }

export type PlatformView = { id: number; name: string; category: string }

// A genre or a company of the catalog: id is the database id the FilterSpec filters by (FLT-002, FLT-003).
export type CatalogItemView = { id: number; name: string }

export type CatalogView = {
  platforms: PlatformView[]
  genres: CatalogItemView[]
  companies: CatalogItemView[]
  playModes: PlayModeName[]
}

// Prefix shared by every games query; invalidating it also reaches each query key.
export const libraryGamesKey = ['library', 'games'] as const

export const libraryKeys = {
  games: (q: LibraryQuery) => [...libraryGamesKey, q] as const,
  game: (id: number) => ['library', 'game', id] as const,
  sessions: (id: number) => ['library', 'sessions', id] as const,
  catalog: ['library', 'catalog'] as const,
}

const gamesPageSize = 50
const sessionsPageSize = 50
// Failed list calls are retried every 2 s [UI-044].
const retryEveryMs = 2000

// The bound methods are reached through window with local types, as lib/games.ts does.
type ListGamesArgs = { filter: FilterSpec; sort: SortKey; offset: number; limit: number }

type LibraryApp = {
  ListGames?: (args: ListGamesArgs) => Promise<GameListView>
  GetGame?: (id: number) => Promise<GameDetailView>
  GameSessions?: (id: number, offset: number, limit: number) => Promise<SessionListView>
  Catalog?: () => Promise<CatalogView>
  UpdateGame?: (id: number, patch: Record<string, unknown>) => Promise<GameDetailView>
  DeleteGame?: (id: number) => Promise<void>
  AddPlatform?: (name: string, category: string) => Promise<PlatformView>
}

type LibraryWindow = { go?: { main?: { App?: LibraryApp } } }

function libraryBinding<K extends keyof LibraryApp>(name: K): NonNullable<LibraryApp[K]> {
  const fn = (window as unknown as LibraryWindow).go?.main?.App?.[name]
  if (!fn) {
    throw new Error(`wails bindings unavailable: ${name}`)
  }
  return fn as NonNullable<LibraryApp[K]>
}

// Wails v2 rejects bound calls with a plain string; normalize to Error [UI-008].
function toError(e: unknown): Error {
  return e instanceof Error ? e : new Error(String(e))
}

async function call<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn()
  } catch (e) {
    throw toError(e)
  }
}

function listGames(q: LibraryQuery, offset: number): Promise<GameListView> {
  return call(() =>
    libraryBinding('ListGames')({ filter: q.filter, sort: q.sort, offset, limit: gamesPageSize }),
  )
}

function getGame(id: number): Promise<GameDetailView> {
  return call(() => libraryBinding('GetGame')(id))
}

function gameSessions(id: number, offset: number): Promise<SessionListView> {
  return call(() => libraryBinding('GameSessions')(id, offset, sessionsPageSize))
}

function catalog(): Promise<CatalogView> {
  return call(() => libraryBinding('Catalog')())
}

function updateGame(id: number, patch: Record<string, unknown>): Promise<GameDetailView> {
  return call(() => libraryBinding('UpdateGame')(id, patch))
}

function deleteGame(id: number): Promise<void> {
  return call(() => libraryBinding('DeleteGame')(id))
}

function addPlatform(name: string, category: string): Promise<PlatformView> {
  return call(() => libraryBinding('AddPlatform')(name, category))
}

// Sum of the items loaded so far; the next offset [UI-036].
function loadedCount(pages: { length: number }[]): number {
  return pages.reduce((n, p) => n + p.length, 0)
}

// useInfiniteGames pages the library by offset. A change in q is a new key, so the
// loaded pages are dropped [UI-036]. While a call fails it is retried every 2 s [UI-044].
export function useInfiniteGames(q: LibraryQuery): UseInfiniteQueryResult<InfiniteData<GameListView>, Error> {
  return useInfiniteQuery({
    queryKey: libraryKeys.games(q),
    queryFn: ({ pageParam }) => listGames(q, pageParam),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      const loaded = loadedCount(allPages.map((p) => p.games))
      return loaded < lastPage.total ? loaded : undefined
    },
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: (query) => (query.state.status === 'error' ? retryEveryMs : false),
  })
}

export function useGame(id: number): UseQueryResult<GameDetailView, Error> {
  return useQuery({
    queryKey: libraryKeys.game(id),
    queryFn: () => getGame(id),
    retry: false,
    refetchOnWindowFocus: false,
  })
}

// useGameSessions pages the sessions of a game, 50 per page [UI-038].
export function useGameSessions(id: number): UseInfiniteQueryResult<InfiniteData<SessionListView>, Error> {
  return useInfiniteQuery({
    queryKey: libraryKeys.sessions(id),
    queryFn: ({ pageParam }) => gameSessions(id, pageParam),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      const loaded = loadedCount(allPages.map((p) => p.sessions))
      return loaded < lastPage.total ? loaded : undefined
    },
    retry: false,
    refetchOnWindowFocus: false,
  })
}

export function useCatalog(): UseQueryResult<CatalogView, Error> {
  return useQuery({
    queryKey: libraryKeys.catalog,
    queryFn: catalog,
    retry: false,
    refetchOnWindowFocus: false,
  })
}

// After a write, the game lists, the changed game and the catalog are reloaded once [UI-042].
export function useUpdateGame(): UseMutationResult<
  GameDetailView,
  Error,
  { id: number; patch: Record<string, unknown> }
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, patch }: { id: number; patch: Record<string, unknown> }) => updateGame(id, patch),
    onSuccess: async (_saved, { id }) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: libraryGamesKey }),
        qc.invalidateQueries({ queryKey: libraryKeys.game(id) }),
        qc.invalidateQueries({ queryKey: libraryKeys.catalog }),
      ])
    },
  })
}

export function useDeleteGame(): UseMutationResult<void, Error, number> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deleteGame(id),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: libraryGamesKey }),
        qc.invalidateQueries({ queryKey: libraryKeys.catalog }),
      ])
    },
  })
}

export function useAddPlatform(): UseMutationResult<PlatformView, Error, { name: string; category: string }> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, category }: { name: string; category: string }) => addPlatform(name, category),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: libraryGamesKey }),
        qc.invalidateQueries({ queryKey: libraryKeys.catalog }),
      ])
    },
  })
}
