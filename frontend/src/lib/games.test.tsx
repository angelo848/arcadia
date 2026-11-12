import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query'
import {
  addGame,
  currentSession,
  currentSessionKey,
  formatPlayTime,
  launchGame,
  launchGameUntracked,
  pauseSession,
  resumeSession,
  sessionEventSeq,
  useCurrentSession,
  useLaunchGame,
  useLaunchGameUntracked,
  useReloadOnReconnect,
  useSessionEvents,
  useToggleSession,
} from './games'
import type { SessionView } from './games'
import { resetQuery } from './filters'
import { libraryKeys, useInfiniteGames, type GameListView, type GameView } from './library'

// ---------------------------------------------------------------------------
// Fakes for window.go.main.App and window.runtime.EventsOn
// ---------------------------------------------------------------------------

type Fn = ReturnType<typeof vi.fn>
type FakeApp = {
  AddGame: Fn
  AgentStatus: Fn
  CurrentSession: Fn
  LaunchGame: Fn
  LaunchGameUntracked: Fn
  ListGames: Fn
  PauseSession: Fn
  PickGameExe: Fn
  ResumeSession: Fn
}

let app: FakeApp
let handlers: Map<string, Set<(...data: unknown[]) => void>>

function install() {
  handlers = new Map()
  app = {
    AddGame: vi.fn(() => Promise.resolve(game())),
    AgentStatus: vi.fn(() => Promise.resolve({ connected: true })),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    LaunchGame: vi.fn(() => Promise.resolve(session())),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    ListGames: vi.fn(() => Promise.resolve(listPage([]))),
    PauseSession: vi.fn(() => Promise.resolve(session({ state: 'paused' }))),
    PickGameExe: vi.fn(() => Promise.resolve({ path: '', title: '' })),
    ResumeSession: vi.fn(() => Promise.resolve(session({ state: 'running' }))),
  }
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: app } }
  w.runtime = {
    EventsOn: vi.fn((name: string, cb: (...data: unknown[]) => void) => {
      let set = handlers.get(name)
      if (!set) {
        set = new Set()
        handlers.set(name, set)
      }
      set.add(cb)
      return () => {
        set.delete(cb)
      }
    }),
  }
}

function uninstall() {
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
}

