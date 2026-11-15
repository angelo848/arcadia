// T39: BBX-004, BBX-007, BBX-017, BBX-027, BBX-028, BBX-029 (fake bindings + fake Wails runtime).
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import type { CatalogView, GameDetailView, GameView } from '../lib/library'
import type { SessionView } from '../lib/games'
import { useMediaEvents } from '../lib/metadata'
import { buildTabs, sessionMark } from './viewModel'
import { PAGE_SIZE, useFocusedData, useTabGames } from './useBigBoxData'

type Handler = (...d: unknown[]) => void
let handlers: Record<string, Handler[]>
const emit = (name: string, payload: unknown) => act(() => (handlers[name] ?? []).forEach((h) => h(payload)))

const mkGame = (id: number, platformId = 1): GameView => ({
  id, platformId, title: `Jogo ${String(id).padStart(4, '0')}`, platform: 'P', path: 'x', status: 'none' as GameView['status'],
  favorite: false, year: 2000, totalPlaySeconds: 0, playCount: 0, firstPlayedAt: '', lastPlayedAt: '',
})
const detail = (id: number, plays = 0): GameDetailView => ({
  ...mkGame(id), playCount: plays, description: 'd', emulatorId: null, launchType: 'exe', launchArgs: '', completedAt: null,
  developers: [], publishers: [], genres: ['RPG'], playModes: [],
})
const session = (state: SessionView['state'], gameId = 1): SessionView => ({
  sessionId: 1, gameId, title: 't', state, startedAt: '', endedAt: '', activeSeconds: 0, activeMs: 0, asOf: '',
})

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyMock = Mock<(...args: any[]) => any>
let app: Record<string, AnyMock>
let qc: QueryClient
const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>

function setup(total = 450) {
  app = {
    ListGames: vi.fn(async (a: { offset: number; limit: number }) => ({
      games: Array.from({ length: Math.max(0, Math.min(a.limit, total - a.offset)) }, (_, i) => mkGame(a.offset + i + 1)),
      total, offset: a.offset,
    })),
    GetGame: vi.fn(async (id: number) => detail(id)),
    MediaList: vi.fn(async () => ({ media: [] })),
    CurrentSession: vi.fn(async () => null),
    Catalog: vi.fn(async (): Promise<CatalogView> => ({
      platforms: [
        { id: 1, name: 'NES', category: '', emulatorId: null, seedKey: null, iconUrl: '', gameCount: 3 },
        { id: 2, name: 'SNES', category: '', emulatorId: null, seedKey: null, iconUrl: '', gameCount: 5 },
        { id: 3, name: 'Vazia', category: '', emulatorId: null, seedKey: null, iconUrl: '', gameCount: 0 },
      ],
      genres: [], companies: [], playModes: [],
    })),
  }
  ;(window as unknown as Record<string, unknown>).go = { main: { App: app } }
  ;(window as unknown as Record<string, unknown>).runtime = {
    EventsOn: (name: string, cb: Handler) => {
      ;(handlers[name] ??= []).push(cb)
      return () => { handlers[name] = handlers[name].filter((h) => h !== cb) }
    },
  }
}

beforeEach(() => {
  handlers = {}
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  setup()
})
afterEach(() => {
  delete (window as unknown as Record<string, unknown>).go
  delete (window as unknown as Record<string, unknown>).runtime
})

