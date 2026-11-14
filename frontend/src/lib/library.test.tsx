import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { InfiniteData } from '@tanstack/react-query'
import {
  libraryKeys,
  useAddPlatform,
  useCatalog,
  useDeleteGame,
  useGame,
  useGameSessions,
  useInfiniteGames,
  useUpdateGame,
} from './library'
import type {
  CatalogView,
  GameDetailView,
  GameListView,
  GameView,
  PlatformView,
  SessionItemView,
  SessionListView,
} from './library'
import { useCurrentSession, useReloadOnReconnect } from './games'
import { defaultQuery, withText } from './filters'
import type { GameStatus, LibraryQuery } from './filters'

// ---------------------------------------------------------------------------
// Fakes for window.go.main.App (the bindings of cmd/arcadia, phase 5)
// ---------------------------------------------------------------------------

type Fn = ReturnType<typeof vi.fn>
type FakeApp = {
  AddGame: Fn
  AddPlatform: Fn
  Catalog: Fn
  CurrentSession: Fn
  DeleteGame: Fn
  GameSessions: Fn
  GetGame: Fn
  ListGames: Fn
  UpdateGame: Fn
}

type ListArgs = { filter: { text?: string }; sort: string; offset: number; limit?: number }

// The fake agent returns at most AGENT_PAGE games per call, whatever limit the UI
// asks for. The UI must follow loaded/total, not the limit it sent.
const AGENT_PAGE = 3

let app: FakeApp
// Games held by the fake agent, in the order the agent returns them.
let library: GameView[]

function install() {
  library = []
  app = {
    AddGame: vi.fn(() => Promise.resolve(gameView())),
    AddPlatform: vi.fn((name: string, category: string) =>
      Promise.resolve({ platform: platformView({ name, category }), created: true }),
    ),
    Catalog: vi.fn(() => Promise.resolve(catalog())),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    DeleteGame: vi.fn(() => Promise.resolve()),
    GameSessions: vi.fn((id: number, offset: number, limit: number) =>
      Promise.resolve(sessionList({ offset, total: 0, sessions: [] }, id, limit)),
    ),
    GetGame: vi.fn((id: number) => Promise.resolve(detail({ id }))),
    ListGames: vi.fn((q: ListArgs) => Promise.resolve(pageFor(q))),
    UpdateGame: vi.fn((id: number, patch: Record<string, unknown>) =>
      Promise.resolve(detail({ id, ...patch })),
    ),
  }
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: app } }
}

function uninstall() {
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
}

function pageFor(q: ListArgs): GameListView {
  const text = q.filter.text?.toLowerCase()
  const matching = text ? library.filter((g) => g.title.toLowerCase().includes(text)) : library
  const size = Math.min(q.limit ?? AGENT_PAGE, AGENT_PAGE)
  return {
    games: matching.slice(q.offset, q.offset + size),
    total: matching.length,
    offset: q.offset,
  }
}

function sessionList(over: Partial<SessionListView>, _id: number, _limit: number): SessionListView {
  return { sessions: [], total: 0, offset: 0, ...over }
}

function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function gameView(over: Partial<GameView> = {}): GameView {
  return {
    id: 10,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'PC',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    status: 'not_played' as GameStatus,
    favorite: false,
    year: 2017,
    totalPlaySeconds: 0,
    playCount: 0,
    firstPlayedAt: '',
    lastPlayedAt: '',
    ...over,
  }
}

function games(n: number): GameView[] {
  return Array.from({ length: n }, (_, i) =>
    gameView({ id: i + 1, title: `Jogo ${String(i + 1).padStart(2, '0')}` }),
  )
}

function detail(over: Partial<GameDetailView> & { id?: number } = {}): GameDetailView {
  return {
    ...gameView(),
    description: '',
    completedAt: null,
    developers: [],
    publishers: [],
    genres: [],
    playModes: [],
    ...over,
  } as GameDetailView
}

function platformView(over: Partial<PlatformView> = {}): PlatformView {
  return {
    id: 2,
    name: 'Switch',
    category: 'handheld',
    emulatorId: null,
    seedKey: null,
    iconUrl: '',
    gameCount: 0,
    ...over,
  }
}