function emit(name: string, ...data: unknown[]) {
  act(() => {
    for (const cb of handlers.get(name) ?? []) {
      cb(...data)
    }
  })
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

// The library list is the infinite query under libraryKeys.games; its pages are GameListView [UI-036].
const listKey = libraryKeys.games(resetQuery())

function listPage(games: GameView[]): GameListView {
  return { games, total: games.length, offset: 0 }
}

function game(over: Partial<GameView> = {}): GameView {
  return {
    id: 10,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'Windows',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    status: 'not_played',
    favorite: false,
    year: null,
    totalPlaySeconds: 0,
    playCount: 0,
    firstPlayedAt: '',
    lastPlayedAt: '',
    ...over,
  }
}

function session(over: Partial<SessionView> = {}): SessionView {
  return {
    sessionId: 1,
    gameId: 10,
    title: 'Hollow Knight',
    state: 'running',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '',
    activeSeconds: 0,
    activeMs: 0,
    asOf: '2026-11-09T10:00:00.000Z',
    ...over,
  }
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

beforeEach(() => {
  install()
})

afterEach(() => {
  uninstall()
})

// ---------------------------------------------------------------------------
// formatPlayTime [UI-005]
// ---------------------------------------------------------------------------

describe('formatPlayTime', () => {
  const table: Array<{ name: string; cases: Array<[number, string]> }> = [
    { name: 'UI-005 0 é Nunca jogado', cases: [[0, 'Nunca jogado']] },
    {
      name: 'UI-005 1 e 59 são < 1 min',
      cases: [
        [1, '< 1 min'],
        [59, '< 1 min'],
      ],
    },
    {
      name: 'UI-005 60 é 1 min e 3599 é 59 min',
      cases: [
        [60, '1 min'],
        [3599, '59 min'],
      ],
    },
    {
      name: 'UI-005 3600 é 1 h 00 min, 3900 é 1 h 05 min e 451800 é 125 h 30 min',
      cases: [
        [3600, '1 h 00 min'],
        [3900, '1 h 05 min'],
        [451800, '125 h 30 min'],
      ],
    },
  ]
  for (const { name, cases } of table) {
    it(name, () => {
      for (const [input, want] of cases) {
        expect(formatPlayTime(input), `formatPlayTime(${input})`).toBe(want)
      }
    })
  }
})

// ---------------------------------------------------------------------------
// Plain binding wrappers
// ---------------------------------------------------------------------------

describe('bindings', () => {
  it('UI-006 currentSession devolve null quando não há sessão', async () => {
    app.CurrentSession.mockResolvedValueOnce(null)
    await expect(currentSession()).resolves.toBeNull()

    const s = session({ state: 'grace', activeMs: 1234 })
    app.CurrentSession.mockResolvedValueOnce(s)
    await expect(currentSession()).resolves.toEqual(s)
  })

  it('UI-008 rejeição em string vira Error com a mesma mensagem', async () => {
    const msg = 'O agente não está respondendo. Tente de novo.'
    app.AddGame.mockRejectedValueOnce(msg)
    app.LaunchGame.mockRejectedValueOnce(msg)
    app.CurrentSession.mockRejectedValueOnce(msg)

    for (const p of [addGame('C:\\x.exe', 'X'), launchGame(10), currentSession()]) {
      const err = await p.then(
        () => null,
        (e: unknown) => e,
      )
      expect(err).toBeInstanceOf(Error)
      expect((err as Error).message).toBe(msg)
    }
    expect(app.AddGame).toHaveBeenCalledWith('C:\\x.exe', 'X')
    expect(app.LaunchGame).toHaveBeenCalledWith(10)
  })
})

// ---------------------------------------------------------------------------
// useSessionEvents [UI-006]
// ---------------------------------------------------------------------------

describe('useSessionEvents', () => {
  it('UI-006 running e grace gravam a sessão no cache', () => {
    const qc = newClient()
    renderHook(() => useSessionEvents(), { wrapper: wrapperFor(qc) })

    const running = session({ state: 'running', activeMs: 5000 })
    emit('session:changed', running)
    expect(qc.getQueryData(currentSessionKey)).toEqual(running)

    const grace = session({ state: 'grace', activeMs: 7000 })
    emit('session:changed', grace)
    expect(qc.getQueryData(currentSessionKey)).toEqual(grace)
  })

  for (const state of ['ended', 'discarded'] as const) {
    it(`UI-006 ended e discarded limpam a sessão e invalidam games (${state})`, () => {
      const qc = newClient()
      qc.setQueryData(listKey, { pages: [listPage([game()])], pageParams: [0] })
      qc.setQueryData(currentSessionKey, session())
      renderHook(() => useSessionEvents(), { wrapper: wrapperFor(qc) })
      expect(qc.getQueryState(listKey)?.isInvalidated).toBe(false)

      emit('session:changed', session({ state, endedAt: '2026-11-09T10:30:00.000Z' }))

      expect(qc.getQueryData(currentSessionKey)).toBeNull()
      expect(qc.getQueryState(listKey)?.isInvalidated).toBe(true)
    })
  }

  it('UI-006 desmontar cancela a assinatura de session:changed', () => {
    const qc = newClient()
    const { unmount } = renderHook(() => useSessionEvents(), { wrapper: wrapperFor(qc) })
    expect(handlers.get('session:changed')?.size ?? 0).toBe(1)
    unmount()
    expect(handlers.get('session:changed')?.size ?? 0).toBe(0)
  })
})

// ---------------------------------------------------------------------------
// useCurrentSession / event counter [UI-018]
// ---------------------------------------------------------------------------

describe('useCurrentSession', () => {
  it('UI-018 evento ended antes da resposta running atrasada mantém null', async () => {
    const qc = newClient()
    const pending = deferred<SessionView | null>()
    app.CurrentSession.mockReturnValueOnce(pending.promise)

    const { result } = renderHook(
      () => {
        useSessionEvents()
        return useCurrentSession()
      },
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalledTimes(1))

    emit('session:changed', session({ state: 'ended', endedAt: '2026-11-09T10:30:00.000Z' }))
    await act(async () => {
      pending.resolve(session({ state: 'running', activeMs: 99000 }))
      await pending.promise
    })

    await waitFor(() => expect(result.current.isFetching).toBe(false))
    expect(result.current.data).toBeNull()
    expect(qc.getQueryData(currentSessionKey)).toBeNull()
  })

  it('UI-018 sem evento a resposta é aplicada', async () => {
    const qc = newClient()
    const pending = deferred<SessionView | null>()
    app.CurrentSession.mockReturnValueOnce(pending.promise)

    const { result } = renderHook(
      () => {
        useSessionEvents()
        return useCurrentSession()
      },
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalledTimes(1))

    const s = session({ state: 'running', activeMs: 99000 })
    await act(async () => {
      pending.resolve(s)
      await pending.promise
    })

    await waitFor(() => expect(result.current.data).toEqual(s))
  })

  it('UI-018 contador de eventos é por QueryClient e sobe a cada session:changed', () => {
    const a = newClient()
    const b = newClient()
    const startA = sessionEventSeq(a)
    const startB = sessionEventSeq(b)

    renderHook(() => useSessionEvents(), { wrapper: wrapperFor(a) })
    emit('session:changed', session())
    emit('session:changed', session({ state: 'grace' }))

    expect(sessionEventSeq(a)).toBe(startA + 2)
    expect(sessionEventSeq(b)).toBe(startB)
  })

  it('UI-006 foco ou visibilidade da janela não chamam CurrentSession nem ListGames de novo', async () => {
    const qc = newClient()
    app.ListGames.mockResolvedValue(listPage([game()]))
    app.CurrentSession.mockResolvedValue(session())
    const { result } = renderHook(() => ({ games: useInfiniteGames(resetQuery()), current: useCurrentSession() }), {
      wrapper: wrapperFor(qc),
    })
    await waitFor(() => expect(result.current.games.isSuccess).toBe(true))
    await waitFor(() => expect(result.current.current.isSuccess).toBe(true))
    const gamesCalls = app.ListGames.mock.calls.length
    const sessionCalls = app.CurrentSession.mock.calls.length

    try {
      await act(async () => {
        focusManager.setFocused(false)
        focusManager.setFocused(true)
        await Promise.resolve()
      })
      await act(async () => {
        focusManager.setFocused(undefined)
        window.dispatchEvent(new Event('visibilitychange'))
        window.dispatchEvent(new Event('focus'))
        await Promise.resolve()
      })
      // Let any refetch the focus would have started reach the bindings.
      await act(async () => {
        await new Promise((r) => setTimeout(r, 20))
      })
    } finally {
      focusManager.setFocused(undefined)
    }

    expect(app.ListGames).toHaveBeenCalledTimes(gamesCalls)
    expect(app.CurrentSession).toHaveBeenCalledTimes(sessionCalls)
  })

  it('UI-006 useCurrentSession não faz polling', async () => {
    const qc = newClient()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    try {
      renderHook(() => useCurrentSession(), { wrapper: wrapperFor(qc) })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10)
      })
      expect(app.CurrentSession).toHaveBeenCalledTimes(1)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(60_000)
      })
      expect(app.CurrentSession).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })
})

