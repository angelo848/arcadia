import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BigBox } from './BigBox'
import { desktopGrid } from './desktopLayout'
import { useMediaEvents } from '../lib/metadata'
import { useTimeToBeatEvents } from '../lib/timeToBeat'
import type { LibraryView } from './viewModel'

// Phase 17 (T13: TDK-011, TDK-012, TDK-016..TDK-020, TDK-024, TDK-025, TDK-031, TDK-033): the shell with the real Desktop theme.
// Assumed markup (Desktop, T15): <nav aria-label="Plataformas"> with one button per tab named "<label>, <count> jogos" (data-focused and
// aria-current on the active one); <div role="listbox" aria-label="Jogos"> with role="option" (aria-label = title, aria-selected and
// data-focused on the focus in the games zone); aside "Detalhes do jogo" with the real DesktopDetails (buttons "Jogar" and
// "Jogar sem contar tempo" with data-focused in the panel zone). Keyboard: Enter = A, d = X, Escape = B, m = Y, PageUp/PageDown = LB/RB.
// The footer is the shell's <ul> of hints (each li: a key span then a label span).

type Fn = ReturnType<typeof vi.fn>
let handlers: Map<string, Set<(...d: unknown[]) => void>>
let app: Record<string, Fn>
let called: string[]
let runtime: Record<string, Fn>

const PAD = { A: 0, B: 1, X: 2, Y: 3, LB: 4, RB: 5, SELECT: 8, UP: 12, DOWN: 13, LEFT: 14, RIGHT: 15 }
const CATALOG = {
  platforms: [
    { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 60 },
    { id: 2, name: 'PlayStation 2', category: 'console', emulatorId: null, seedKey: 'ps2', iconUrl: '', gameCount: 2 },
  ],
  genres: [], companies: [], playModes: [],
}
const title = (n: number) => `Jogo ${String(n).padStart(3, '0')}`
const g = (id: number, platformId: number) => ({
  id, platformId, title: title(id), platform: platformId === 1 ? 'Windows' : 'PlayStation 2', path: `C:\\g\\${id}.exe`, status: 'not_played',
  favorite: false, year: null, totalPlaySeconds: 0, playCount: 0, firstPlayedAt: '', lastPlayedAt: '',
})
const detailOf = (row: ReturnType<typeof g>) => ({
  ...row, description: 'Descrição.', completedAt: null, developers: [], publishers: [], genres: [], playModes: [], launchType: 'exe',
  launchArgs: '', sessionPlaySeconds: 0, emulatorId: null,
})
type Library = { rows: ReturnType<typeof g>[]; ps2: ReturnType<typeof g>[]; gates: Map<number, Promise<void>>; inflight: { now: number; max: number } }
let lib: Library

function session(over: Record<string, unknown> = {}) {
  return {
    sessionId: 1, gameId: 1, title: title(1), state: 'running', startedAt: '2026-11-15T10:00:00.000Z', endedAt: '', activeSeconds: 0,
    activeMs: 0, asOf: '2026-11-15T10:00:00.000Z', ...over,
  }
}

