import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { libraryGamesKey, libraryKeys, useGame, useGameSessions } from './library'
import type { GameDetailView, SessionItemView } from './library'
import { statsKey } from './stats'
import {
  isSessionNotFound,
  sessionNotFoundMessage,
  useCreateSession,
  useDeleteSession,
  useUpdateSession,
} from './sessionEdit'

// Phase 16 (edicao-sessoes), SEL-013: the write hooks and the cache after a write.
type Fn = ReturnType<typeof vi.fn>
type FakeApp = { CreateSession: Fn; UpdateSession: Fn; DeleteSession: Fn; GetGame: Fn; GameSessions: Fn }

const GAME_ID = 10
let app: FakeApp

function game(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: GAME_ID,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'PC',
    path: 'C:\\Games\\HK\\hk.exe',
    status: 'playing',
    favorite: false,
    year: 2017,
    totalPlaySeconds: 3600,
    playCount: 1,
    firstPlayedAt: '',
    lastPlayedAt: '',
    description: '',
    completedAt: null,
    developers: [],
    publishers: [],
    genres: [],
    playModes: [],
    launchType: 'exe',
    launchArgs: '',
    emulatorId: null,
    sessionPlaySeconds: 3600,
    ...over,
  } as GameDetailView
}

function session(over: Partial<SessionItemView> = {}): SessionItemView {
  return {
    sessionId: 5,
    gameId: GAME_ID,
    state: 'ended',
    startedAt: '2026-11-25T18:00:00.000Z',
    endedAt: '2026-11-25T19:00:00.000Z',
    activeSeconds: 3600,
    endReason: null,
    source: 'manual',
    editedAt: '',
    ...over,
  }
}

const input = {
  startedAt: '2026-11-25T18:00:00.000Z',
  endedAt: '2026-11-25T19:00:00.000Z',
  activeSeconds: 3600,
  includedInTotal: false,
}

beforeEach(() => {
  app = {
    CreateSession: vi.fn(() => Promise.resolve({ session: session(), game: game({ totalPlaySeconds: 7200 }) })),
    UpdateSession: vi.fn(() => Promise.resolve({ session: session({ editedAt: 'x' }), game: game({ totalPlaySeconds: 5400 }) })),
    DeleteSession: vi.fn(() => Promise.resolve(game({ totalPlaySeconds: 0, sessionPlaySeconds: 0 }))),
    GetGame: vi.fn(() => Promise.resolve(game())),
    GameSessions: vi.fn(() => Promise.resolve({ sessions: [], total: 0, offset: 0 })),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
})

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  return { qc, wrapper }
}