// ---------------------------------------------------------------------------
// useLaunchGame [UI-018]
// ---------------------------------------------------------------------------

describe('useLaunchGame', () => {
  it('UI-018 resultado de LaunchGame ignorado se chegou session:changed depois do pedido', async () => {
    const qc = newClient()
    const pending = deferred<SessionView>()
    app.LaunchGame.mockReturnValueOnce(pending.promise)

    const { result } = renderHook(
      () => {
        useSessionEvents()
        return useLaunchGame()
      },
      { wrapper: wrapperFor(qc) },
    )

    act(() => {
      result.current.mutate(10)
    })
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(10))

    // The event arrives while LaunchGame is pending: it wins.
    emit('session:changed', session({ state: 'ended', endedAt: '2026-11-09T10:00:05.000Z' }))
    await act(async () => {
      pending.resolve(session({ state: 'running', activeMs: 0 }))
      await pending.promise
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(qc.getQueryData(currentSessionKey)).toBeNull()
  })

  it('UI-018 sem session:changed o resultado de LaunchGame vai para o cache', async () => {
    const qc = newClient()
    const s = session({ sessionId: 7, state: 'running', activeMs: 0 })
    app.LaunchGame.mockResolvedValueOnce(s)

    const { result } = renderHook(
      () => {
        useSessionEvents()
        return useLaunchGame()
      },
      { wrapper: wrapperFor(qc) },
    )
    act(() => {
      result.current.mutate(10)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(qc.getQueryData(currentSessionKey)).toEqual(s)
  })

  it('UI-008 falha de LaunchGame vira Error com a mensagem do binding', async () => {
    const qc = newClient()
    app.LaunchGame.mockRejectedValueOnce('O jogo já está em execução.')
    const { result } = renderHook(() => useLaunchGame(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate(10)
    })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('O jogo já está em execução.')
  })
})

// ---------------------------------------------------------------------------
// useReloadOnReconnect [UI-009, UI-016]
// ---------------------------------------------------------------------------

describe('useReloadOnReconnect', () => {
  function useAll(connected: boolean) {
    useReloadOnReconnect(connected)
    return { games: useInfiniteGames(resetQuery()), current: useCurrentSession() }
  }

  it('UI-009 Desconectado para Conectado recarrega games e a sessão', async () => {
    const qc = newClient()
    app.ListGames.mockResolvedValue(listPage([game()]))
    app.CurrentSession.mockResolvedValue(null)

    const { result, rerender } = renderHook(({ connected }) => useAll(connected), {
      wrapper: wrapperFor(qc),
      initialProps: { connected: false },
    })
    await waitFor(() => expect(result.current.games.isSuccess).toBe(true))
    await waitFor(() => expect(result.current.current.isSuccess).toBe(true))
    const gamesCalls = app.ListGames.mock.calls.length
    const sessionCalls = app.CurrentSession.mock.calls.length

    // Staying disconnected does not reload.
    rerender({ connected: false })
    expect(app.ListGames).toHaveBeenCalledTimes(gamesCalls)
    expect(app.CurrentSession).toHaveBeenCalledTimes(sessionCalls)

    rerender({ connected: true })
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBeGreaterThan(gamesCalls))
    await waitFor(() =>
      expect(app.CurrentSession.mock.calls.length).toBeGreaterThan(sessionCalls),
    )
  })

  it('UI-009 sem observadores invalida games e reseta a sessão', () => {
    const qc = newClient()
    qc.setQueryData(listKey, { pages: [listPage([game()])], pageParams: [0] })
    qc.setQueryData(currentSessionKey, session())

    const { rerender } = renderHook(({ connected }) => useReloadOnReconnect(connected), {
      wrapper: wrapperFor(qc),
      initialProps: { connected: false },
    })
    rerender({ connected: true })

    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(true)
    expect(qc.getQueryData(currentSessionKey)).toBeUndefined()
  })

  it('UI-016 Conectado para Desconectado já reseta a sessão em cache', () => {
    const qc = newClient()
    qc.setQueryData(listKey, { pages: [listPage([game()])], pageParams: [0] })
    qc.setQueryData(currentSessionKey, session({ activeMs: 60000 }))

    const { rerender } = renderHook(({ connected }) => useReloadOnReconnect(connected), {
      wrapper: wrapperFor(qc),
      initialProps: { connected: true },
    })
    // Staying connected keeps the cached session.
    rerender({ connected: true })
    expect(qc.getQueryData(currentSessionKey)).toEqual(session({ activeMs: 60000 }))

    rerender({ connected: false })
    // The reconnect render must find an empty cache (design, "Revisão da T14").
    expect(qc.getQueryData(currentSessionKey)).toBeUndefined()
  })

  it('UI-016 a sessão antiga é descartada até a recarga', async () => {
    const qc = newClient()
    const old = session({ sessionId: 1, activeMs: 60000 })
    const fresh = session({ sessionId: 1, activeMs: 180000 })
    const reload = deferred<SessionView | null>()
    app.ListGames.mockResolvedValue(listPage([game()]))
    app.CurrentSession.mockResolvedValueOnce(old).mockReturnValueOnce(reload.promise)

    const { result, rerender } = renderHook(({ connected }) => useAll(connected), {
      wrapper: wrapperFor(qc),
      initialProps: { connected: false },
    })
    await waitFor(() => expect(result.current.current.data).toEqual(old))

    rerender({ connected: true })
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalledTimes(2))
    // While the reload is pending the old value must not be shown.
    expect(result.current.current.data).toBeUndefined()

    await act(async () => {
      reload.resolve(fresh)
      await reload.promise
    })
    await waitFor(() => expect(result.current.current.data).toEqual(fresh))
  })

  it('UI-016/reconexão com sessão paused zera o cache e recarrega', async () => {
    const qc = newClient()
    const old = session({ state: 'paused', activeMs: 100000 })
    const fresh = session({ state: 'paused', activeMs: 100000 })
    const reload = deferred<SessionView | null>()
    app.ListGames.mockResolvedValue(listPage([game()]))
    app.CurrentSession.mockResolvedValueOnce(old).mockReturnValueOnce(reload.promise)

    const { result, rerender } = renderHook(({ connected }) => useAll(connected), {
      wrapper: wrapperFor(qc),
      initialProps: { connected: false },
    })
    await waitFor(() => expect(result.current.current.data).toEqual(old))

    rerender({ connected: true })
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalledTimes(2))
    // The paused value from before the disconnect must not be shown while reloading.
    expect(result.current.current.data).toBeUndefined()
    expect(qc.getQueryData(currentSessionKey)).toBeUndefined()

    await act(async () => {
      reload.resolve(fresh)
      await reload.promise
    })
    await waitFor(() => expect(result.current.current.data).toEqual(fresh))
  })
})

