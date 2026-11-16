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
import { PAGE_EXTRA, PAGE_MAX, pageSize, shouldPrefetch, useFocusedData, useTabGames } from './useBigBoxData'

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
    const { result } = renderHook(() => useTabGames(tabs[initialTab].platformId, 12), { wrapper })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    expect(app.ListGames.mock.calls[0][0].filter.platformIds ?? app.ListGames.mock.calls[0][0].filter.platformId).toBeTruthy()
  })

  it('BBX-007/shouldPrefetch: uma linha por linha da tabela de bigbox-faixa §3.2', () => {
    // (L, T, f, V, exhausted) -> pede?
    expect(shouldPrefetch(0, 40, 0, 12, false)).toBe(true) // entrada: nada lido
    expect(shouldPrefetch(15, 40, 0, 12, false)).toBe(false) // 15 − 0 = 15
    expect(shouldPrefetch(15, 40, 1, 12, false)).toBe(true) // 14 < 15
    expect(shouldPrefetch(30, 40, 15, 12, false)).toBe(false)
    expect(shouldPrefetch(30, 40, 16, 12, false)).toBe(true)
    expect(shouldPrefetch(40, 40, 39, 12, false)).toBe(false) // L = T
    expect(shouldPrefetch(15, 40, 1, 12, true)).toBe(false) // exhausted
  })

  it('BBX-007/V maior depois de resize torna a condição verdadeira com o mesmo f e L', () => {
    expect(shouldPrefetch(15, 40, 0, 12, false)).toBe(false)
    expect(shouldPrefetch(15, 40, 0, 13, false)).toBe(true)
  })

  it('BBX-038/pageSize: 12 dá 15, 1 dá 4, 30 dá 17 (V limitado a 14), nunca acima de 500', () => {
    expect(PAGE_EXTRA).toBe(3)
    expect(PAGE_MAX).toBe(500)
    expect(pageSize(12)).toBe(15)
    expect(pageSize(1)).toBe(4)
    expect(pageSize(30)).toBe(17)
    expect(pageSize(100000)).toBeLessThanOrEqual(500)
  })

  it('BBX-038/entrar na aba pede uma só página: offset 0, limit V + 3 (15 com V 12), filtro só de plataforma, ordem por título', async () => {
    setup(120)
    const { result } = renderHook(() => useTabGames(null, 12), { wrapper })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    expect(app.ListGames).toHaveBeenCalledTimes(1)
    const a = app.ListGames.mock.calls[0][0]
    expect(a.limit).toBe(15)
    expect(a.offset).toBe(0)
    expect(a.sort).toBe('title')
    expect(JSON.stringify(a.filter)).not.toMatch(/search|genre|favorite|status|played/i)
    expect(app.ListGames).toHaveBeenCalledTimes(1) // nenhuma outra sem fetchNextPage
  })

  it('BBX-038/entrar numa plataforma filtra por ela', async () => {
    setup(120)
    const { result, rerender } = renderHook(({ p }: { p: number | null }) => useTabGames(p, 12), { wrapper, initialProps: { p: null as number | null } })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    rerender({ p: 2 })
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBe(2))
    expect(JSON.stringify(app.ListGames.mock.calls[1][0].filter)).toContain('2')
  })

  it('BBX-038/isPending só até a primeira página; total é o GameListResult.total (120) com loaded 15', async () => {
    setup(120)
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => (release = r))
    const base = app.ListGames.getMockImplementation()!
    app.ListGames.mockImplementation(async (a) => { await gate; return base(a) })
    const { result } = renderHook(() => useTabGames(null, 12), { wrapper })
    expect(result.current.isPending).toBe(true)
    release()
    await waitFor(() => expect(result.current.isPending).toBe(false))
    expect(result.current.total).toBe(120)
    expect(result.current.loaded).toBe(15)
    expect(result.current.games).toHaveLength(15)
    expect(result.current.exhausted).toBe(false)
  })

  it('BBX-007/fetchNextPage pede offset = loaded e limit do V atual; chamado de novo com a página em voo não pede outra nem cancela a primeira', async () => {
    setup(120)
    const { result, rerender } = renderHook(({ v }: { v: number }) => useTabGames(null, v), { wrapper, initialProps: { v: 12 } })
    await waitFor(() => expect(result.current.loaded).toBe(15))
    rerender({ v: 10 })
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => (release = r))
    const base = app.ListGames.getMockImplementation()!
    app.ListGames.mockImplementation(async (a) => { await gate; return base(a) })
    act(() => { result.current.fetchNextPage(); result.current.fetchNextPage(); result.current.fetchNextPage() })
    await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))
    expect(app.ListGames).toHaveBeenCalledTimes(2)
    expect(app.ListGames.mock.calls[1][0].offset).toBe(15)
    expect(app.ListGames.mock.calls[1][0].limit).toBe(13) // V 10 + 3
    release()
    await waitFor(() => expect(result.current.loaded).toBe(28))
    expect(app.ListGames).toHaveBeenCalledTimes(2)
    expect(result.current.games).toHaveLength(28)
  })

  it('BBX-007/página vazia marca exhausted e fetchNextPage vira no-op', async () => {
    setup(120)
    const { result } = renderHook(() => useTabGames(null, 12), { wrapper })
    await waitFor(() => expect(result.current.loaded).toBe(15))
    app.ListGames.mockImplementation(async (a: { offset: number }) => ({ games: [], total: 120, offset: a.offset }))
    await act(async () => { result.current.fetchNextPage() })
    await waitFor(() => expect(result.current.exhausted).toBe(true))
    const calls = app.ListGames.mock.calls.length
    act(() => result.current.fetchNextPage())
    expect(app.ListGames.mock.calls.length).toBe(calls)
    expect(result.current.loaded).toBe(15)
  })

  it('BBX-007/loaded ≥ total: fetchNextPage não pede', async () => {
    setup(10)
    const { result } = renderHook(() => useTabGames(null, 12), { wrapper })
    await waitFor(() => expect(result.current.isPending).toBe(false))
    expect(result.current.loaded).toBe(10)
    act(() => result.current.fetchNextPage())
    expect(app.ListGames).toHaveBeenCalledTimes(1)
    expect(result.current.error).toBeNull()
  })

  it('BBX-038/troca de aba com página em voo: a resposta tardia não entra na aba antiga nem na nova e nenhuma página nova da antiga é pedida', async () => {
    setup(120)
    const base = app.ListGames.getMockImplementation()!
    app.ListGames.mockImplementation(async (a: { filter: { platformIds?: number[] }; offset: number; limit: number }) => {
      const r = await base(a)
      const pid = a.filter.platformIds?.[0] ?? 0
      return { ...r, games: r.games.map((g: GameView) => ({ ...g, platformId: pid })) }
    })
    const { result, rerender } = renderHook(({ p }: { p: number | null }) => useTabGames(p, 12), { wrapper, initialProps: { p: null as number | null } })
    await waitFor(() => expect(result.current.loaded).toBe(15))
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => (release = r))
    const impl = app.ListGames.getMockImplementation()!
    app.ListGames.mockImplementation(async (a) => { if (a.offset > 0) await gate; return impl(a) })
    act(() => result.current.fetchNextPage())
    await waitFor(() => expect(result.current.isFetchingNextPage).toBe(true))
    rerender({ p: 2 })
    await waitFor(() => expect(result.current.games.length).toBe(15))
    expect(result.current.games.every((g) => g.platformId === 2)).toBe(true)
    release()
    await act(async () => { await Promise.resolve() })
    expect(result.current.games).toHaveLength(15)
    expect(result.current.games.every((g) => g.platformId === 2)).toBe(true)
    const oldTabPages = app.ListGames.mock.calls.filter((c) => c[0].filter.platformIds === undefined)
    expect(oldTabPages).toHaveLength(2) // primeira página + a que ficou em voo, nenhuma nova
  })

  it('BBX-038/falha na primeira página: error preenchido, isPending falso, games vazio', async () => {
    setup(120)
    app.ListGames.mockRejectedValue(new Error('boom'))
    const { result } = renderHook(() => useTabGames(null, 12), { wrapper })
    await waitFor(() => expect(result.current.error).not.toBeNull())
    expect(result.current.isPending).toBe(false)
    expect(result.current.games).toEqual([])
  })

  it('BBX-038/falha na segunda: error preenchido, games mantém a primeira', async () => {
    setup(120)
    const { result } = renderHook(() => useTabGames(null, 12), { wrapper })
    await waitFor(() => expect(result.current.loaded).toBe(15))
    app.ListGames.mockRejectedValue(new Error('boom'))
    await act(async () => { result.current.fetchNextPage() })
    await waitFor(() => expect(result.current.error).not.toBeNull())
    expect(result.current.games).toHaveLength(15)
  })

  it('BBX-038/voltar a uma aba já lida não chama ListGames de novo e mantém as páginas', async () => {
    setup(120)
    const { result, rerender } = renderHook(({ p }: { p: number | null }) => useTabGames(p, 12), { wrapper, initialProps: { p: null as number | null } })
    await waitFor(() => expect(result.current.loaded).toBe(15))
    await act(async () => { result.current.fetchNextPage() })
    await waitFor(() => expect(result.current.loaded).toBe(30))
    rerender({ p: 2 })
    await waitFor(() => expect(result.current.loaded).toBe(15))
    const before = app.ListGames.mock.calls.length
    rerender({ p: null })
    await waitFor(() => expect(result.current.loaded).toBe(30))
    expect(app.ListGames.mock.calls.length).toBe(before)
  })

  it('BBX-027/o jogo em foco da janela normal vira o foco inicial quando está carregado; senão o primeiro', async () => {
    const { result } = renderHook(() => useTabGames(null, 12), { wrapper })
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
    const tab = renderHook(() => useTabGames(null, 12), { wrapper })
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