function install(n = 60) {
  handlers = new Map()
  called = []
  lib = {
    rows: Array.from({ length: n }, (_, i) => g(i + 1, 1)), ps2: [g(1001, 2), g(1002, 2)], gates: new Map(), inflight: { now: 0, max: 0 },
  }
  app = {
    ListGames: vi.fn(async ({ filter, offset, limit }: { filter: { platformIds?: number[] }; offset: number; limit: number }) => {
      lib.inflight.now += 1
      lib.inflight.max = Math.max(lib.inflight.max, lib.inflight.now)
      try {
        await lib.gates.get(offset)
        const all = filter.platformIds?.[0] === 2 ? lib.ps2 : filter.platformIds?.[0] === 1 ? lib.rows : [...lib.rows, ...lib.ps2]
        return { games: all.slice(offset, offset + limit), total: all.length, offset }
      } finally {
        lib.inflight.now -= 1
      }
    }),
    GetGame: vi.fn((id: number) => Promise.resolve(detailOf([...lib.rows, ...lib.ps2].find((x) => x.id === id)!))),
    Catalog: vi.fn(() => Promise.resolve({ ...CATALOG, platforms: [{ ...CATALOG.platforms[0], gameCount: n }, CATALOG.platforms[1]] })),
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    LaunchGame: vi.fn((id: number) => Promise.resolve(session({ gameId: id }))),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    BringToFront: vi.fn(() => Promise.resolve()),
    TimeToBeatGet: vi.fn(() => Promise.resolve(null)),
  }
  const proxy = new Proxy(app, {
    get(target, name: string) {
      if (typeof name !== 'string') return undefined
      return (...args: unknown[]) => {
        called.push(name)
        const fn = target[name] as ((...a: unknown[]) => unknown) | undefined
        return fn ? fn(...args) : Promise.resolve(undefined)
      }
    },
  })
  runtime = {
    EventsOn: vi.fn((name: string, cb: (...d: unknown[]) => void) => {
      let set = handlers.get(name)
      if (!set) handlers.set(name, (set = new Set()))
      set.add(cb)
      return () => { set!.delete(cb) }
    }),
    WindowIsFullscreen: vi.fn(() => Promise.resolve(false)),
    WindowFullscreen: vi.fn(),
    WindowUnfullscreen: vi.fn(),
  }
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: proxy } }
  w.runtime = runtime
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  setViewport(1920, 1080)
}
function setViewport(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: height })
}

// ---- fake gamepad (standard mapping, edges read by a controlled requestAnimationFrame) ----
let padPressed: Set<number>
let rafQueue: Map<number, FrameRequestCallback>
let rafSpies: Array<{ mockRestore(): void }> = []
function installPad() {
  padPressed = new Set()
  rafQueue = new Map()
  let id = 0
  Object.defineProperty(navigator, 'getGamepads', {
    configurable: true,
    value: () => [{
      connected: true, mapping: 'standard', axes: [0, 0, 0, 0],
      buttons: Array.from({ length: 17 }, (_, i) => ({ pressed: padPressed.has(i), touched: padPressed.has(i), value: padPressed.has(i) ? 1 : 0 })),
    }],
  })
  rafSpies = [
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => { id += 1; rafQueue.set(id, cb); return id }),
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((n: number) => { rafQueue.delete(n) }),
  ]
}
function uninstallPad() {
  rafSpies.forEach((s) => s.mockRestore())
  rafSpies = []
  Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: undefined })
}
function frame() {
  const q = [...rafQueue.values()]
  rafQueue.clear()
  act(() => { for (const cb of q) cb(performance.now()) })
}
function tap(button: number) {
  padPressed.add(button)
  frame()
  padPressed.delete(button)
  frame()
}
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 20)) })
const wait = (ms: number) => act(async () => { await new Promise((r) => setTimeout(r, ms)) })
const emit = (name: string, ...data: unknown[]) => act(() => { for (const cb of [...(handlers.get(name) ?? [])]) cb(...data) })
const key = (k: string) => fireEvent.keyDown(window, { key: k })

function setup(p: { initialPlatformId?: number | null; initialGameId?: number | null; libraryView?: LibraryView } = {}) {
  const onExit = vi.fn()
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const Events = ({ children }: { children: ReactNode }) => {
    useMediaEvents()
    useTimeToBeatEvents()
    return <>{children}</>
  }
  const r = render(
    <BigBox
      initialPlatformId={p.initialPlatformId ?? null}
      initialGameId={p.initialGameId ?? null}
      themeId="desktop"
      librarySort="title"
      libraryView={p.libraryView ?? 'grid'}
      onExit={onExit}
    />,
    { wrapper: ({ children }) => <QueryClientProvider client={qc}><Events>{children}</Events></QueryClientProvider> },
  )
  return { onExit, ...r }
}