function catalog(over: Partial<CatalogView> = {}): CatalogView {
  return {
    platforms: [{ id: 1, name: 'PC', category: 'pc', emulatorId: null, seedKey: null, iconUrl: '', gameCount: 0 }],
    genres: [],
    companies: [],
    playModes: [],
    ...over,
  }
}

function sessionItem(over: Partial<SessionItemView> = {}): SessionItemView {
  return {
    sessionId: 1,
    gameId: 10,
    state: 'ended',
    startedAt: '2026-11-09T10:00:00Z',
    endedAt: '2026-11-09T11:00:00Z',
    activeSeconds: 3600,
    endReason: null,
    ...over,
  } as SessionItemView
}

function newClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function wrapperFor(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

// Ids of the games loaded so far, in order.
function loadedIds(data: { pages: GameListView[] } | undefined): number[] {
  return data?.pages.flatMap((p) => p.games.map((g) => g.id)) ?? []
}

beforeEach(() => {
  install()
})

afterEach(() => {
  uninstall()
})

// ---------------------------------------------------------------------------
// useInfiniteGames [UI-036]
// ---------------------------------------------------------------------------

describe('useInfiniteGames', () => {
  it('UI-036/getNextPageParam devolve loaded enquanto loaded < total e undefined depois', async () => {
    library = games(7)
    const qc = newClient()
    const { result } = renderHook(() => useInfiniteGames(defaultQuery), { wrapper: wrapperFor(qc) })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(loadedIds(result.current.data)).toEqual([1, 2, 3])
    expect(result.current.hasNextPage).toBe(true)

    await act(async () => {
      await result.current.fetchNextPage()
    })
    await waitFor(() => expect(loadedIds(result.current.data)).toEqual([1, 2, 3, 4, 5, 6]))
    await waitFor(() => expect(result.current.hasNextPage).toBe(true))

    await act(async () => {
      await result.current.fetchNextPage()
    })
    await waitFor(() => expect(loadedIds(result.current.data)).toEqual([1, 2, 3, 4, 5, 6, 7]))
    await waitFor(() => expect(result.current.hasNextPage).toBe(false))

    const calls = app.ListGames.mock.calls.length
    await act(async () => {
      await result.current.fetchNextPage()
    })
    expect(app.ListGames).toHaveBeenCalledTimes(calls)
  })

  it('UI-036/cada página pede offset = loaded, uma vez, sem repetir nem pular jogos', async () => {
    library = games(7)
    const qc = newClient()
    const { result } = renderHook(() => useInfiniteGames(defaultQuery), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    await act(async () => {
      await result.current.fetchNextPage()
    })
    await act(async () => {
      await result.current.fetchNextPage()
    })

    expect(app.ListGames.mock.calls.map((c) => (c[0] as ListArgs).offset)).toEqual([0, 3, 6])
    // The pages are checked in the QueryClient cache: the observer delivery is asynchronous.
    await waitFor(() =>
      expect(loadedIds(qc.getQueryData(libraryKeys.games(defaultQuery)))).toEqual([
        1, 2, 3, 4, 5, 6, 7,
      ]),
    )
    expect(new Set(loadedIds(qc.getQueryData(libraryKeys.games(defaultQuery)))).size).toBe(7)
  })

  it('UI-036/mudar a query cria uma nova chave e descarta as páginas carregadas', async () => {
    library = [
      gameView({ id: 1, title: 'Hollow Knight' }),
      gameView({ id: 2, title: 'Celeste' }),
      gameView({ id: 3, title: 'Hollow Shell' }),
      gameView({ id: 4, title: 'Hades' }),
    ]
    const qc = newClient()
    const q1: LibraryQuery = defaultQuery
    const q2: LibraryQuery = withText(defaultQuery, 'hollow')
    expect(libraryKeys.games(q2)).not.toEqual(libraryKeys.games(q1))

    const { result, rerender } = renderHook(({ q }: { q: LibraryQuery }) => useInfiniteGames(q), {
      wrapper: wrapperFor(qc),
      initialProps: { q: q1 },
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(loadedIds(result.current.data)).toEqual([1, 2, 3])

    rerender({ q: q2 })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(loadedIds(result.current.data)).toEqual([1, 3])
    expect(loadedIds(result.current.data)).not.toContain(4)
    expect(result.current.data?.pages).toHaveLength(1)

    const last = app.ListGames.mock.calls.at(-1)?.[0] as ListArgs
    expect(last.filter.text).toBe('hollow')
    expect(last.offset).toBe(0)
  })

  it('UI-044/a lista com erro é pedida de novo a cada 2 s', async () => {
    app.ListGames.mockRejectedValue('O agente não está respondendo. Tente de novo.')
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    try {
      renderHook(() => useInfiniteGames(defaultQuery), { wrapper: wrapperFor(newClient()) })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10)
      })
      expect(app.ListGames).toHaveBeenCalledTimes(1)

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1900)
      })
      expect(app.ListGames).toHaveBeenCalledTimes(1)

      await act(async () => {
        await vi.advanceTimersByTimeAsync(200)
      })
      expect(app.ListGames).toHaveBeenCalledTimes(2)

      await act(async () => {
        await vi.advanceTimersByTimeAsync(2000)
      })
      expect(app.ListGames).toHaveBeenCalledTimes(3)
    } finally {
      vi.useRealTimers()
    }
  })

  it('UI-009/reconexão recarrega games e a sessão atual', async () => {
    library = games(2)
    app.CurrentSession.mockResolvedValue(null)

    const qc = newClient()
    const { result, rerender } = renderHook(
      ({ connected }: { connected: boolean }) => {
        useReloadOnReconnect(connected)
        return { games: useInfiniteGames(defaultQuery), current: useCurrentSession() }
      },
      { wrapper: wrapperFor(qc), initialProps: { connected: false } },
    )
    await waitFor(() => expect(result.current.games.isSuccess).toBe(true))
    await waitFor(() => expect(result.current.current.isSuccess).toBe(true))
    const gamesCalls = app.ListGames.mock.calls.length
    const sessionCalls = app.CurrentSession.mock.calls.length

    rerender({ connected: true })
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBeGreaterThan(gamesCalls))
    await waitFor(() =>
      expect(app.CurrentSession.mock.calls.length).toBeGreaterThan(sessionCalls),
    )
  })
})