// ---------------------------------------------------------------------------
// Sessão paused no cache [UI-022, UI-024]
// ---------------------------------------------------------------------------

describe('sessão paused no cache', () => {
  it('UI-022/session:changed com paused entra no cache como sessão viva, sem recarregar a lista', () => {
    const qc = newClient()
    qc.setQueryData(listKey, { pages: [listPage([game()])], pageParams: [0] })
    renderHook(() => useSessionEvents(), { wrapper: wrapperFor(qc) })

    const paused = session({ state: 'paused', activeMs: 100000 })
    emit('session:changed', paused)

    expect(qc.getQueryData(currentSessionKey)).toEqual(paused)
    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(false)
  })

  it('UI-022/currentSession devolve a sessão paused no formato recebido', async () => {
    const paused = session({ state: 'paused', activeMs: 100000 })
    app.CurrentSession.mockResolvedValueOnce(paused)
    await expect(currentSession()).resolves.toEqual(paused)
  })

  it('UI-024/running, paused e grace mantêm a lista sem recarga; ended depois de paused a recarrega', () => {
    const qc = newClient()
    qc.setQueryData(listKey, { pages: [listPage([game()])], pageParams: [0] })
    renderHook(() => useSessionEvents(), { wrapper: wrapperFor(qc) })

    emit('session:changed', session({ state: 'running', activeMs: 1000 }))
    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(false)

    const paused = session({ state: 'paused', activeMs: 1000 })
    emit('session:changed', paused)
    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(false)
    expect(qc.getQueryData(currentSessionKey)).toEqual(paused)

    emit('session:changed', session({ state: 'grace', activeMs: 1000 }))
    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(false)

    emit('session:changed', session({ state: 'paused', activeMs: 1000 }))
    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(false)

    emit('session:changed', session({ state: 'ended', activeMs: 1000, endedAt: '2026-11-09T10:30:00.000Z' }))
    expect(qc.getQueryData(currentSessionKey)).toBeNull()
    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(true)
  })

  for (const state of ['ended', 'discarded'] as const) {
    it(`UI-024/${state} vindo de paused limpa a sessão e invalida games`, () => {
      const qc = newClient()
      qc.setQueryData(listKey, { pages: [listPage([game()])], pageParams: [0] })
      qc.setQueryData(currentSessionKey, session({ state: 'paused', activeMs: 100000 }))
      renderHook(() => useSessionEvents(), { wrapper: wrapperFor(qc) })

      emit('session:changed', session({ state, activeMs: 100000, endedAt: '2026-11-09T10:30:00.000Z' }))

      expect(qc.getQueryData(currentSessionKey)).toBeNull()
      expect(qc.getQueryState(listKey)?.isInvalidated).toBe(true)
    })
  }
})