const options = () => screen.queryAllByRole('option')
const optionLabels = () => options().map((o) => o.getAttribute('aria-label'))
const focusedOption = () => options().find((o) => o.getAttribute('aria-selected') === 'true')
const nav = () => screen.getByRole('navigation', { name: 'Plataformas' })
const panel = () => screen.getByRole('complementary', { name: 'Detalhes do jogo' })
const marks = () => Array.from(document.querySelectorAll('[data-focused="true"]'))
const zoneMarks = () => ({
  games: options().filter((o) => o.getAttribute('data-focused') === 'true').length,
  platforms: Array.from(nav().querySelectorAll('[data-focused="true"]')).length,
  panel: Array.from(panel().querySelectorAll('[data-focused="true"]')).length,
})
async function ready() {
  await waitFor(() => expect(options().length).toBeGreaterThan(0))
  await waitFor(() => expect(within(panel()).getByRole('heading', { name: /Jogo/ })).toBeInTheDocument())
  frame() // primes the edge detector of the gamepad
}
const hints = () => {
  const ul = screen.getAllByText('LB/RB')[0].closest('ul') as HTMLElement
  return Array.from(ul.querySelectorAll('li')).map((li) => Array.from(li.querySelectorAll('span')).map((s) => s.textContent).join(' '))
}
const GRID = () => desktopGrid({ width: 1920, height: 1080 }, 'grid')
const ttbCalls = () => app.TimeToBeatGet.mock.calls as unknown as Array<[number, boolean]>
const fetchTrue = () => ttbCalls().filter((c) => c[1] === true)
const deferred = <T,>() => {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => { resolve = r })
  return { promise, resolve }
}

beforeEach(() => {
  install()
  installPad()
})
afterEach(() => {
  uninstallPad()
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
  vi.unstubAllGlobals()
})