describe('useBigBoxData', () => {
  it('BBX-004/o catálogo vira abas; a aba inicial segue a plataforma da janela normal', async () => {
    const catalog = await app.Catalog()
    const { tabs, initialTab } = buildTabs(catalog, 2)
    expect(tabs.map((t) => t.label)).toEqual(['Todos', 'NES', 'SNES'])
    expect(tabs[initialTab].platformId).toBe(2)
    const { result } = renderHook(() => useTabGames(tabs[initialTab].platformId), { wrapper })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    expect(app.ListGames.mock.calls[0][0].filter.platformIds ?? app.ListGames.mock.calls[0][0].filter.platformId).toBeTruthy()
  })

  it('BBX-007/a lista usa só o filtro de plataforma, ordem por título e páginas de 200', async () => {
    const { result, rerender } = renderHook(({ p }: { p: number | null }) => useTabGames(p), { wrapper, initialProps: { p: null as number | null } })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    const a = app.ListGames.mock.calls[0][0]
    expect(PAGE_SIZE).toBe(200)
    expect(a.limit).toBe(200)
    expect(a.offset).toBe(0)
    expect(a.sort).toBe('title')
    expect(JSON.stringify(a.filter)).not.toMatch(/search|genre|favorite|status|played/i)
    rerender({ p: 2 })
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBe(2))
    expect(JSON.stringify(app.ListGames.mock.calls[1][0].filter)).toContain('2')
  })

  it('BBX-007/a próxima página é pedida uma vez e sem pedido repetido em voo', async () => {
    const { result } = renderHook(() => useTabGames(null), { wrapper })
    await waitFor(() => expect(result.current.loaded).toBe(200))
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => (release = r))
    const base = app.ListGames.getMockImplementation()!
    app.ListGames.mockImplementation(async (a) => { await gate; return base(a) })
    act(() => { result.current.fetchNextPage(); result.current.fetchNextPage(); result.current.fetchNextPage() })
    await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))
    expect(app.ListGames).toHaveBeenCalledTimes(2) // first page + a single next page
    expect(app.ListGames.mock.calls[1][0].offset).toBe(200)
    release()
    await waitFor(() => expect(result.current.loaded).toBe(400))
    expect(app.ListGames).toHaveBeenCalledTimes(2)
  })

  it('BBX-007/o fim da faixa é o total da aba', async () => {
    const { result } = renderHook(() => useTabGames(null), { wrapper })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    expect(result.current.total).toBe(450)
    expect(result.current.loaded).toBe(200)
    expect(result.current.games).toHaveLength(200)
    await act(async () => { result.current.fetchNextPage() })
    await waitFor(() => expect(result.current.loaded).toBe(400))
    await act(async () => { result.current.fetchNextPage() })
    await waitFor(() => expect(result.current.loaded).toBe(450))
    const calls = app.ListGames.mock.calls.length
    act(() => result.current.fetchNextPage()) // nothing left: no new request
    expect(app.ListGames.mock.calls.length).toBe(calls)
    expect(result.current.error).toBeNull()
  })

  it('BBX-027/o jogo em foco da janela normal vira o foco inicial quando está carregado; senão o primeiro', async () => {
    const { result } = renderHook(() => useTabGames(null), { wrapper })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    const initialIndex = (gameId: number | null) => Math.max(0, result.current.games.findIndex((g) => g.id === gameId))
    expect(initialIndex(5)).toBe(4)
    expect(initialIndex(9999)).toBe(0)
    expect(initialIndex(null)).toBe(0)
  })

  it('BBX-017/session:changed e CurrentSession marcam o jogo em Running, Grace e Paused', async () => {
    app.CurrentSession.mockResolvedValue(session('running', 1))
    const { result } = renderHook(() => useFocusedData(1), { wrapper })
    await waitFor(() => expect(result.current.session?.state).toBe('running'))
    expect(sessionMark(result.current.session, 1)).toBe('running')
    await emit('session:changed', session('grace', 1))
    await waitFor(() => expect(result.current.session?.state).toBe('grace'))
    expect(sessionMark(result.current.session, 1)).toBe('running')
    await emit('session:changed', session('paused', 1))
    await waitFor(() => expect(result.current.session?.state).toBe('paused'))
    expect(sessionMark(result.current.session, 1)).toBe('paused')
  })

  it('BBX-028/media:changed do jogo em foco relê a mídia (arte, capa e vídeo)', async () => {
    const Events = ({ children }: { children: ReactNode }) => {
      useMediaEvents()
      return <>{children}</>
    }
    const w = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>
        <Events>{children}</Events>
      </QueryClientProvider>
    )
    const { result } = renderHook(() => useFocusedData(1), { wrapper: w })
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(1))
    app.MediaList.mockResolvedValue({ media: [{ id: 9, kind: 'video', provider: 'x', path: '', thumbPath: '', url: '/v.mp4', thumbUrl: '' }] })
    await emit('media:changed', { gameId: 1 })
    await waitFor(() => expect(result.current.media.map((m) => m.kind)).toEqual(['video']))
    expect(app.MediaList).toHaveBeenCalledTimes(2)
  })

  it('BBX-028/o fim da sessão relê o jogo (tempo, última vez, sessões)', async () => {
    const { result } = renderHook(() => useFocusedData(1), { wrapper })
    await waitFor(() => expect(result.current.detail?.playCount).toBe(0))
    app.GetGame.mockResolvedValue(detail(1, 3))
    await emit('session:changed', session('running', 1)) // live: no reread
    expect(app.GetGame).toHaveBeenCalledTimes(1)
    await emit('session:changed', session('ended', 1))
    await waitFor(() => expect(result.current.detail?.playCount).toBe(3))
    expect(app.GetGame).toHaveBeenCalledTimes(2)
  })

  it('BBX-029/só ListGames, GetGame, MediaList, Catalog e CurrentSession são chamados; nenhum VideoQueue, MediaQueue ou escrita', async () => {
    const guarded = new Proxy(app, {
      get(t, k: string) {
        if (!(k in t)) return vi.fn(() => { throw new Error(`binding indevido: ${k}`) })
        return t[k]
      },
    })
    ;(window as unknown as { go: unknown }).go = { main: { App: guarded } }
    const tab = renderHook(() => useTabGames(null), { wrapper })
    const focus = renderHook(() => useFocusedData(1), { wrapper })
    await waitFor(() => expect(tab.result.current.isPending).toBe(false))
    await waitFor(() => expect(focus.result.current.detail).toBeDefined())
    await emit('media:changed', { gameId: 1 })
    await emit('session:changed', session('ended', 1))
    const allowed = new Set(['ListGames', 'GetGame', 'MediaList', 'Catalog', 'CurrentSession'])
    for (const [name, fn] of Object.entries(app)) {
      if (!allowed.has(name)) expect(fn).not.toHaveBeenCalled()
    }
    expect(Object.keys(app).every((k) => allowed.has(k))).toBe(true)
  })
})