// ---------------------------------------------------------------------------
// pauseSession, resumeSession e launchGameUntracked (bindings) [UI-020, UI-028]
// ---------------------------------------------------------------------------

describe('bindings de pausa e lançamento sem contar tempo', () => {
  it('UI-028/pauseSession e resumeSession chamam o binding sem argumentos e devolvem a sessão', async () => {
    const paused = session({ state: 'paused', activeMs: 100000 })
    const running = session({ state: 'running', activeMs: 100000 })
    app.PauseSession.mockResolvedValueOnce(paused)
    app.ResumeSession.mockResolvedValueOnce(running)

    await expect(pauseSession()).resolves.toEqual(paused)
    expect(app.PauseSession).toHaveBeenCalledWith()
    await expect(resumeSession()).resolves.toEqual(running)
    expect(app.ResumeSession).toHaveBeenCalledWith()
  })

  it('UI-028/rejeição em string de PauseSession e ResumeSession vira Error com a mesma mensagem', async () => {
    const msg = 'Sessão não pode ser pausada agora.'
    app.PauseSession.mockRejectedValueOnce(msg)
    app.ResumeSession.mockRejectedValueOnce(msg)

    for (const p of [pauseSession(), resumeSession()]) {
      const err = await p.then(
        () => null,
        (e: unknown) => e,
      )
      expect(err).toBeInstanceOf(Error)
      expect((err as Error).message).toBe(msg)
    }
  })

  it('UI-020/launchGameUntracked chama LaunchGameUntracked com o id e resolve sem valor', async () => {
    await expect(launchGameUntracked(10)).resolves.toBeUndefined()
    expect(app.LaunchGameUntracked).toHaveBeenCalledWith(10)
    expect(app.LaunchGame).not.toHaveBeenCalled()
  })

  it('UI-020/rejeição em string de LaunchGameUntracked (sessão ativa, -32002) vira Error com a mensagem', async () => {
    const msg = 'Já existe uma sessão ativa.'
    app.LaunchGameUntracked.mockRejectedValueOnce(msg)

    const err = await launchGameUntracked(10).then(
      () => null,
      (e: unknown) => e,
    )
    expect(err).toBeInstanceOf(Error)
    expect((err as Error).message).toBe(msg)
  })
})