describe('BigBox + Desktop: zonas e foco', () => {
  it('TDK-012/entra em Jogos com o foco no jogo em foco da janela normal (BBX-027) e o painel mostra o jogo em foco em qualquer zona', async () => {
    setup({ initialPlatformId: 1, initialGameId: 5 })
    await ready()
    expect(focusedOption()).toHaveAttribute('aria-label', title(5))
    expect(zoneMarks()).toEqual({ games: 1, platforms: 0, panel: 0 })
    expect(within(panel()).getByRole('heading', { name: title(5) })).toBeInTheDocument()
    for (let i = 0; i < 12; i++) tap(PAD.LEFT)
    expect(zoneMarks()).toEqual({ games: 0, platforms: 1, panel: 0 })
    expect(within(panel()).getByRole('heading', { name: /Jogo/ })).toBeInTheDocument()
    expect(marks().filter((m) => !panel().contains(m))).toHaveLength(1)
  })

  it('TDK-012/jogo inicial fora da aba cai no primeiro jogo, na zona Jogos', async () => {
    setup({ initialPlatformId: 2, initialGameId: 5 })
    await ready()
    expect(focusedOption()).toHaveAttribute('aria-label', title(1001))
    expect(zoneMarks().games).toBe(1)
  })

  it('TDK-016/X leva o foco ao Painel em Jogar; LB/RB trocam de aba sem mudar a zona', async () => {
    setup()
    await ready()
    tap(PAD.RB)
    await waitFor(() => expect(nav().querySelector('[aria-current="true"]')).toHaveTextContent('Windows'))
    expect(zoneMarks().games).toBe(1)
    for (let i = 0; i < 12; i++) tap(PAD.LEFT)
    expect(zoneMarks().platforms).toBe(1)
    tap(PAD.RB)
    await waitFor(() => expect(nav().querySelector('[aria-current="true"]')).toHaveTextContent('PlayStation 2'))
    expect(zoneMarks().platforms).toBe(1) // the zone did not change
    tap(PAD.X)
    const play = await within(panel()).findByRole('button', { name: 'Jogar' })
    expect(play).toHaveAttribute('data-focused', 'true')
    expect(zoneMarks()).toEqual({ games: 0, platforms: 0, panel: 1 })
  })

  it('TDK-017/A (Enter) chama LaunchGame uma vez; tecla mantida não relança', async () => {
    setup()
    await ready()
    padPressed.add(PAD.A)
    frame()
    frame()
    frame() // held for three frames: a single edge
    padPressed.delete(PAD.A)
    frame()
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    expect(app.LaunchGame).toHaveBeenCalledWith(1)
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
  })

  it('TDK-017/B sai do Big Box; Y liga e desliga o som do vídeo visível', async () => {
    app.MediaList.mockResolvedValue({
      media: [{ id: 2, kind: 'video', provider: 'x', path: 'v', thumbPath: '', url: '/m/v.mp4', thumbUrl: '' }],
    })
    const { onExit } = setup()
    await ready()
    const video = await screen.findByTestId('bb-video', undefined, { timeout: 5000 })
    expect((video as HTMLVideoElement).muted).toBe(false)
    tap(PAD.Y)
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(true))
    tap(PAD.Y)
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(false))
    expect(onExit).not.toHaveBeenCalled()
    tap(PAD.B)
    expect(onExit).toHaveBeenCalledTimes(1)
  }, 15000)

  it('TDK-018/← na primeira coluna vai a Plataformas, → na última coluna vai ao Painel, ← num botão volta a Jogos', async () => {
    setup()
    await ready()
    const { columns } = GRID()
    tap(PAD.LEFT)
    expect(zoneMarks().platforms).toBe(1) // focus 0 = first column
    tap(PAD.RIGHT) // →: back to games
    expect(zoneMarks().games).toBe(1)
    for (let i = 0; i < columns - 1; i++) tap(PAD.RIGHT)
    expect(focusedOption()).toHaveAttribute('aria-label', title(columns))
    tap(PAD.RIGHT) // last column
    expect(await within(panel()).findByRole('button', { name: 'Jogar' })).toHaveAttribute('data-focused', 'true')
    expect(zoneMarks()).toEqual({ games: 0, platforms: 0, panel: 1 })
    tap(PAD.LEFT)
    expect(zoneMarks().games).toBe(1)
    expect(focusedOption()).toHaveAttribute('aria-label', title(columns)) // the same game
  })

  it('TDK-018/B volta a Jogos com o mesmo jogo; → num botão não faz nada', async () => {
    const { onExit } = setup()
    await ready()
    tap(PAD.RIGHT)
    tap(PAD.X)
    await within(panel()).findByRole('button', { name: 'Jogar' })
    tap(PAD.RIGHT)
    expect(zoneMarks().panel).toBe(1)
    tap(PAD.B)
    expect(onExit).not.toHaveBeenCalled()
    expect(zoneMarks().games).toBe(1)
    expect(focusedOption()).toHaveAttribute('aria-label', title(2))
  })

  it('TDK-018/A no botão lança com e sem contar tempo; A no visor não faz nada', async () => {
    app.MediaList.mockResolvedValue({
      media: [
        { id: 1, kind: 'screenshot', provider: 'x', path: 'p1', thumbPath: 't1', url: '/m/s1.jpg', thumbUrl: '/m/s1-t.jpg' },
        { id: 2, kind: 'screenshot', provider: 'x', path: 'p2', thumbPath: 't2', url: '/m/s2.jpg', thumbUrl: '/m/s2-t.jpg' },
      ],
    })
    setup()
    await ready()
    tap(PAD.RIGHT) // Jogo 002
    tap(PAD.X)
    await within(panel()).findByRole('button', { name: 'Jogar sem contar tempo' })
    tap(PAD.UP) // viewer (the gallery exists)
    await waitFor(() => expect(within(panel()).getByRole('region', { name: 'Imagens' })).toHaveAttribute('data-focused', 'true'))
    tap(PAD.A)
    await flush()
    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
    tap(PAD.DOWN) // Jogar
    tap(PAD.DOWN) // Jogar sem contar tempo
    tap(PAD.A)
    await waitFor(() => expect(app.LaunchGameUntracked).toHaveBeenCalledWith(2))
    expect(app.LaunchGame).not.toHaveBeenCalled()
  })

  it('TDK-018/A em Jogar chama LaunchGame com o jogo em foco', async () => {
    setup()
    await ready()
    tap(PAD.X)
    await within(panel()).findByRole('button', { name: 'Jogar' })
    tap(PAD.A)
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(1))
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
  })
})