// ---------------------------------------------------------------------------
// Leituras de jogo, sessões e catálogo [UI-037, UI-038, IPC-049]
// ---------------------------------------------------------------------------

describe('leituras da biblioteca', () => {
  it('UI-037/useGame busca o detalhe pelo id e devolve GameDetailView', async () => {
    const d = detail({ id: 10, description: 'Metroidvania', genres: ['Ação'] })
    app.GetGame.mockResolvedValueOnce(d)
    const { result } = renderHook(() => useGame(10), { wrapper: wrapperFor(newClient()) })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(app.GetGame).toHaveBeenCalledWith(10)
    expect(result.current.data).toEqual(d)
  })

  it('UI-038/useGameSessions pede 50 por página e segue pelo offset', async () => {
    app.GameSessions.mockImplementation((id: number, offset: number, limit: number) =>
      Promise.resolve({
        sessions: Array.from({ length: limit }, (_, i) =>
          sessionItem({ sessionId: offset + i + 1, gameId: id }),
        ),
        total: 120,
        offset,
      }),
    )
    const qc = newClient()
    const { result } = renderHook(() => useGameSessions(10), { wrapper: wrapperFor(qc) })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(app.GameSessions).toHaveBeenNthCalledWith(1, 10, 0, 50)
    expect(result.current.hasNextPage).toBe(true)

    await act(async () => {
      await result.current.fetchNextPage()
    })
    expect(app.GameSessions).toHaveBeenNthCalledWith(2, 10, 50, 50)
    // Checked in the QueryClient cache: the observer delivery is asynchronous.
    await waitFor(() => {
      const cached = qc.getQueryData(libraryKeys.sessions(10)) as
        | InfiniteData<SessionListView>
        | undefined
      const ids = cached?.pages.flatMap((p) => p.sessions.map((s) => s.sessionId)) ?? []
      expect(ids).toHaveLength(100)
      expect(new Set(ids).size).toBe(100)
    })
  })

  it('IPC-049/useCatalog chama library.catalog e devolve o catálogo', async () => {
    const c = catalog({ genres: [{ id: 5, name: 'Ação' }], companies: [{ id: 9, name: 'Team Cherry' }] })
    app.Catalog.mockResolvedValueOnce(c)
    const { result } = renderHook(() => useCatalog(), { wrapper: wrapperFor(newClient()) })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(app.Catalog).toHaveBeenCalledTimes(1)
    expect(result.current.data).toEqual(c)
  })
})