// ---------------------------------------------------------------------------
// useLaunchGameUntracked [UI-020]
// ---------------------------------------------------------------------------

describe('useLaunchGameUntracked', () => {
  it('UI-020/useLaunchGameUntracked chama LaunchGameUntracked uma vez e não toca no cache de sessão nem recarrega a lista', async () => {
    const qc = newClient()
    app.ListGames.mockResolvedValue(listPage([game()]))
    app.CurrentSession.mockResolvedValue(null)

    const { result } = renderHook(
      () => ({
        games: useInfiniteGames(resetQuery()),
        current: useCurrentSession(),
        launch: useLaunchGameUntracked(),
      }),
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(result.current.games.isSuccess).toBe(true))
    await waitFor(() => expect(result.current.current.isSuccess).toBe(true))
    const gamesCalls = app.ListGames.mock.calls.length
    const sessionCalls = app.CurrentSession.mock.calls.length

    act(() => {
      result.current.launch.mutate(10)
    })
    await waitFor(() => expect(result.current.launch.isSuccess).toBe(true))

    expect(app.LaunchGameUntracked).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).toHaveBeenCalledWith(10)
    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(app.ListGames).toHaveBeenCalledTimes(gamesCalls)
    expect(app.CurrentSession).toHaveBeenCalledTimes(sessionCalls)
    expect(qc.getQueryState(listKey)?.isInvalidated).toBe(false)
    expect(qc.getQueryData(currentSessionKey)).toBeNull()
  })

  it('UI-020/erro de LaunchGameUntracked vira Error na mutation e não muda o cache de sessão', async () => {
    const qc = newClient()
    const running = session({ state: 'running', activeMs: 5000 })
    qc.setQueryData(currentSessionKey, running)
    app.LaunchGameUntracked.mockRejectedValueOnce('Já existe uma sessão ativa.')
    const { result } = renderHook(() => useLaunchGameUntracked(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate(10)
    })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('Já existe uma sessão ativa.')
    expect(qc.getQueryData(currentSessionKey)).toEqual(running)
  })
})