describe('BigBox + Desktop: rolagem e pré-busca', () => {
  it('TDK-019/andar pela grade só rola ao passar da última fileira inteira e o foco nunca fica fora da área visível', async () => {
    setup()
    await ready()
    const { columns, rows } = GRID()
    expect(optionLabels()[0]).toBe(title(1))
    for (let r = 1; r < rows; r++) {
      key('ArrowDown')
      expect(optionLabels()[0]).toBe(title(1)) // inside the whole visible rows: no scroll
    }
    expect(focusedOption()).toHaveAttribute('aria-label', title((rows - 1) * columns + 1))
    key('ArrowDown') // past the last whole row: scrolls one row
    expect(optionLabels()[0]).toBe(title(columns + 1))
    const idx = optionLabels().indexOf(title(rows * columns + 1))
    expect(idx).toBeGreaterThanOrEqual(0)
    expect(idx).toBeLessThan(rows * columns) // inside the whole rows
    key('ArrowUp') // back inside: no scroll
    expect(optionLabels()[0]).toBe(title(columns + 1))
    for (let r = 0; r < rows; r++) key('ArrowUp')
    expect(optionLabels()[0]).toBe(title(1))
    expect(options().length).toBeLessThanOrEqual((rows + 1) * columns)
  })

  it('TDK-020/pré-busca por páginas com R\' (uma leitura em voo por aba, sem repetir)', async () => {
    install(400)
    const { columns, rows } = GRID()
    setup()
    await ready()
    const first = (app.ListGames.mock.calls[0][0] as { limit: number }).limit
    expect(first).toBeGreaterThanOrEqual((rows + 1) * columns)
    let open!: () => void
    lib.gates.set(first, new Promise<void>((r) => { open = r }))
    for (let i = 0; i < Math.ceil(first / columns); i++) key('ArrowDown')
    await waitFor(() => expect(app.ListGames.mock.calls.some((c) => (c[0] as { offset: number }).offset === first)).toBe(true))
    for (let i = 0; i < 6; i++) key('ArrowDown')
    await flush()
    const reads = app.ListGames.mock.calls.filter((c) => (c[0] as { offset: number }).offset === first)
    expect(reads).toHaveLength(1)
    expect(lib.inflight.max).toBeLessThanOrEqual(1)
    open()
    await flush()
    expect(app.ListGames.mock.calls.filter((c) => (c[0] as { offset: number }).offset === first)).toHaveLength(1)
  })
})

describe('BigBox + Desktop: tempo para zerar', () => {
  it('TDK-011/em Navegar a busca vem depois de 1.000 ms com o mesmo jogo; no Painel, na hora; sem botão de atualizar', async () => {
    setup()
    await ready()
    expect(fetchTrue()).toHaveLength(0)
    await wait(600)
    tap(PAD.RIGHT) // the timer restarts with the new game
    await wait(600)
    expect(fetchTrue()).toHaveLength(0)
    await wait(600)
    expect(fetchTrue()).toEqual([[2, true]])
    tap(PAD.RIGHT) // Jogo 003
    tap(PAD.X)
    await waitFor(() => expect(fetchTrue()).toContainEqual([3, true]))
    expect(screen.queryByRole('button', { name: /atualizar|confirmar|escolher|buscar/i })).toBeNull()
  })
})

