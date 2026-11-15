import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { mediaKey, useCardImages, useMedia, useMediaEvents } from './metadata'

type Fn = ReturnType<typeof vi.fn>

let app: Record<string, Fn>
let handlers: Map<string, Set<(...data: unknown[]) => void>>

function install() {
  handlers = new Map()
  app = {
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
    ListGames: vi.fn(() => Promise.resolve({ games: [], total: 0, offset: 0 })),
    GetGame: vi.fn(() => Promise.resolve({})),
    GameSessions: vi.fn(() => Promise.resolve({ sessions: [], total: 0, offset: 0 })),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    LaunchGame: vi.fn(() => Promise.resolve({})),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    PauseSession: vi.fn(() => Promise.resolve({})),
    ResumeSession: vi.fn(() => Promise.resolve({})),
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

async function settle() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 20))
  })
}

function otherBindingCalls() {
  return Object.entries(app)
    .filter(([name]) => name !== 'MediaList')
    .reduce((n, [, fn]) => n + fn.mock.calls.length, 0)
}

beforeEach(() => {
  install()
})

afterEach(() => {
  uninstall()
})

describe('useMediaEvents', () => {
  it('IMG-006/lista de G em uso na tela é relida na hora, ignorando o staleTime de 60 s', async () => {
    const qc = newClient()
    // useCardImages has staleTime 60 s: only an explicit invalidation can reread it.
    renderHook(
      () => {
        useMediaEvents()
        return useCardImages([10])
      },
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(1))

    emit('media:changed', { gameId: 10 })

    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(2))
    expect(app.MediaList).toHaveBeenLastCalledWith(10)
    await settle()
    expect(app.MediaList).toHaveBeenCalledTimes(2)
  })

  it('IMG-006/useMedia em uso também é relida com MediaList(G)', async () => {
    const qc = newClient()
    renderHook(
      () => {
        useMediaEvents()
        return useMedia(10)
      },
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(1))

    emit('media:changed', { gameId: 10 })

    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(2))
    expect(app.MediaList).toHaveBeenLastCalledWith(10)
  })

  it('IMG-006/lista de G em cache fora da tela fica velha, sem leitura', async () => {
    const qc = newClient()
    qc.setQueryData(mediaKey(10), [])
    renderHook(() => useMediaEvents(), { wrapper: wrapperFor(qc) })
    expect(qc.getQueryState(mediaKey(10))?.isInvalidated).toBe(false)

    emit('media:changed', { gameId: 10 })
    await settle()

    expect(qc.getQueryState(mediaKey(10))?.isInvalidated).toBe(true)
    expect(app.MediaList).not.toHaveBeenCalled()
  })

  it('IMG-006/G sem lista em cache: nada é lido nem criado', async () => {
    const qc = newClient()
    renderHook(() => useMediaEvents(), { wrapper: wrapperFor(qc) })

    emit('media:changed', { gameId: 10 })
    await settle()

    expect(app.MediaList).not.toHaveBeenCalled()
    expect(qc.getQueryState(mediaKey(10))).toBeUndefined()
  })

  it('IMG-006/o aviso de G não toca a lista de mídia de outro jogo H', async () => {
    const qc = newClient()
    qc.setQueryData(mediaKey(10), [])
    qc.setQueryData(mediaKey(20), [])
    renderHook(() => useMediaEvents(), { wrapper: wrapperFor(qc) })

    emit('media:changed', { gameId: 10 })

    expect(qc.getQueryState(mediaKey(10))?.isInvalidated).toBe(true)
    expect(qc.getQueryState(mediaKey(20))?.isInvalidated).toBe(false)
  })

  it('IMG-006/nenhum outro binding é chamado e nenhuma outra consulta é invalidada', async () => {
    const qc = newClient()
    qc.setQueryData(['library', 'games'], { pages: [], pageParams: [] })
    qc.setQueryData(['library', 'game', 10], {})
    qc.setQueryData(['library', 'sessions', 10], {})
    qc.setQueryData(['session', 'current'], null)
    renderHook(
      () => {
        useMediaEvents()
        return useMedia(10)
      },
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(1))

    emit('media:changed', { gameId: 10 })
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(2))
    await settle()

    expect(otherBindingCalls()).toBe(0)
    for (const key of [['library', 'games'], ['library', 'game', 10], ['library', 'sessions', 10], ['session', 'current']]) {
      expect(qc.getQueryState(key)?.isInvalidated, JSON.stringify(key)).toBe(false)
    }
  })

  const invalid: Array<[string, unknown[]]> = [
    ['sem argumentos', []],
    ['undefined', [undefined]],
    ['null', [null]],
    ['objeto vazio', [{}]],
    ['gameId 0', [{ gameId: 0 }]],
    ['gameId negativo', [{ gameId: -3 }]],
    ['gameId fracionário', [{ gameId: 1.5 }]],
    ['gameId string', [{ gameId: '10' }]],
    ['gameId NaN', [{ gameId: Number.NaN }]],
    ['gameId nulo', [{ gameId: null }]],
    ['argumento não é objeto', [10]],
  ]
  for (const [name, args] of invalid) {
    it(`IMG-006/dado inválido é ignorado (${name})`, async () => {
      const qc = newClient()
      qc.setQueryData(mediaKey(10), [])
      renderHook(() => useMediaEvents(), { wrapper: wrapperFor(qc) })

      expect(() => emit('media:changed', ...args)).not.toThrow()
      await settle()

      expect(qc.getQueryState(mediaKey(10))?.isInvalidated).toBe(false)
      expect(app.MediaList).not.toHaveBeenCalled()
    })
  }

  it('IMG-007/uma única assinatura de media:changed enquanto montado; desmontar a cancela', () => {
    const qc = newClient()
    const { rerender, unmount } = renderHook(() => useMediaEvents(), { wrapper: wrapperFor(qc) })
    expect(handlers.get('media:changed')?.size ?? 0).toBe(1)

    rerender()
    expect(handlers.get('media:changed')?.size ?? 0).toBe(1)

    unmount()
    expect(handlers.get('media:changed')?.size ?? 0).toBe(0)
  })

  it('IMG-007/depois de desmontar, o aviso não invalida nada', () => {
    const qc = newClient()
    qc.setQueryData(mediaKey(10), [])
    const { unmount } = renderHook(() => useMediaEvents(), { wrapper: wrapperFor(qc) })
    unmount()

    emit('media:changed', { gameId: 10 })

    expect(qc.getQueryState(mediaKey(10))?.isInvalidated).toBe(false)
  })

  it('IMG-012/o aviso só chama MediaList: nada de lançar, pausar, retomar nem ler sessão e jogos', async () => {
    const qc = newClient()
    renderHook(
      () => {
        useMediaEvents()
        return useMedia(10)
      },
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(1))

    emit('media:changed', { gameId: 10 })
    emit('media:changed', { gameId: 99 })
    emit('media:changed', { gameId: 'x' })
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(2))
    await settle()

    for (const name of ['LaunchGame', 'LaunchGameUntracked', 'PauseSession', 'ResumeSession', 'CurrentSession', 'ListGames', 'GetGame', 'GameSessions']) {
      expect(app[name], name).not.toHaveBeenCalled()
    }
    expect(qc.getQueryData(['session', 'current'])).toBeUndefined()
  })
})