// ---------------------------------------------------------------------------
// useToggleSession [UI-028, UI-018]
// ---------------------------------------------------------------------------

describe('useToggleSession', () => {
  it('UI-028/useToggleSession chama PauseSession em running e ResumeSession em paused, uma vez cada', async () => {
    const qc = newClient()
    qc.setQueryData(currentSessionKey, session({ state: 'running', activeMs: 0 }))
    const { result } = renderHook(() => useToggleSession(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate()
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(app.PauseSession).toHaveBeenCalledTimes(1)
    expect(app.ResumeSession).not.toHaveBeenCalled()

    qc.setQueryData(currentSessionKey, session({ state: 'paused', activeMs: 0 }))
    act(() => {
      result.current.mutate()
    })
    await waitFor(() => expect(app.ResumeSession).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(app.PauseSession).toHaveBeenCalledTimes(1)
    expect(app.ResumeSession).toHaveBeenCalledTimes(1)
  })

  it('UI-028/o cache não muda antes da resposta de PauseSession', async () => {
    const qc = newClient()
    const running = session({ state: 'running', activeMs: 5000 })
    const paused = session({ state: 'paused', activeMs: 5000 })
    qc.setQueryData(currentSessionKey, running)
    const pending = deferred<SessionView>()
    app.PauseSession.mockReturnValueOnce(pending.promise)
    const { result } = renderHook(() => useToggleSession(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate()
    })
    await waitFor(() => expect(app.PauseSession).toHaveBeenCalledTimes(1))
    expect(qc.getQueryData(currentSessionKey)).toEqual(running)

    await act(async () => {
      pending.resolve(paused)
      await pending.promise
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(qc.getQueryData(currentSessionKey)).toEqual(paused)
  })

  it('UI-018/resposta de pauseSession atrasada, depois de um session:changed, não sobrescreve o cache (caso paused)', async () => {
    const qc = newClient()
    qc.setQueryData(currentSessionKey, session({ state: 'running', activeMs: 40000 }))
    const pending = deferred<SessionView>()
    app.PauseSession.mockReturnValueOnce(pending.promise)
    const { result } = renderHook(
      () => {
        useSessionEvents()
        return useToggleSession()
      },
      { wrapper: wrapperFor(qc) },
    )

    act(() => {
      result.current.mutate()
    })
    await waitFor(() => expect(app.PauseSession).toHaveBeenCalledTimes(1))

    const evt = session({ state: 'paused', activeMs: 50000 })
    emit('session:changed', evt)
    await act(async () => {
      pending.resolve(session({ state: 'paused', activeMs: 60000 }))
      await pending.promise
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(qc.getQueryData(currentSessionKey)).toEqual(evt)
  })

  it('UI-028/erro de PauseSession vira Error na mutation e o cache não muda', async () => {
    const qc = newClient()
    const running = session({ state: 'running', activeMs: 5000 })
    qc.setQueryData(currentSessionKey, running)
    app.PauseSession.mockRejectedValueOnce('Sessão não pode ser pausada agora.')
    const { result } = renderHook(() => useToggleSession(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate()
    })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('Sessão não pode ser pausada agora.')
    expect(qc.getQueryData(currentSessionKey)).toEqual(running)
  })

  it('UI-028/erro de ResumeSession vira Error na mutation e o cache não muda', async () => {
    const qc = newClient()
    const paused = session({ state: 'paused', activeMs: 100000 })
    qc.setQueryData(currentSessionKey, paused)
    app.ResumeSession.mockRejectedValueOnce('Sessão não pode ser retomada agora.')
    const { result } = renderHook(() => useToggleSession(), { wrapper: wrapperFor(qc) })

    act(() => {
      result.current.mutate()
    })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
    expect(result.current.error?.message).toBe('Sessão não pode ser retomada agora.')
    expect(qc.getQueryData(currentSessionKey)).toEqual(paused)
  })
})