describe('BigBox + Desktop: rodapé', () => {
  it('TDK-024/o rodapé muda a cada zona (Jogos, Plataformas, botão, visor, aba vazia) e nunca mostra Select+A', async () => {
    app.MediaList.mockResolvedValue({
      media: [
        { id: 1, kind: 'screenshot', provider: 'x', path: 'p1', thumbPath: 't1', url: '/m/s1.jpg', thumbUrl: '/m/s1-t.jpg' },
        { id: 2, kind: 'screenshot', provider: 'x', path: 'p2', thumbPath: 't2', url: '/m/s2.jpg', thumbUrl: '/m/s2-t.jpg' },
      ],
    })
    setup()
    await ready()
    const seen: string[][] = []
    expect(hints()).toEqual(['A Jogar', 'X Detalhes', 'LB/RB Plataforma', 'Select+B Fechar o jogo', 'B Sair'])
    seen.push(hints())
    tap(PAD.LEFT)
    expect(hints()).toEqual(['↑/↓ Plataforma', 'A Ver jogos', 'LB/RB Plataforma', 'B Voltar', 'Select+B Fechar o jogo'])
    seen.push(hints())
    tap(PAD.RIGHT)
    tap(PAD.X)
    await within(panel()).findByRole('button', { name: 'Jogar' })
    expect(hints()).toEqual(['A Jogar', '↑/↓ Mover', 'B Voltar', 'Select+B Fechar o jogo'])
    seen.push(hints())
    tap(PAD.UP)
    await waitFor(() => expect(hints()).toEqual(['←/→ Imagem', '↓ Botões', 'B Voltar', 'Select+B Fechar o jogo']))
    seen.push(hints())
    for (const h of seen) expect(h.join('|')).not.toMatch(/Select\+A/)
    expect(new Set(seen.map((h) => h.join('|'))).size).toBe(seen.length)
  })

  it('TDK-024/aba vazia: LB/RB Plataforma, ← Plataformas e B Sair', async () => {
    lib.ps2 = []
    setup({ initialPlatformId: 2 })
    expect(await screen.findByText('Nenhum jogo nesta plataforma')).toBeInTheDocument()
    expect(hints()).toEqual(['LB/RB Plataforma', '← Plataformas', 'B Sair'])
    expect(hints().join('|')).not.toMatch(/Select\+A/)
  })
})

describe('BigBox + Desktop: mouse', () => {
  it('TDK-025/clicar num cartão foca e volta a zona a Jogos; duplo clique lança; clicar numa plataforma a ativa', async () => {
    setup()
    await ready()
    tap(PAD.LEFT) // platforms zone
    expect(zoneMarks().platforms).toBe(1)
    fireEvent.click(options()[3])
    expect(focusedOption()).toHaveAttribute('aria-label', title(4))
    expect(zoneMarks().games).toBe(1)
    fireEvent.doubleClick(options()[3])
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(4))
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
  })

  it('TDK-025/clicar numa plataforma a ativa (BBX-065) e lê a lista dela', async () => {
    setup()
    await ready()
    fireEvent.click(within(nav()).getByRole('button', { name: /PlayStation 2/ }))
    await waitFor(() => expect(nav().querySelector('[aria-current="true"]')).toHaveTextContent('PlayStation 2'))
    await waitFor(() => expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ filter: { version: 1, platformIds: [2] } })))
    await waitFor(() => expect(optionLabels()).toEqual([title(1001), title(1002)]))
  })

  it('TDK-025/Jogar do painel lança; um lançamento pendente bloqueia o seguinte (UI-021)', async () => {
    const pending = deferred<unknown>()
    app.LaunchGame.mockImplementation(() => pending.promise)
    setup()
    await ready()
    fireEvent.click(within(panel()).getByRole('button', { name: 'Jogar' }))
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    expect(app.LaunchGame).toHaveBeenCalledWith(1)
    fireEvent.click(within(panel()).getByRole('button', { name: 'Jogar' }))
    fireEvent.doubleClick(options()[2])
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    await act(async () => {
      pending.resolve(session())
      await pending.promise
    })
  })
})