describe('SEL-013 atualização depois de escrita', () => {
  it('SEL-013/useCreateSession chama CreateSession com o gameId e o input e devolve a resposta', async () => {
    const { wrapper } = setup()
    const { result } = renderHook(() => useCreateSession(GAME_ID), { wrapper })
    let out: unknown
    await act(async () => {
      out = await result.current.mutateAsync(input)
    })
    expect(app.CreateSession).toHaveBeenCalledWith(GAME_ID, input)
    expect(out).toEqual({ session: session(), game: game({ totalPlaySeconds: 7200 }) })
  })

  it('SEL-013/depois de criar, editar ou excluir, o detalhe do jogo é preenchido com o game da resposta, sem nova chamada GetGame', async () => {
    const { qc, wrapper } = setup()
    const detail = renderHook(() => useGame(GAME_ID), { wrapper })
    await waitFor(() => expect(detail.result.current.isSuccess).toBe(true))
    expect(app.GetGame).toHaveBeenCalledTimes(1)

    const create = renderHook(() => useCreateSession(GAME_ID), { wrapper })
    await act(async () => {
      await create.result.current.mutateAsync(input)
    })
    expect((qc.getQueryData(libraryKeys.game(GAME_ID)) as GameDetailView).totalPlaySeconds).toBe(7200)

    const update = renderHook(() => useUpdateSession(GAME_ID), { wrapper })
    await act(async () => {
      await update.result.current.mutateAsync({ sessionId: 5, patch: { activeSeconds: 1800 } })
    })
    expect(app.UpdateSession).toHaveBeenCalledWith(5, { activeSeconds: 1800 })
    expect((qc.getQueryData(libraryKeys.game(GAME_ID)) as GameDetailView).totalPlaySeconds).toBe(5400)

    const del = renderHook(() => useDeleteSession(GAME_ID), { wrapper })
    await act(async () => {
      await del.result.current.mutateAsync(5)
    })
    expect(app.DeleteSession).toHaveBeenCalledWith(5)
    expect((qc.getQueryData(libraryKeys.game(GAME_ID)) as GameDetailView).totalPlaySeconds).toBe(0)

    expect(app.GetGame).toHaveBeenCalledTimes(1)
  })

  it('SEL-013/depois de uma escrita, a lista de sessões do jogo, as listas de jogos, os recentes e as estatísticas são recarregados uma vez', async () => {
    const { qc, wrapper } = setup()
    const spy = vi.spyOn(qc, 'invalidateQueries')
    const sessions = renderHook(() => useGameSessions(GAME_ID), { wrapper })
    await waitFor(() => expect(sessions.result.current.isSuccess).toBe(true))
    expect(app.GameSessions).toHaveBeenCalledTimes(1)

    const create = renderHook(() => useCreateSession(GAME_ID), { wrapper })
    await act(async () => {
      await create.result.current.mutateAsync(input)
    })

    const keys = spy.mock.calls.map((c) => JSON.stringify((c[0] as { queryKey: unknown }).queryKey))
    for (const k of [libraryKeys.sessions(GAME_ID), libraryGamesKey, statsKey]) {
      expect(keys.filter((x) => x === JSON.stringify(k))).toHaveLength(1)
    }
    // libraryGamesKey also covers libraryKeys.recent (it is a child of that prefix).
    await waitFor(() => expect(app.GameSessions).toHaveBeenCalledTimes(2))
  })

  it('SEL-013/falha do agente não invalida nada e propaga o Error com a mensagem do agente', async () => {
    // Wails rejects with a plain string.
    app.UpdateSession.mockRejectedValueOnce('O horário cruza outra sessão de «Celeste», de 10:00 a 11:00.')
    const { qc, wrapper } = setup()
    const before = game()
    qc.setQueryData(libraryKeys.game(GAME_ID), before)
    const spy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useUpdateSession(GAME_ID), { wrapper })

    let caught: unknown
    await act(async () => {
      try {
        await result.current.mutateAsync({ sessionId: 5, patch: { endedAt: input.endedAt } })
      } catch (e) {
        caught = e
      }
    })
    expect(caught).toBeInstanceOf(Error)
    expect((caught as Error).message).toBe('O horário cruza outra sessão de «Celeste», de 10:00 a 11:00.')
    expect(spy).not.toHaveBeenCalled()
    expect(qc.getQueryData(libraryKeys.game(GAME_ID))).toBe(before)
  })

  it('SEL-013/o erro "Esta sessão não existe mais." é reconhecido por isSessionNotFound', () => {
    expect(sessionNotFoundMessage).toBe('Esta sessão não existe mais.')
    expect(isSessionNotFound(new Error('Esta sessão não existe mais.'))).toBe(true)
    expect(isSessionNotFound(new Error('Outro erro.'))).toBe(false)
    expect(isSessionNotFound('Esta sessão não existe mais.')).toBe(false)
    expect(isSessionNotFound(null)).toBe(false)
  })

  it('SEL-013/sem polling: nenhuma chamada além das invalidações', async () => {
    const { wrapper } = setup()
    const detail = renderHook(() => useGame(GAME_ID), { wrapper })
    const sessions = renderHook(() => useGameSessions(GAME_ID), { wrapper })
    await waitFor(() => expect(detail.result.current.isSuccess && sessions.result.current.isSuccess).toBe(true))
    const del = renderHook(() => useDeleteSession(GAME_ID), { wrapper })
    await act(async () => {
      await del.result.current.mutateAsync(5)
    })
    await waitFor(() => expect(app.GameSessions).toHaveBeenCalledTimes(2))
    // Let any stray refetch show up: a macrotask turn is enough for the promise-based fakes.
    await act(async () => {
      await new Promise<void>((resolve) => setTimeout(resolve, 0))
    })
    expect(app.GameSessions).toHaveBeenCalledTimes(2)
    expect(app.GetGame).toHaveBeenCalledTimes(1)
    expect(app.DeleteSession).toHaveBeenCalledTimes(1)
  })
})