// ---------------------------------------------------------------------------
// Mutações [UI-039, UI-041, UI-042, EDT-008]
// ---------------------------------------------------------------------------

describe('mutações da biblioteca', () => {
  it('UI-042/useUpdateGame concluído invalida games, game e catalog', async () => {
    library = games(3)
    const qc = newClient()
    qc.setQueryData(libraryKeys.games(defaultQuery), {
      pages: [{ games: library, total: 3, offset: 0 }],
      pageParams: [0],
    })
    qc.setQueryData(libraryKeys.game(10), detail({ id: 10 }))
    qc.setQueryData(libraryKeys.game(99), detail({ id: 99 }))
    qc.setQueryData(libraryKeys.catalog, catalog())
    const { result } = renderHook(() => useUpdateGame(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate({ id: 10, patch: { title: 'Hollow Knight GOTY' } })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(qc.getQueryState(libraryKeys.games(defaultQuery))?.isInvalidated).toBe(true)
    expect(qc.getQueryState(libraryKeys.game(10))?.isInvalidated).toBe(true)
    expect(qc.getQueryState(libraryKeys.catalog)?.isInvalidated).toBe(true)
    expect(qc.getQueryState(libraryKeys.game(99))?.isInvalidated).toBe(false)
  })

  it('UI-042/useDeleteGame e useAddPlatform também invalidam games e catalog', async () => {
    library = games(3)

    const qcDelete = newClient()
    qcDelete.setQueryData(libraryKeys.games(defaultQuery), {
      pages: [{ games: library, total: 3, offset: 0 }],
      pageParams: [0],
    })
    qcDelete.setQueryData(libraryKeys.catalog, catalog())
    const del = renderHook(() => useDeleteGame(), { wrapper: wrapperFor(qcDelete) })
    act(() => {
      del.result.current.mutate(10)
    })
    await waitFor(() => expect(del.result.current.isSuccess).toBe(true))
    expect(qcDelete.getQueryState(libraryKeys.games(defaultQuery))?.isInvalidated).toBe(true)
    expect(qcDelete.getQueryState(libraryKeys.catalog)?.isInvalidated).toBe(true)

    const qcAdd = newClient()
    qcAdd.setQueryData(libraryKeys.games(defaultQuery), {
      pages: [{ games: library, total: 3, offset: 0 }],
      pageParams: [0],
    })
    qcAdd.setQueryData(libraryKeys.catalog, catalog())
    const add = renderHook(() => useAddPlatform(), { wrapper: wrapperFor(qcAdd) })
    act(() => {
      add.result.current.mutate({ name: 'Switch', category: 'handheld' })
    })
    await waitFor(() => expect(add.result.current.isSuccess).toBe(true))
    expect(qcAdd.getQueryState(libraryKeys.games(defaultQuery))?.isInvalidated).toBe(true)
    expect(qcAdd.getQueryState(libraryKeys.catalog)?.isInvalidated).toBe(true)
  })

  it('UI-039/useUpdateGame envia o patch recebido sem acrescentar campos e preserva null', async () => {
    const qc = newClient()
    const { result } = renderHook(() => useUpdateGame(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate({ id: 10, patch: { completedAt: null, year: 2001 } })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(app.UpdateGame).toHaveBeenCalledTimes(1)
    expect(app.UpdateGame.mock.calls[0]).toStrictEqual([10, { completedAt: null, year: 2001 }])
  })

  it('UI-039/isPending fica verdadeiro durante o envio e o sucesso devolve o detalhe', async () => {
    const pending = deferred<GameDetailView>()
    app.UpdateGame.mockReturnValueOnce(pending.promise)
    const { result } = renderHook(() => useUpdateGame(), { wrapper: wrapperFor(newClient()) })

    act(() => {
      result.current.mutate({ id: 10, patch: { title: 'Novo' } })
    })
    await waitFor(() => expect(result.current.isPending).toBe(true))

    const saved = detail({ id: 10, title: 'Novo' })
    await act(async () => {
      pending.resolve(saved)
      await pending.promise
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.isPending).toBe(false)
    expect(result.current.data).toEqual(saved)
  })

  it('UI-039/erro de UpdateGame vira Error e não invalida o detalhe nem a lista', async () => {
    const qc = newClient()
    const current = detail({ id: 10 })
    qc.setQueryData(libraryKeys.game(10), current)
    app.UpdateGame.mockRejectedValueOnce('Título não pode ficar vazio.')
    const { result } = renderHook(() => useUpdateGame(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate({ id: 10, patch: { title: '' } })
    })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('Título não pode ficar vazio.')
    expect(qc.getQueryData(libraryKeys.game(10))).toEqual(current)
    expect(qc.getQueryState(libraryKeys.game(10))?.isInvalidated).toBe(false)
  })

  it('UI-041/useDeleteGame chama DeleteGame uma vez com o id', async () => {
    const { result } = renderHook(() => useDeleteGame(), { wrapper: wrapperFor(newClient()) })

    act(() => {
      result.current.mutate(10)
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(app.DeleteGame).toHaveBeenCalledTimes(1)
    expect(app.DeleteGame).toHaveBeenCalledWith(10)
  })

  it('EDT-008/useAddPlatform chama platform.add com name e category e devolve o PlatformView', async () => {
    const { result } = renderHook(() => useAddPlatform(), { wrapper: wrapperFor(newClient()) })

    act(() => {
      result.current.mutate({ name: 'Switch', category: 'handheld' })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(app.AddPlatform).toHaveBeenCalledWith('Switch', 'handheld')
    expect(result.current.data?.platform).toEqual(platformView({ name: 'Switch', category: 'handheld' }))
  })

  it('PLT-016/useAddPlatform devolve platform e created', async () => {
    const { result } = renderHook(() => useAddPlatform(), { wrapper: wrapperFor(newClient()) })

    act(() => {
      result.current.mutate({ name: 'Switch', category: 'handheld' })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual({
      platform: platformView({ name: 'Switch', category: 'handheld' }),
      created: true,
    })
  })

  it('PLT-016/nome que já existe devolve a plataforma existente com created false', async () => {
    const existing = platformView({ id: 7, name: 'Nintendo Switch', category: 'console', seedKey: 'switch' })
    app.AddPlatform.mockResolvedValueOnce({ platform: existing, created: false })
    const { result } = renderHook(() => useAddPlatform(), { wrapper: wrapperFor(newClient()) })

    act(() => {
      result.current.mutate({ name: 'nintendo switch', category: 'handheld' })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual({ platform: existing, created: false })
  })
})

// ---------------------------------------------------------------------------
// Rejeições em string viram Error [UI-008]
// ---------------------------------------------------------------------------

describe('erros da biblioteca', () => {
  it('UI-008/rejeição em string vira Error com a mesma mensagem nas leituras e escritas', async () => {
    const msg = 'O agente não está respondendo. Tente de novo.'
    app.ListGames.mockRejectedValue(msg)
    app.GetGame.mockRejectedValue(msg)
    app.GameSessions.mockRejectedValue(msg)
    app.Catalog.mockRejectedValue(msg)
    app.UpdateGame.mockRejectedValue(msg)
    app.DeleteGame.mockRejectedValue(msg)
    app.AddPlatform.mockRejectedValue(msg)
    const qc = newClient()
    const wrapper = wrapperFor(qc)

    const list = renderHook(() => useInfiniteGames(defaultQuery), { wrapper })
    const game = renderHook(() => useGame(10), { wrapper })
    const sessions = renderHook(() => useGameSessions(10), { wrapper })
    const catalogHook = renderHook(() => useCatalog(), { wrapper })
    for (const r of [list, game, sessions, catalogHook]) {
      await waitFor(() => expect(r.result.current.isError).toBe(true))
      expect(r.result.current.error).toBeInstanceOf(Error)
      expect(r.result.current.error?.message).toBe(msg)
    }

    const update = renderHook(() => useUpdateGame(), { wrapper })
    const del = renderHook(() => useDeleteGame(), { wrapper })
    const add = renderHook(() => useAddPlatform(), { wrapper })
    act(() => {
      update.result.current.mutate({ id: 10, patch: { title: 'X' } })
      del.result.current.mutate(10)
      add.result.current.mutate({ name: 'Switch', category: 'handheld' })
    })
    for (const r of [update, del, add]) {
      await waitFor(() => expect(r.result.current.isError).toBe(true))
      expect(r.result.current.error).toBeInstanceOf(Error)
      expect(r.result.current.error?.message).toBe(msg)
    }
  })
})