describe('BigBox + Desktop: só leituras', () => {
  it('TDK-031/em toda a navegação só são chamados as leituras e o lançamento; nenhum download de mídia', async () => {
    app.MediaList.mockResolvedValue({
      media: [
        { id: 1, kind: 'screenshot', provider: 'x', path: 'p1', thumbPath: 't1', url: '/m/s1.jpg', thumbUrl: '/m/s1-t.jpg' },
        { id: 2, kind: 'video', provider: 'x', path: 'v', thumbPath: '', url: '/m/v.mp4', thumbUrl: '' },
      ],
    })
    setup()
    await ready()
    for (const b of [PAD.DOWN, PAD.RIGHT, PAD.LEFT, PAD.LEFT, PAD.LEFT, PAD.DOWN, PAD.RB, PAD.LB, PAD.A, PAD.RIGHT, PAD.X, PAD.UP, PAD.DOWN,
      PAD.DOWN, PAD.A, PAD.Y, PAD.LEFT, PAD.B]) tap(b)
    for (const k of ['PageDown', 'PageUp', 'm', 'd', 'Escape']) key(k)
    await flush()
    expect(called.length).toBeGreaterThan(0)
    const allowed = ['ListGames', 'GetGame', 'MediaList', 'Catalog', 'CurrentSession', 'TimeToBeatGet', 'LaunchGame', 'LaunchGameUntracked', 'BringToFront']
    for (const n of called) expect(allowed, n).toContain(n)
    expect(called.filter((n) => /download|fetchMedia|MediaDownload|MediaFetch/i.test(n))).toEqual([])
  })
})

describe('BigBox + Desktop: suspensão, Select e retorno do jogo', () => {
  it('TDK-033/suspensão enquanto o jogo roda: entrada ignorada até session:changed ended do jogo', async () => {
    const { onExit } = setup()
    await ready()
    tap(PAD.A)
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    await flush()
    tap(PAD.A)
    tap(PAD.B)
    tap(PAD.RIGHT)
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(onExit).not.toHaveBeenCalled()
    expect(focusedOption()).toHaveAttribute('aria-label', title(1))
    emit('session:changed', session({ state: 'ended', endedAt: '2026-11-15T10:10:00.000Z' }))
    tap(PAD.RIGHT)
    expect(focusedOption()).toHaveAttribute('aria-label', title(2))
  })

  it('TDK-033/Select segurado bloqueia A, B, Y e ↑/↓ do direcional', async () => {
    const { onExit } = setup()
    await ready()
    padPressed.add(PAD.SELECT)
    frame()
    tap(PAD.A)
    tap(PAD.B)
    tap(PAD.Y)
    tap(PAD.DOWN)
    await flush()
    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(onExit).not.toHaveBeenCalled()
    expect(focusedOption()).toHaveAttribute('aria-label', title(1))
    padPressed.delete(PAD.SELECT)
    frame()
    tap(PAD.DOWN)
    expect(focusedOption()).toHaveAttribute('aria-label', title(GRID().columns + 1))
  })

  it('TDK-033/depois de game:closedByHotkey o Big Box volta com zona Jogos, foco no jogo que rodava e a fileira dele no meio', async () => {
    install(200)
    const { columns, rows } = GRID()
    setup()
    await ready()
    const target = 40 * columns + 1 // first game of row 40
    // walk down to row 40, then open the panel and launch from there
    for (let r = 0; r < 40; r++) key('ArrowDown')
    expect(focusedOption()).toHaveAttribute('aria-label', title(target))
    tap(PAD.X)
    await within(panel()).findByRole('button', { name: 'Jogar' })
    tap(PAD.A)
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(target))
    await flush()
    // while the game runs the view is moved elsewhere by data only: emulate the return
    emit('game:closedByHotkey', { gameId: target })
    await waitFor(() => expect(app.BringToFront).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(zoneMarks().games).toBe(1))
    expect(focusedOption()).toHaveAttribute('aria-label', title(target))
    expect(zoneMarks().panel).toBe(0)
    const idx = optionLabels().indexOf(title(target))
    expect(Math.floor(idx / columns)).toBe(Math.floor((rows - 1) / 2)) // the row sits in the middle (BBX-063)
    tap(PAD.RIGHT)
    expect(focusedOption()).toHaveAttribute('aria-label', title(target + 1)) // the entry is free again
  })
})
