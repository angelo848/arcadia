import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { stripWindow } from './viewModel'
import type { BigBoxViewModel, LibraryView } from './viewModel'
import { gridPageSize } from './useBigBoxData'
import type { SortKey } from '../lib/filters'
import { BIGBOX_THEMES } from './themes/registry'
import type { BigBoxThemeProps } from './themes/registry'
import { BigBox } from './BigBox'
import { useMediaEvents } from '../lib/metadata'
import { useTimeToBeatEvents } from '../lib/timeToBeat'
import { FocusVideo } from './FocusVideo'
import type { BigBoxDetailsProps } from './BigBoxDetails'
import type { BigBoxThemeDef } from './themes/registry'

// Phase 13 adjustments (T60: BBX-007, BBX-024, BBX-027, BBX-032..BBX-034, BBX-036, BBX-038, BBX-039, BBX-042, BBX-045):
// the shell also runs against a fake gamepad (navigator.getGamepads + a controlled requestAnimationFrame), a fake
// viewport (window.innerWidth/innerHeight) and a deterministic faixa.listCapacity (1920 -> 12, 2560 -> 13, 1280 -> 8).

// Tests of the Big Box shell (phase 13, T40; BBX-001..BBX-004, BBX-014, BBX-018, BBX-020, BBX-021, BBX-024,
// BBX-027, BBX-029, CON-022, CON-024). Fake window.go.main.App and window.runtime.
// Assumed props: <BigBox initialPlatformId initialGameId themeId onExit />. Assumed markup (Faixa):
// covers data-testid="bb-cover" with data-focused, tabs role="tab", the focused game title in a heading,
// the hints footer as text, the alert in role="alert". Keyboard: Enter = A, d = X, PageUp/PageDown = LB/RB,
// Escape = B, m = Y (sound). Assumed payloads: session:changed = SessionView, game:closedByHotkey = { gameId }.

type Fn = ReturnType<typeof vi.fn>

const originalThemes = BIGBOX_THEMES.map((t) => ({ ...t }))
let handlers: Map<string, Set<(...d: unknown[]) => void>>
let app: Record<string, Fn>
let called: string[]
let fullscreen: boolean
let runtime: Record<string, Fn>

const CATALOG = {
  platforms: [
    { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 3 },
    { id: 2, name: 'PlayStation 2', category: 'console', emulatorId: null, seedKey: 'ps2', iconUrl: '', gameCount: 1 },
  ],
  genres: [],
  companies: [],
  playModes: [],
}

const g = (id: number, title: string, platformId: number) => ({
  id, platformId, title, platform: platformId === 1 ? 'Windows' : 'PlayStation 2', path: `C:\\g\\${id}.exe`, status: 'not_played',
  favorite: false, year: null, totalPlaySeconds: 0, playCount: 0, firstPlayedAt: '', lastPlayedAt: '',
})
const ALL = [g(30, 'Celeste', 1), g(20, 'Hades', 1), g(10, 'Hollow Knight', 1), g(40, 'Gran Turismo', 2)]

const detailOf = (id: number) => ({
  ...ALL.find((x) => x.id === id)!, description: 'Descrição.', completedAt: null, developers: [], publishers: [], genres: [],
  playModes: [], launchType: 'exe', launchArgs: '', sessionPlaySeconds: 0 /* alterado em 2026-11-25 (fase 16) */, emulatorId: null,
})

function session(over: Record<string, unknown> = {}) {
  return {
    sessionId: 1, gameId: 30, title: 'Celeste', state: 'running', startedAt: '2026-11-15T10:00:00.000Z', endedAt: '',
    activeSeconds: 0, activeMs: 0, asOf: '2026-11-15T10:00:00.000Z', ...over,
  }
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

function install() {
  handlers = new Map()
  called = []
  fullscreen = false
  app = {
    ListGames: vi.fn(({ filter, offset, limit }: { filter: { platformIds?: number[] }; offset: number; limit: number }) => {
      const rows = filter.platformIds ? ALL.filter((x) => filter.platformIds!.includes(x.platformId)) : ALL
      return Promise.resolve({ games: rows.slice(offset, offset + limit), total: rows.length, offset })
    }),
    GetGame: vi.fn((id: number) => Promise.resolve(detailOf(id))),
    Catalog: vi.fn(() => Promise.resolve(CATALOG)),
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    GameSessions: vi.fn(() => Promise.resolve({ sessions: [], total: 0, offset: 0 })),
    LaunchGame: vi.fn((id: number) => Promise.resolve(session({ gameId: id }))),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    BringToFront: vi.fn(() => Promise.resolve()),
    // alterado em 2026-11-22 (fase 15): o shell lê o tempo para zerar (TimeToBeatGet, só leitura, BBX-059).
    TimeToBeatGet: vi.fn(() => Promise.resolve(null)),
  }
  // Every binding the shell touches is recorded by name; unknown names answer with an empty promise.
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
      if (!set) {
        set = new Set()
        handlers.set(name, set)
      }
      set.add(cb)
      return () => {
        set!.delete(cb)
      }
    }),
    WindowIsFullscreen: vi.fn(() => Promise.resolve(fullscreen)),
    WindowFullscreen: vi.fn(() => {
      fullscreen = true
    }),
    WindowUnfullscreen: vi.fn(() => {
      fullscreen = false
    }),
  }
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: proxy } }
  w.runtime = runtime
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  setViewport(1920, 1080)
  capacitySpy = vi.fn<(vp: { width: number; height: number }) => number>((vp) => (vp.width === 1280 ? 8 : vp.width === 2560 ? 13 : 12))
  const faixa = BIGBOX_THEMES.find((t) => t.id === 'faixa')!
  faixa.listCapacity = capacitySpy
}

let capacitySpy: ReturnType<typeof vi.fn<(vp: { width: number; height: number }) => number>>
const realViewport = { w: window.innerWidth, h: window.innerHeight }
function setViewport(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: height })
}
function resizeTo(width: number, height: number) {
  setViewport(width, height)
  act(() => {
    window.dispatchEvent(new Event('resize'))
  })
}

// ---- fake gamepad: standard mapping, edge-triggered by a controlled requestAnimationFrame ----
const PAD = { A: 0, B: 1, X: 2, Y: 3, LB: 4, RB: 5, UP: 12, DOWN: 13, LEFT: 14, RIGHT: 15 }
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
// One press and release of a button, as two animation frames.
function tap(button: number) {
  padPressed.add(button)
  frame()
  padPressed.delete(button)
  frame()
}
const settle = () => act(async () => { await new Promise((r) => setTimeout(r, 0)) })

function uninstall() {
  setViewport(realViewport.w, realViewport.h)
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
  vi.unstubAllGlobals()
  BIGBOX_THEMES.splice(0, BIGBOX_THEMES.length, ...originalThemes.map((t) => ({ ...t })))
}

function emit(name: string, ...data: unknown[]) {
  act(() => {
    for (const cb of [...(handlers.get(name) ?? [])]) cb(...data)
  })
}

function subscribed(): number {
  return [...handlers.values()].reduce((n, s) => n + s.size, 0)
}

// alterado em 2026-11-27 (fase 17): + librarySort e libraryView, as duas props novas do BigBox (BBX-067, BBX-068)
type Props = {
  initialPlatformId?: number | null
  initialGameId?: number | null
  themeId?: 'faixa' | 'mosaico' | 'roda'
  librarySort?: SortKey
  libraryView?: LibraryView
}
function setup(p: Props = {}) {
  const onExit = vi.fn()
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const MediaEvents = ({ children }: { children: ReactNode }) => {
    useMediaEvents()
    useTimeToBeatEvents() // alterado em 2026-11-22 (fase 15): o App monta o aviso timeToBeat:changed (TTB-017)
    return <>{children}</>
  }
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <MediaEvents>{children}</MediaEvents>
    </QueryClientProvider>
  )
  const r = render(
    <BigBox
      initialPlatformId={p.initialPlatformId ?? null}
      initialGameId={p.initialGameId ?? null}
      themeId={p.themeId ?? 'faixa'}
      librarySort={p.librarySort ?? 'title'}
      libraryView={p.libraryView ?? 'grid'}
      onExit={onExit}
    />,
    { wrapper },
  )
  return { onExit, ...r }
}

const covers = () => screen.queryAllByTestId('bb-cover')
const focusedCover = () => covers().find((c) => c.getAttribute('data-focused') === 'true')
const key = (k: string) => fireEvent.keyDown(window, { key: k })
async function ready(n = 1) {
  await waitFor(() => expect(covers().length).toBeGreaterThanOrEqual(n))
  await waitFor(() => expect(screen.getByRole('heading', { name: /./ })).toBeInTheDocument())
}
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 20)) })
const tabs = () => screen.getAllByRole('tab')
const activeTab = () => tabs().find((t) => t.getAttribute('aria-selected') === 'true')

beforeEach(install)
afterEach(uninstall)

describe('BigBox: tema, entrada e saída', () => {
  it('BBX-001/o Big Box monta o tema escolhido na tela Navegar com data-bigbox-theme', async () => {
    const { container } = setup()
    await ready()
    expect(container.querySelector('[data-bigbox-theme]')).toHaveAttribute('data-bigbox-theme', 'faixa')
    expect(screen.getByRole('heading', { name: /Celeste/ })).toBeInTheDocument()
    expect(screen.getByText('Fechar o jogo')).toBeInTheDocument() // the footer comes from the shell
  })

  it('BBX-001/o tema vem do registro: um tema registrado e disponível é o que desenha', async () => {
    const seen: BigBoxViewModel[] = []
    const mosaico = BIGBOX_THEMES.find((t) => t.id === 'mosaico')!
    mosaico.available = true
    mosaico.Component = ({ model }: BigBoxThemeProps) => {
      seen.push(model)
      return <div data-testid="fake-theme">{model.games.map((x) => x.title).join('|')}</div>
    }
    const { container } = setup({ themeId: 'mosaico' })
    await waitFor(() => expect(screen.getByTestId('fake-theme').textContent).toContain('Celeste'))
    expect(container.querySelector('[data-bigbox-theme]')).toHaveAttribute('data-bigbox-theme', 'mosaico')
    expect(screen.queryAllByTestId('bb-cover')).toHaveLength(0)
  })

  it('BBX-002/B em Navegar chama onExit; B em Detalhes volta a Navegar', async () => {
    const { onExit } = setup()
    await ready()
    key('d')
    expect(await screen.findByRole('button', { name: 'Jogar sem contar tempo' })).toBeInTheDocument()
    key('Escape')
    expect(screen.queryByRole('button', { name: 'Jogar sem contar tempo' })).toBeNull()
    expect(onExit).not.toHaveBeenCalled()
    key('Escape')
    expect(onExit).toHaveBeenCalledTimes(1)
  })

  it('BBX-003/nenhum modal abre', async () => {
    setup()
    await ready()
    for (const k of ['d', 'ArrowDown', 'ArrowUp', 'Escape', 'PageDown', 'Enter']) key(k)
    await flush()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('BBX-004/LB e RB trocam a aba e a lista passa a ser a da aba, com o foco no primeiro', async () => {
    setup()
    await ready(4)
    expect(tabs().map((t) => t.textContent)).toEqual(['Todos', 'Windows', 'PlayStation 2'])
    expect(covers()).toHaveLength(4)
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Hades/ })).toBeInTheDocument()

    key('PageDown')
    await waitFor(() => expect(activeTab()).toHaveTextContent('Windows'))
    await waitFor(() => expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ filter: { version: 1, platformIds: [1] } })))
    await waitFor(() => expect(covers()).toHaveLength(3))
    expect(screen.getByRole('heading', { name: /Celeste/ })).toBeInTheDocument()

    key('PageDown')
    await waitFor(() => expect(covers()).toHaveLength(1))
    expect(screen.getByRole('heading', { name: /Gran Turismo/ })).toBeInTheDocument()

    key('PageUp')
    await waitFor(() => expect(activeTab()).toHaveTextContent('Windows'))
  })

  it('BBX-027/o foco inicial é o jogo da janela normal quando carregado, e a aba inicial é a da janela', async () => {
    setup({ initialPlatformId: 1, initialGameId: 10 })
    await waitFor(() => expect(activeTab()).toHaveTextContent('Windows'))
    await waitFor(() => expect(screen.getByRole('heading', { name: /Hollow Knight/ })).toBeInTheDocument())
    expect(focusedCover()).toBeDefined()
  })

  it('BBX-027/jogo inicial que não está na aba cai no primeiro', async () => {
    setup({ initialPlatformId: 2, initialGameId: 10 })
    await waitFor(() => expect(activeTab()).toHaveTextContent('PlayStation 2'))
    await waitFor(() => expect(screen.getByRole('heading', { name: /Gran Turismo/ })).toBeInTheDocument())
  })

  it('BBX-021/biblioteca vazia e aba vazia mostram os textos e as dicas reduzidas', async () => {
    app.ListGames.mockImplementation(() => Promise.resolve({ games: [], total: 0, offset: 0 }))
    app.Catalog.mockResolvedValue({ ...CATALOG, platforms: [] })
    const first = setup()
    expect(await screen.findByText('Nenhum jogo na biblioteca')).toBeInTheDocument()
    expect(screen.getByText('Plataforma')).toBeInTheDocument()
    expect(screen.getByText('Sair')).toBeInTheDocument()
    expect(screen.queryByText('Detalhes')).toBeNull()
    expect(screen.queryByText('Fechar o jogo')).toBeNull()
    first.unmount()

    app.Catalog.mockResolvedValue(CATALOG)
    setup({ initialPlatformId: 2 })
    expect(await screen.findByText('Nenhum jogo nesta plataforma')).toBeInTheDocument()
    expect(screen.getByText('Plataforma')).toBeInTheDocument()
  })

  it('BBX-029/navegar (abas, foco, Detalhes, galeria, som) só faz leituras', async () => {
    setup()
    await ready()
    for (const k of ['ArrowRight', 'ArrowLeft', 'PageDown', 'PageUp', 'd', 'ArrowDown', 'ArrowRight', 'm', 'Escape']) key(k)
    await flush()
    expect(called.length).toBeGreaterThan(0)
    for (const n of called) {
      // alterado em 2026-11-22 (fase 15): + TimeToBeatGet (leitura, BBX-059)
      expect(['ListGames', 'GetGame', 'Catalog', 'MediaList', 'CurrentSession', 'GameSessions', 'TimeToBeatGet']).toContain(n)
    }
  })
})

describe('BigBox: lançamento', () => {
  it('BBX-014/A lança uma vez (UIX-046); um lançamento pendente bloqueia outro', async () => {
    const pending = deferred<unknown>()
    app.LaunchGame.mockImplementation(() => pending.promise)
    setup()
    await ready()

    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    expect(app.LaunchGame).toHaveBeenCalledWith(30)
    key('Enter')
    key('ArrowRight')
    key('Enter')
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()

    await act(async () => {
      pending.resolve(session())
      await pending.promise
    })
  })

  it('BBX-014/o botão Jogar do bloco lança o jogo em foco uma vez', async () => {
    setup()
    await ready()
    fireEvent.click(screen.getByRole('button', { name: 'Jogar' }))
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    expect(app.LaunchGame).toHaveBeenCalledWith(30)
  })

  it('BBX-014/X abre Detalhes e A em Jogar sem contar tempo chama LaunchGameUntracked', async () => {
    setup()
    await ready()
    key('ArrowRight') // Hades
    key('d')
    expect(await screen.findByRole('button', { name: 'Jogar sem contar tempo' })).toBeInTheDocument()
    key('ArrowRight') // BBX-033: → moves between the two buttons
    key('Enter')

    await waitFor(() => expect(app.LaunchGameUntracked).toHaveBeenCalledTimes(1))
    expect(app.LaunchGameUntracked).toHaveBeenCalledWith(20)
    expect(app.LaunchGame).not.toHaveBeenCalled()
  })

  it('BBX-014/Jogar sem contar tempo só existe nos Detalhes', async () => {
    setup()
    await ready()
    expect(screen.queryByRole('button', { name: 'Jogar sem contar tempo' })).toBeNull()
    key('d')
    expect(await screen.findByRole('button', { name: 'Jogar sem contar tempo' })).toBeInTheDocument()
  })

  it('BBX-018/depois do lançamento a entrada fica suspensa até session:changed ended do jogo', async () => {
    const { onExit } = setup()
    await ready()
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    await flush()

    // A and B pressed inside the game neither launch nor leave [BBX-018].
    key('Enter')
    key('Escape')
    key('ArrowRight')
    key('d')
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(onExit).not.toHaveBeenCalled()
    expect(screen.getByRole('heading', { name: /Celeste/ })).toBeInTheDocument()

    emit('session:changed', session({ sessionId: 2, gameId: 99, state: 'ended', endedAt: '2026-11-15T10:05:00.000Z' }))
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Celeste/ })).toBeInTheDocument()

    emit('session:changed', session({ state: 'ended', endedAt: '2026-11-15T10:10:00.000Z' }))
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Hades/ })).toBeInTheDocument()
  })

  it('BBX-018/session:changed discarded também libera a entrada', async () => {
    setup()
    await ready()
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    await flush()

    emit('session:changed', session({ state: 'discarded' }))
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Hades/ })).toBeInTheDocument()
  })

  it('BBX-018/focus da janela depois de blur libera; focus sem blur antes não libera', async () => {
    setup()
    await ready()
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    await flush()

    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Celeste/ })).toBeInTheDocument()

    act(() => {
      window.dispatchEvent(new Event('blur'))
    })
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Hades/ })).toBeInTheDocument()
  })

  it('BBX-020/lançamento recusado com -32002, -32003 ou agente desconectado mostra o aviso com role alert e o Big Box segue navegável', async () => {
    const messages = [
      'Já há um jogo em andamento: Celeste. Feche-o antes de iniciar outro.',
      'O executável do jogo não foi encontrado.',
      'O agente não está respondendo. Tente de novo.',
    ]
    for (const msg of messages) {
      app.LaunchGame.mockReset()
      app.LaunchGame.mockRejectedValue(msg)
      const { unmount } = setup()
      await ready()

      key('Enter')

      const alert = await screen.findByRole('alert')
      expect(alert).toHaveTextContent(msg)
      key('ArrowRight')
      expect(screen.getByRole('heading', { name: /Hades/ })).toBeInTheDocument()
      key('Enter')
      await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(2))
      expect(app.LaunchGame).toHaveBeenLastCalledWith(20)
      unmount()
    }
  })
})

describe('BigBox: som', () => {
  it('BBX-013/Y alterna o som do vídeo quando ele está visível', async () => {
    app.MediaList.mockResolvedValue({
      media: [
        { id: 1, kind: 'screenshot', provider: 'x', path: 'p', thumbPath: 't', url: '/m/s.jpg', thumbUrl: '/m/s-t.jpg' },
        { id: 2, kind: 'video', provider: 'x', path: 'v', thumbPath: '', url: '/m/v.mp4', thumbUrl: '' },
      ],
    })
    setup()
    await ready()
    const video = await screen.findByTestId('bb-video', undefined, { timeout: 5000 })
    // alterado em 2026-11-17 (1.0.3): o som começa ligado e Y desliga
    expect((video as HTMLVideoElement).muted).toBe(false)
    expect(await screen.findByText('Som')).toBeInTheDocument()

    key('m')

    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(true))
    key('m')
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(false))
  }, 15000)

  it('BBX-013/sem vídeo visível Y não faz nada e a dica Som não aparece', async () => {
    setup()
    await ready()
    expect(screen.queryByText('Som')).toBeNull()
    key('m')
    expect(screen.queryByTestId('bb-video')).toBeNull()
  })
})

describe('BigBox: tema de eixo vertical', () => {
  it('BBX-024/um tema falso de eixo vertical navega com ↑/↓ pelo mesmo reducer', async () => {
    const roda = BIGBOX_THEMES.find((t) => t.id === 'roda')!
    roda.available = true
    roda.axis = 'vertical'
    roda.Component = ({ model }: BigBoxThemeProps) => <div data-testid="vert" data-focus={model.focus}>{model.focused?.game.title}</div>
    const cap = vi.fn(() => 12)
    roda.listCapacity = cap
    setup({ themeId: 'roda' })
    await waitFor(() => expect(screen.getByTestId('vert')).toHaveTextContent('Celeste'))
    expect(cap).toHaveBeenCalledWith({ width: 1920, height: 1080 }) // BBX-024: the shell asks the theme for its capacity

    key('ArrowDown')
    expect(screen.getByTestId('vert')).toHaveTextContent('Hades')
    key('ArrowUp')
    expect(screen.getByTestId('vert')).toHaveTextContent('Celeste')
    key('ArrowRight')
    expect(screen.getByTestId('vert')).toHaveTextContent('Celeste')
  })
})

describe('BigBox: retorno do jogo (CON-022..CON-024)', () => {
  async function launchFromDetails() {
    const view = setup()
    await ready()
    key('ArrowRight') // Hades
    key('d')
    await screen.findByRole('button', { name: 'Jogar sem contar tempo' })
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(20))
    await flush()
    return view
  }

  it('CON-022/game:closedByHotkey com o Big Box ativo chama BringToFront e enterBigBox, sai de Detalhes, libera a suspensão e foca o jogo (na aba)', async () => {
    await launchFromDetails()
    key('ArrowRight')
    expect(screen.getByRole('button', { name: 'Jogar sem contar tempo' })).toBeInTheDocument() // still suspended

    emit('game:closedByHotkey', { gameId: 20 })

    await waitFor(() => expect(app.BringToFront).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(runtime.WindowFullscreen).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole('button', { name: 'Jogar sem contar tempo' })).toBeNull()
    expect(screen.getByRole('heading', { name: /Hades/ })).toBeInTheDocument()
    expect(focusedCover()).toHaveTextContent('Hades')
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Hollow Knight/ })).toBeInTheDocument()
  })

  it('CON-022/gameId de outro jogo vale o mesmo retorno e o foco não muda', async () => {
    await launchFromDetails()

    emit('game:closedByHotkey', { gameId: 999 })

    await waitFor(() => expect(app.BringToFront).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(runtime.WindowFullscreen).toHaveBeenCalled())
    expect(screen.getByRole('heading', { name: /Hades/ })).toBeInTheDocument()
    key('ArrowLeft')
    expect(screen.getByRole('heading', { name: /Celeste/ })).toBeInTheDocument()
  })

  it('CON-022/BringToFront que falha é ignorado e o Big Box volta mesmo assim', async () => {
    app.BringToFront.mockRejectedValue('falhou')
    await launchFromDetails()

    emit('game:closedByHotkey', { gameId: 20 })

    await waitFor(() => expect(app.BringToFront).toHaveBeenCalled())
    await flush()
    key('ArrowRight')
    expect(screen.getByRole('heading', { name: /Hollow Knight/ })).toBeInTheDocument()
  })

  it('CON-022/com o Big Box inativo nada acontece (BringToFront não é chamado) e as assinaturas são desfeitas ao desmontar', async () => {
    const { unmount } = setup()
    await ready()
    expect(handlers.get('game:closedByHotkey')?.size).toBe(1)
    expect(handlers.get('session:changed')?.size ?? 0).toBeGreaterThan(0)

    unmount()

    expect(subscribed()).toBe(0)
    emit('game:closedByHotkey', { gameId: 20 })
    emit('session:changed', session({ state: 'ended' }))
    await flush()
    expect(app.BringToFront).not.toHaveBeenCalled()
    expect(runtime.WindowFullscreen).not.toHaveBeenCalled()
  })

  it('CON-024/o shell não tem ação de fechar jogo nem de encerrar sessão: os bindings chamados são só leituras, LaunchGame, LaunchGameUntracked e BringToFront', async () => {
    await launchFromDetails()
    emit('session:changed', session({ gameId: 20, state: 'ended' }))
    key('Enter')
    emit('game:closedByHotkey', { gameId: 20 })
    await flush()

    const allowed = [
      'ListGames', 'GetGame', 'Catalog', 'MediaList', 'CurrentSession', 'GameSessions',
      'LaunchGame', 'LaunchGameUntracked', 'BringToFront',
      'TimeToBeatGet', // alterado em 2026-11-22 (fase 15): leitura, BBX-059
    ]
    for (const n of called) expect(allowed).toContain(n)
    expect(screen.queryByRole('button', { name: /fechar|encerrar|parar/i })).toBeNull()
  })
})

// ---------------------------------------------------------------------------------------------------------
// Ajustes do teste com controle (T60)
// ---------------------------------------------------------------------------------------------------------

const SHOTS = {
  media: [
    { id: 1, kind: 'screenshot', provider: 'x', path: 'p1', thumbPath: 't1', url: '/m/s1.jpg', thumbUrl: '/m/s1-t.jpg' },
    { id: 2, kind: 'screenshot', provider: 'x', path: 'p2', thumbPath: 't2', url: '/m/s2.jpg', thumbUrl: '/m/s2-t.jpg' },
  ],
}
const galleryThumbs = (c: HTMLElement) => c.querySelector('[data-bb-thumbs]') as HTMLElement | null
const title = (n: number) => `Jogo ${String(n).padStart(3, '0')}`

type Library = {
  rows: ReturnType<typeof g>[]
  gates: Map<number, Promise<void>> // offset -> response held until the promise resolves
  failures: Map<number, number> // offset -> how many requests still fail
  inflight: { now: number; max: number }
}
// A single-platform library of n games answering ListGames by offset/limit with the tab total, as the real binding does.
function bigLibrary(n: number): Library {
  const rows = Array.from({ length: n }, (_, i) => g(i + 1, title(i + 1), 1))
  const lib: Library = { rows, gates: new Map(), failures: new Map(), inflight: { now: 0, max: 0 } }
  app.Catalog.mockResolvedValue({ ...CATALOG, platforms: [{ ...CATALOG.platforms[0], gameCount: n }] })
  app.GetGame.mockImplementation((id: number) =>
    Promise.resolve({ ...rows[id - 1], description: 'Descrição.', completedAt: null, developers: [], publishers: [], genres: [], playModes: [], launchType: 'exe', launchArgs: '', sessionPlaySeconds: 0 /* alterado em 2026-11-25 (fase 16) */, emulatorId: null }),
  )
  app.ListGames.mockImplementation(async ({ offset, limit }: { offset: number; limit: number }) => {
    lib.inflight.now += 1
    lib.inflight.max = Math.max(lib.inflight.max, lib.inflight.now)
    try {
      await lib.gates.get(offset)
      const left = lib.failures.get(offset) ?? 0
      if (left > 0) {
        lib.failures.set(offset, left - 1)
        throw new Error('falha de leitura')
      }
      return { games: rows.slice(offset, offset + limit), total: n, offset }
    } finally {
      lib.inflight.now -= 1
    }
  })
  return lib
}
const offsets = () => app.ListGames.mock.calls.map((c) => (c[0] as { offset: number }).offset)
const limits = () => app.ListGames.mock.calls.map((c) => (c[0] as { limit: number }).limit)
function gate(lib: Library, offset: number): () => void {
  let open!: () => void
  lib.gates.set(offset, new Promise<void>((r) => { open = r }))
  return open
}
async function readyPad() {
  await ready()
  frame() // primes the edge detector of the gamepad
}

describe('BigBox: controle nos Detalhes (BBX-032..BBX-036)', () => {
  beforeEach(() => {
    installPad()
    app.MediaList.mockResolvedValue(SHOTS)
  })
  afterEach(uninstallPad)

  async function openDetails() {
    const view = setup()
    await readyPad()
    tap(PAD.X)
    await screen.findByRole('button', { name: 'Jogar sem contar tempo' })
    await waitFor(() => expect(view.container.querySelectorAll('[data-gallery-item]')).toHaveLength(2))
    return view
  }
  const play = () => screen.getByRole('button', { name: 'Jogar' })
  const untracked = () => screen.getByRole('button', { name: 'Jogar sem contar tempo' })

  it('BBX-032/X abre Detalhes com Jogar em foco; ↓ no direcional leva o foco à galeria; ↑ volta a Jogar (data-focused no DOM)', async () => {
    const { container } = await openDetails()
    expect(play()).toHaveAttribute('data-focused', 'true')
    expect(galleryThumbs(container)).not.toHaveAttribute('data-focused', 'true')

    tap(PAD.DOWN)
    expect(galleryThumbs(container)).toHaveAttribute('data-focused', 'true')
    expect(play()).not.toHaveAttribute('data-focused', 'true')
    expect(untracked()).not.toHaveAttribute('data-focused', 'true')

    tap(PAD.UP)
    expect(play()).toHaveAttribute('data-focused', 'true')
    expect(galleryThumbs(container)).not.toHaveAttribute('data-focused', 'true')
  })

  it('BBX-033/→ no direcional com foco em Jogar foca Jogar sem contar tempo e A chama LaunchGameUntracked', async () => {
    await openDetails()
    tap(PAD.RIGHT)
    expect(untracked()).toHaveAttribute('data-focused', 'true')
    expect(play()).not.toHaveAttribute('data-focused', 'true')
    tap(PAD.A)
    await waitFor(() => expect(app.LaunchGameUntracked).toHaveBeenCalledTimes(1))
    expect(app.LaunchGameUntracked).toHaveBeenCalledWith(30)
    expect(app.LaunchGame).not.toHaveBeenCalled()
  })

  it('BBX-034/com foco na galeria → troca o item do visor e o contador vai a 2 / N; A não lança', async () => {
    const { container } = await openDetails()
    tap(PAD.DOWN)
    const viewer = () => container.querySelector('[data-bb-viewer]') as HTMLElement
    expect(viewer().textContent).toContain('1 / 2')
    expect(viewer().querySelector('img')?.getAttribute('src')).toBe('/m/s1.jpg')
    tap(PAD.RIGHT)
    expect(viewer().textContent).toContain('2 / 2')
    expect(viewer().querySelector('img')?.getAttribute('src')).toBe('/m/s2.jpg')
    tap(PAD.A)
    await settle()
    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
  })

  it('BBX-032/media:changed que esvazia a galeria com foco nela devolve o foco ao botão', async () => {
    const { container } = await openDetails()
    tap(PAD.DOWN)
    expect(galleryThumbs(container)).toHaveAttribute('data-focused', 'true')
    app.MediaList.mockResolvedValue({ media: [] })
    emit('media:changed', { gameId: 30 })
    await waitFor(() => expect(galleryThumbs(container)).toBeNull())
    expect(play()).toHaveAttribute('data-focused', 'true')
  })

  it('BBX-036/PageDown nos detalhes rola a descrição e não troca a aba', async () => {
    const scrollBy = vi.fn()
    const proto = HTMLElement.prototype as unknown as { scrollBy?: unknown }
    proto.scrollBy = scrollBy
    try {
      await openDetails()
      key('PageDown')
      await waitFor(() => expect(scrollBy).toHaveBeenCalled())
      expect(scrollBy.mock.instances.at(-1)).toBe(screen.getByRole('region', { name: /descrição/i }))
      expect(untracked()).toBeInTheDocument() // still in Detalhes
      expect(app.ListGames.mock.calls.every((c) => (c[0] as { filter: { platformIds?: number[] } }).filter.platformIds === undefined)).toBe(true)
    } finally {
      delete proto.scrollBy
    }
  })
})

describe('BigBox: leitura por páginas e faixa (BBX-007, BBX-027, BBX-038, BBX-039, BBX-045)', () => {
  beforeEach(installPad)
  afterEach(uninstallPad)

  it('BBX-038/"Carregando jogos…" só até a primeira página (15 de 120); depois a faixa aparece e as páginas seguintes chegam sem o aviso', async () => {
    const lib = bigLibrary(120)
    const open = gate(lib, 0)
    setup()
    expect(await screen.findByText('Carregando jogos…')).toBeInTheDocument()
    expect(covers()).toHaveLength(0)
    open()
    await ready(13)
    expect(screen.queryByText('Carregando jogos…')).toBeNull()
    expect(offsets()).toEqual([0])
    expect(limits()).toEqual([15])
    frame()
    tap(PAD.RIGHT) // f = 1: the second page is requested
    await waitFor(() => expect(offsets()).toEqual([0, 15]))
    await settle()
    expect(screen.queryByText('Carregando jogos…')).toBeNull()
    expect(covers().length).toBeGreaterThanOrEqual(13)
  })

  it('BBX-038/LB/RB funcionam com a primeira página pendente', async () => {
    const lib = bigLibrary(120)
    gate(lib, 0)
    setup()
    await screen.findByText('Carregando jogos…')
    await waitFor(() => expect(tabs().length).toBeGreaterThan(1))
    frame()
    tap(PAD.RB)
    await waitFor(() => expect(activeTab()).toHaveTextContent('Windows'))
    tap(PAD.LB)
    await waitFor(() => expect(activeTab()).toHaveTextContent('Todos'))
  })

  it('BBX-038/falha na primeira página mostra o aviso role alert e o texto de aba vazia', async () => {
    const lib = bigLibrary(120)
    lib.failures.set(0, 1)
    setup()
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText(/Nenhum jogo/)).toBeInTheDocument()
    expect(screen.queryByText('Carregando jogos…')).toBeNull()
  })

  it('BBX-038/falha na segunda página mostra o aviso e mantém a faixa; a próxima seta → tenta de novo uma vez', async () => {
    const lib = bigLibrary(120)
    lib.failures.set(15, 1)
    setup()
    await readyPad()
    tap(PAD.RIGHT) // f = 1 asks for offset 15, which fails
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(covers().length).toBeGreaterThanOrEqual(13)
    expect(offsets().filter((o) => o === 15)).toHaveLength(1)
    await settle()
    expect(offsets().filter((o) => o === 15)).toHaveLength(1) // no retry loop without moving the focus
    tap(PAD.RIGHT) // f = 2: one new attempt, which works
    await waitFor(() => expect(offsets().filter((o) => o === 15)).toHaveLength(2))
    await settle()
    expect(offsets().filter((o) => o === 15)).toHaveLength(2)
  })

  it('BBX-007/andar com → de 0 a 39 numa aba de 40 com V 12 pede offset 15 em f 1 e offset 30 em f 16, cada um uma vez, nunca dois em voo', async () => {
    const lib = bigLibrary(40)
    setup()
    await readyPad()
    expect(offsets()).toEqual([0])
    for (let f = 1; f <= 39; f++) {
      tap(PAD.RIGHT)
      await settle()
      if (f === 1) expect(offsets()).toEqual([0, 15])
      if (f === 15) expect(offsets()).toEqual([0, 15])
      if (f === 16) expect(offsets()).toEqual([0, 15, 30])
    }
    expect(offsets()).toEqual([0, 15, 30])
    expect(limits()).toEqual([15, 15, 15])
    expect(lib.inflight.max).toBe(1)
    expect(screen.getByRole('heading', { name: new RegExp(title(40)) })).toBeInTheDocument()
  })

  it('BBX-007/com a página pendente, → em loaded − 1 não move o foco; quando ela chega, → avança', async () => {
    const lib = bigLibrary(120)
    const open = gate(lib, 15)
    setup()
    await readyPad()
    for (let i = 0; i < 20; i++) tap(PAD.RIGHT) // 14 steps are possible, the rest is held at the last loaded game
    await waitFor(() => expect(offsets()).toEqual([0, 15]))
    expect(screen.getByRole('heading', { name: new RegExp(title(15)) })).toBeInTheDocument()
    expect(screen.getByTestId('bb-strip-counter')).toHaveTextContent('15 / 120')
    open()
    await settle()
    tap(PAD.RIGHT)
    expect(screen.getByRole('heading', { name: new RegExp(title(16)) })).toBeInTheDocument()
  })

  it('BBX-007/resize que aumenta V dispara a pré-busca sem mexer no foco', async () => {
    bigLibrary(40)
    setup()
    await readyPad()
    expect(offsets()).toEqual([0]) // 15 − 0 = 15, not < 15
    resizeTo(2560, 1440) // V = 13 -> 15 − 0 < 16
    await waitFor(() => expect(offsets()).toEqual([0, 15]))
    expect(screen.getByRole('heading', { name: new RegExp(title(1)) })).toBeInTheDocument()
    expect(focusedCover()).toHaveAttribute('aria-label', title(1))
  })

  it('BBX-027/jogo inicial na primeira página perto do fim: "Carregando jogos…" até a segunda página e foco nele com a faixa cheia', async () => {
    const lib = bigLibrary(120)
    const open = gate(lib, 15)
    setup({ initialGameId: 14 }) // index 13 of the first 15
    await waitFor(() => expect(offsets()).toEqual([0, 15]))
    expect(screen.getByText('Carregando jogos…')).toBeInTheDocument()
    expect(screen.queryByRole('heading')).toBeNull()
    open()
    await waitFor(() => expect(screen.getByRole('heading', { name: new RegExp(title(14)) })).toBeInTheDocument())
    expect(screen.queryByText('Carregando jogos…')).toBeNull()
    expect(focusedCover()).toHaveAttribute('aria-label', title(14))
    const w = stripWindow(120, 13, 12)
    expect(covers()).toHaveLength(w.end - w.start) // full strip, no hole
  })

  it('BBX-027/jogo inicial fora da primeira página: foco no primeiro jogo, sem ler outras páginas', async () => {
    bigLibrary(120)
    setup({ initialGameId: 100 })
    await ready(13)
    await settle()
    expect(screen.getByRole('heading', { name: new RegExp(title(1)) })).toBeInTheDocument()
    expect(offsets()).toEqual([0])
  })

  it('BBX-039/ao trocar para uma aba de 40 jogos com janela 1920×1080, a faixa mostra V + 1 capas a partir da primeira, com só a primeira página lida', async () => {
    bigLibrary(40)
    setup()
    await readyPad()
    tap(PAD.RB)
    await waitFor(() => expect(activeTab()).toHaveTextContent('Windows'))
    await waitFor(() => expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ filter: { version: 1, platformIds: [1] } })))
    await waitFor(() => expect(covers()).toHaveLength(13))
    expect(covers()[0]).toHaveAttribute('data-focused', 'true')
    expect(covers()[0]).toHaveAttribute('aria-label', title(1))
    const platformPages = app.ListGames.mock.calls.filter((c) => (c[0] as { filter: { platformIds?: number[] } }).filter.platformIds)
    expect(platformPages.map((c) => (c[0] as { offset: number }).offset)).toEqual([0])
  })

  it('BBX-039/com as páginas respondendo na hora, nenhum índice da janela fica sem jogo em toda a varredura de → até o fim', async () => {
    bigLibrary(40)
    setup()
    await readyPad()
    for (let f = 0; f < 40; f++) {
      if (f > 0) tap(PAD.RIGHT)
      await settle()
      const w = stripWindow(40, f, 12)
      const labels = covers().map((c) => c.getAttribute('aria-label'))
      expect(labels, `f = ${f}`).toEqual(Array.from({ length: w.end - w.start }, (_, i) => title(w.start + i + 1)))
      expect(focusedCover(), `f = ${f}`).toHaveAttribute('aria-label', title(f + 1))
    }
  })

  it('BBX-039/resize da janela muda a capacidade; desmontar remove o listener', async () => {
    bigLibrary(40)
    const add = vi.spyOn(window, 'addEventListener')
    const remove = vi.spyOn(window, 'removeEventListener')
    const { unmount } = setup()
    await readyPad()
    expect(covers()).toHaveLength(13)
    resizeTo(1280, 720) // V = 8
    await waitFor(() => expect(covers()).toHaveLength(9))
    expect(capacitySpy).toHaveBeenLastCalledWith({ width: 1280, height: 720 })
    const handler = add.mock.calls.find((c) => c[0] === 'resize')?.[1]
    expect(handler).toBeDefined()
    unmount()
    expect(remove.mock.calls.some((c) => c[0] === 'resize' && c[1] === handler)).toBe(true)
    add.mockRestore()
    remove.mockRestore()
  })

  it('BBX-045/o contador vai de "1 / 40" a "2 / 40" com → e não aparece durante "Carregando jogos…"', async () => {
    const lib = bigLibrary(40)
    const open = gate(lib, 0)
    setup()
    await screen.findByText('Carregando jogos…')
    expect(screen.queryByTestId('bb-strip-counter')).toBeNull()
    open()
    await ready(13)
    frame()
    expect(screen.getByTestId('bb-strip-counter')).toHaveTextContent('1 / 40')
    tap(PAD.RIGHT)
    expect(screen.getByTestId('bb-strip-counter')).toHaveTextContent('2 / 40')
  })

  it('BBX-042/tema, aviso e rodapé ficam dentro de data-bb-safe; o fundo fica fora', async () => {
    app.LaunchGame.mockRejectedValue('O executável do jogo não foi encontrado.')
    const { container } = setup()
    await ready()
    key('Enter')
    const alert = await screen.findByRole('alert')
    const safe = container.querySelector('[data-bb-safe]') as HTMLElement
    expect(safe).toBeTruthy()
    expect(safe.contains(screen.getByRole('tablist'))).toBe(true)
    expect(safe.contains(screen.getAllByTestId('bb-cover')[0])).toBe(true)
    expect(safe.contains(alert)).toBe(true)
    expect(safe.contains(screen.getByText('Fechar o jogo'))).toBe(true)
    const background = container.querySelector('[data-testid="bb-gradient"], [data-testid="bb-art-layer"]')
    expect(background).not.toBeNull()
    expect(safe.contains(background)).toBe(false)
  })
})

// ---------------------------------------------------------------------------------------------------------
// Fase 15 (T31): shell v2 com temas falsos, fundo, vídeo na capa, Detalhes próprios e tempo para zerar.
// Os temas falsos reaproveitam as entradas 'mosaico' e 'roda' do registro (restauradas em uninstall()).
// Marcação assumida de FocusVideo: <video data-testid="bb-video"> (como BackgroundStage).
// ---------------------------------------------------------------------------------------------------------

let modelLog: BigBoxViewModel[]
let propKeys: string[][]
function SimpleTheme(p: BigBoxThemeProps) {
  const { model, details } = p
  modelLog.push(model)
  propKeys.push(Object.keys(p).sort())
  if (model.screen === 'details') return <div data-testid="simple-details">{details}</div>
  return (
    <div data-testid="simple" data-focus={model.focus} data-start={model.stripStart} data-count={model.games.length}>
      {model.focused?.game.title ?? ''}
    </div>
  )
}
// A cover-video theme: the focused game gets its own card with FocusVideo inside (BBX-053).
function VideoTheme(p: BigBoxThemeProps) {
  const { model, details, onVideoError } = p
  if (model.screen === 'details') return <div data-testid="simple-details">{details}</div>
  const f = model.focused
  return (
    <div data-testid="simple">
      {f?.game.title}
      <div data-testid="card">
        {f && f.videoUrl && (
          <FocusVideo gameId={f.game.id} url={f.videoUrl} active={model.screen === 'browse'} sound={model.sound} failed={model.videoFailed} onError={onVideoError} />
        )}
      </div>
    </div>
  )
}
function useFake(id: 'mosaico' | 'roda', patch: Partial<BigBoxThemeDef> = {}) {
  const t = BIGBOX_THEMES.find((x) => x.id === id)!
  Object.assign(t, { available: true, Component: SimpleTheme, listCapacity: () => 12 }, patch)
}
const simple = () => screen.getByTestId('simple')
const readySimple = () => waitFor(() => expect(screen.getByTestId('simple')).toHaveTextContent(/./))
const wait = (ms: number) => act(async () => { await new Promise((r) => setTimeout(r, ms)) })
const ttbCalls = () => app.TimeToBeatGet.mock.calls as unknown as Array<[number, boolean]>
const fetchTrue = () => ttbCalls().filter((c) => c[1] === true)
const fetchFalse = (id: number) => ttbCalls().filter((c) => c[0] === id && c[1] === false)
const VIDEO = { id: 2, kind: 'video', provider: 'x', path: 'v', thumbPath: '', url: '/m/v.mp4', thumbUrl: '' }
const SHOT = { id: 1, kind: 'screenshot', provider: 'x', path: 'p', thumbPath: 't', url: '/m/s.jpg', thumbUrl: '/m/s-t.jpg' }

beforeEach(() => {
  modelLog = []
  propKeys = []
})

describe('BigBox v2: contrato de tema e grade (BBX-047, BBX-050, BBX-055)', () => {
  it.each([
    { name: 'horizontal', axis: 'horizontal', columns: 1, move: 'ArrowRight', back: 'ArrowLeft', ignored: 'ArrowDown', next: 'Jogo 002' },
    { name: 'vertical', axis: 'vertical', columns: 1, move: 'ArrowDown', back: 'ArrowUp', ignored: 'ArrowRight', next: 'Jogo 002' },
  ] as const)('BBX-047/tema falso $name navega pelo mesmo reducer e recebe só o modelo e as ações', async (c) => {
    bigLibrary(30)
    useFake('roda', { axis: c.axis, columns: c.columns, gridRows: undefined })
    setup({ themeId: 'roda' })
    await readySimple()
    expect(simple()).toHaveTextContent('Jogo 001')
    key(c.ignored)
    expect(simple()).toHaveTextContent('Jogo 001')
    key(c.move)
    expect(simple()).toHaveTextContent(c.next)
    key(c.back)
    expect(simple()).toHaveTextContent('Jogo 001')
    // alterado em 2026-11-27 (fase 17): onTab (opcional, BBX-065) pode ou não vir; as demais props continuam as mesmas
    for (const keys of propKeys) expect(keys.filter((k) => k !== 'onTab')).toEqual(['details', 'model', 'onDetails', 'onPick', 'onPlay', 'onVideoError'])
    expect(modelLog.at(-1)).toMatchObject({ axis: c.axis, columns: c.columns })
  })

  it('BBX-047/tema falso grid (3 colunas): ←/→ andam de um em um, ↑/↓ de uma fileira, nas pontas não passa', async () => {
    bigLibrary(30)
    useFake('roda', { axis: 'grid', columns: 3, listCapacity: () => 3, gridRows: () => 1 })
    setup({ themeId: 'roda' })
    await readySimple()
    key('ArrowUp')
    expect(simple()).toHaveTextContent('Jogo 001')
    key('ArrowLeft')
    expect(simple()).toHaveTextContent('Jogo 001')
    key('ArrowDown')
    expect(simple()).toHaveTextContent('Jogo 004')
    key('ArrowRight')
    expect(simple()).toHaveTextContent('Jogo 005')
    key('ArrowUp')
    expect(simple()).toHaveTextContent('Jogo 002')
    key('ArrowLeft')
    expect(simple()).toHaveTextContent('Jogo 001')
    expect(modelLog.at(-1)).toMatchObject({ axis: 'grid', columns: 3 })
    // alterado em 2026-11-27 (fase 17): onTab (opcional, BBX-065) pode ou não vir
    for (const keys of propKeys) expect(keys.filter((k) => k !== 'onTab')).toEqual(['details', 'model', 'onDetails', 'onPick', 'onPlay', 'onVideoError'])
  })

  it('BBX-047/o shell pergunta a capacidade ao tema e, na grade, as fileiras (gridRows) com o viewport', async () => {
    bigLibrary(30)
    const cap = vi.fn(() => 5)
    const rows = vi.fn(() => 2)
    useFake('roda', { axis: 'grid', columns: 5, listCapacity: cap, gridRows: rows })
    setup({ themeId: 'roda' })
    await readySimple()
    expect(cap).toHaveBeenCalledWith({ width: 1920, height: 1080 })
    expect(rows).toHaveBeenCalledWith({ width: 1920, height: 1080 })
    // pages of (R + 1 + 3) × C = (2 + 1 + 3) × 5 = 30
    expect(limits()[0]).toBe(30)
  })

  it('BBX-050/grade 5 colunas, R 1: páginas de 25, a próxima pedida em f 5 e f 30, cada uma uma vez, nunca duas em voo, e a janela nunca passa de L', async () => {
    const lib = bigLibrary(60)
    const openPage2 = gate(lib, 25)
    useFake('roda', { axis: 'grid', columns: 5, listCapacity: () => 5, gridRows: () => 1 })
    setup({ themeId: 'roda' })
    await readySimple()
    expect(offsets()).toEqual([0])
    expect(limits()).toEqual([25])
    for (let i = 0; i < 4; i++) {
      key('ArrowDown')
      await settle()
    }
    expect(simple()).toHaveTextContent('Jogo 021') // f = 20
    await waitFor(() => expect(offsets()).toEqual([0, 25]))
    // page 2 is held: M = 24, the next row is not read yet, so ↓ does not move the focus
    key('ArrowDown')
    await settle()
    expect(simple()).toHaveTextContent('Jogo 021')
    expect(offsets()).toEqual([0, 25])
    for (const m of modelLog) expect(m.stripStart + m.games.length, `janela ${m.stripStart}+${m.games.length}`).toBeLessThanOrEqual(25)
    openPage2()
    await settle()
    key('ArrowDown')
    await waitFor(() => expect(simple()).toHaveTextContent('Jogo 026')) // f = 25
    expect(offsets()).toEqual([0, 25]) // 45 needed at row 5... L = 50 is enough
    key('ArrowDown')
    await settle()
    expect(simple()).toHaveTextContent('Jogo 031') // f = 30 needs 55 > 50
    await waitFor(() => expect(offsets()).toEqual([0, 25, 50]))
    await settle()
    expect(offsets()).toEqual([0, 25, 50])
    expect(limits()).toEqual([25, 25, 25])
    expect(lib.inflight.max).toBe(1)
    for (const m of modelLog) expect(m.games.length).toBeLessThanOrEqual(10) // (R + 1) × C
  }, 15000)

  it('BBX-050/num tema de faixa as páginas continuam V + 3 (15 com V 12)', async () => {
    bigLibrary(60)
    setup()
    await ready(13)
    expect(limits()).toEqual([15])
  })

  it('BBX-055/o shell envia o sort do tema no ListGames: title no Faixa e lastPlayed num tema que o declara', async () => {
    bigLibrary(30)
    const a = setup()
    await ready()
    expect(app.ListGames.mock.calls.length).toBeGreaterThan(0)
    for (const c of app.ListGames.mock.calls) expect((c[0] as { sort: string }).sort).toBe('title')
    a.unmount()

    app.ListGames.mockClear()
    useFake('roda', { sort: 'lastPlayed' })
    setup({ themeId: 'roda' })
    await readySimple()
    expect(app.ListGames.mock.calls.length).toBeGreaterThan(0)
    for (const c of app.ListGames.mock.calls) expect((c[0] as { sort: string }).sort).toBe('lastPlayed')
    expect(modelLog.at(-1)?.sortLabel).toBe('Jogado recentemente')
  })
})

describe('BigBox v2: fundo e vídeo por tema (BBX-052, BBX-053)', () => {
  it('BBX-052/o shell passa o fundo do tema: plain não desenha a arte; cover desenha em object-cover', async () => {
    app.MediaList.mockResolvedValue({ media: [SHOT] })
    useFake('roda', { backdrop: 'plain' })
    const a = setup({ themeId: 'roda' })
    await readySimple()
    await wait(50)
    expect(a.container.querySelector('[data-testid="bb-art"]')).toBeNull()
    expect(a.container.querySelector('img')).toBeNull()
    a.unmount()

    useFake('roda', { backdrop: 'cover' })
    const b = setup({ themeId: 'roda' })
    await readySimple()
    const art = await screen.findByTestId('bb-art')
    expect(art.className).toContain('object-cover')
    expect(b.container.querySelector('[data-testid="bb-art-fill"]')).toBeNull()
  })

  it('BBX-052/contain (Faixa) segue desenhando a arte inteira sobre o preenchimento', async () => {
    app.MediaList.mockResolvedValue({ media: [SHOT] })
    const { container } = setup()
    await ready()
    const art = await screen.findByTestId('bb-art')
    expect(art.className).toContain('object-contain')
    expect(container.querySelector('[data-testid="bb-art-fill"]')).not.toBeNull()
  })

  it('BBX-053/com videoSlot cover o fundo nunca toca vídeo, mesmo depois de 2.000 ms', async () => {
    app.MediaList.mockResolvedValue({ media: [VIDEO] })
    useFake('mosaico', { videoSlot: 'cover', backdrop: 'plain' })
    const { container } = setup({ themeId: 'mosaico' })
    await readySimple()
    await wait(2400)
    expect(container.querySelector('video')).toBeNull()
  }, 10000)

  it('BBX-053/FocusVideo toca dentro da capa em foco depois de 2.000 ms, um único video, e Y liga e desliga o som', async () => {
    app.MediaList.mockResolvedValue({ media: [VIDEO] })
    useFake('mosaico', { videoSlot: 'cover', backdrop: 'plain', Component: VideoTheme })
    const { container } = setup({ themeId: 'mosaico' })
    await readySimple()
    expect(container.querySelectorAll('video')).toHaveLength(0) // not before the 2.000 ms
    const video = (await screen.findByTestId('bb-video', undefined, { timeout: 5000 })) as HTMLVideoElement
    expect(container.querySelectorAll('video')).toHaveLength(1)
    expect(screen.getByTestId('card').contains(video)).toBe(true)
    expect(video.getAttribute('src')).toBe('/m/v.mp4')
    expect(video.muted).toBe(false) // sound starts on (1.0.3)
    key('m')
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(true))
    key('m')
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(false))
  }, 15000)

  it('BBX-053/trocar de foco remove o video e o src, e o novo só toca depois de 2.000 ms parado', async () => {
    app.MediaList.mockResolvedValue({ media: [VIDEO] })
    useFake('mosaico', { videoSlot: 'cover', backdrop: 'plain', Component: VideoTheme })
    const { container } = setup({ themeId: 'mosaico' })
    await readySimple()
    await screen.findByTestId('bb-video', undefined, { timeout: 5000 })
    key('ArrowRight')
    await waitFor(() => expect(simple()).toHaveTextContent('Hades'))
    expect(container.querySelectorAll('video')).toHaveLength(0)
    expect(container.innerHTML).not.toContain('<video')
    await screen.findByTestId('bb-video', undefined, { timeout: 5000 })
    expect(container.querySelectorAll('video')).toHaveLength(1)
  }, 15000)

  it('BBX-053/evento error: o vídeo some e não volta enquanto o foco não sair do jogo', async () => {
    app.MediaList.mockResolvedValue({ media: [VIDEO] })
    useFake('mosaico', { videoSlot: 'cover', backdrop: 'plain', Component: VideoTheme })
    const { container } = setup({ themeId: 'mosaico' })
    await readySimple()
    const video = await screen.findByTestId('bb-video', undefined, { timeout: 5000 })
    fireEvent.error(video)
    await waitFor(() => expect(container.querySelectorAll('video')).toHaveLength(0))
    await wait(2400)
    expect(container.querySelectorAll('video')).toHaveLength(0)
    expect(screen.queryByText('Som')).toBeNull() // videoVisible is false after the failure
  }, 15000)

  it('BBX-053/com videoSlot background o vídeo do fundo vale como em BBX-011 (um video, depois de 2.000 ms)', async () => {
    app.MediaList.mockResolvedValue({ media: [VIDEO] })
    useFake('roda', { videoSlot: 'background', backdrop: 'contain' })
    const { container } = setup({ themeId: 'roda' })
    await readySimple()
    expect(container.querySelectorAll('video')).toHaveLength(0)
    await screen.findByTestId('bb-video', undefined, { timeout: 5000 })
    expect(container.querySelectorAll('video')).toHaveLength(1)
  }, 15000)
})

describe('BigBox v2: Detalhes próprios do tema (BBX-056)', () => {
  function FakeDetails(p: BigBoxDetailsProps) {
    const mark = (f: string) => (p.detailsFocus === f ? 'true' : 'false')
    return (
      <div
        data-testid="fake-details"
        data-detailsfocus={p.detailsFocus}
        data-gallery={p.gallery}
        data-scroll={p.scrollSignal}
        data-ttb={p.timeToBeat.map((r) => r.label).join('|')}
        data-ttbsource={p.timeToBeatSource ?? ''}
        data-title={p.game.title}
      >
        <button data-focused={mark('play')} onClick={p.onPlay}>Iniciar</button>
        <button data-focused={mark('playUntracked')} onClick={p.onPlayUntracked}>Sem tempo</button>
        <div data-testid="fake-gallery" data-focused={mark('gallery')} />
      </div>
    )
  }
  const marks = () => document.querySelectorAll('[data-testid="fake-details"] [data-focused="true"]')

  it('BBX-056/o tema com Details usa o componente com as mesmas props, as mesmas transições de foco, uma única marca e as mesmas dicas', async () => {
    app.MediaList.mockResolvedValue(SHOTS)
    useFake('mosaico', { Details: FakeDetails })
    setup({ themeId: 'mosaico' })
    await readySimple()
    key('d')
    const d = await screen.findByTestId('fake-details')
    expect(d).toHaveAttribute('data-title', 'Celeste')
    expect(d).toHaveAttribute('data-ttb', 'História principal|História + extras|100%')
    expect(screen.getByRole('button', { name: 'Iniciar' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText('Imagens')).toBeInTheDocument()) // hint ↓ appears with a gallery (BBX-015)
    expect(screen.getByText('Voltar')).toBeInTheDocument()
    expect(screen.getByText('Fechar o jogo')).toBeInTheDocument()
    expect(d).toHaveAttribute('data-detailsfocus', 'play')
    expect(marks()).toHaveLength(1)

    key('ArrowRight') // BBX-033
    expect(d).toHaveAttribute('data-detailsfocus', 'playUntracked')
    expect(marks()).toHaveLength(1)
    key('ArrowLeft')
    expect(d).toHaveAttribute('data-detailsfocus', 'play')
    key('ArrowDown') // BBX-032
    expect(d).toHaveAttribute('data-detailsfocus', 'gallery')
    expect(marks()).toHaveLength(1)
    expect(screen.getByText('Imagem')).toBeInTheDocument() // gallery hints (←/→ Imagem)
    key('ArrowRight') // BBX-034
    expect(d).toHaveAttribute('data-gallery', '1')
    key('ArrowUp')
    expect(d).toHaveAttribute('data-detailsfocus', 'play')
    expect(marks()).toHaveLength(1)
    key('PageDown') // BBX-036: scroll signal, the tab does not change
    expect(Number(d.getAttribute('data-scroll'))).not.toBe(0)
    expect(app.ListGames.mock.calls.every((c) => (c[0] as { filter: { platformIds?: number[] } }).filter.platformIds === undefined)).toBe(true)
  })

  it('BBX-056/A no botão sem contar tempo do Details próprio lança sem contar e B volta a Navegar', async () => {
    app.MediaList.mockResolvedValue(SHOTS)
    useFake('mosaico', { Details: FakeDetails })
    const { onExit } = setup({ themeId: 'mosaico' })
    await readySimple()
    key('d')
    await screen.findByTestId('fake-details')
    key('ArrowRight')
    key('Enter')
    await waitFor(() => expect(app.LaunchGameUntracked).toHaveBeenCalledWith(30))
    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(onExit).not.toHaveBeenCalled()
  })

  it('BBX-056/sem Details no tema vale BigBoxDetails (data-bb-details)', async () => {
    useFake('mosaico', { Details: undefined })
    const { container } = setup({ themeId: 'mosaico' })
    await readySimple()
    key('d')
    await screen.findByTestId('simple-details')
    await waitFor(() => expect(container.querySelector('[data-bb-details]')).not.toBeNull())
    expect(screen.queryByTestId('fake-details')).toBeNull()
  })
})

describe('BigBox v2: tempo para zerar (BBX-059, TTB-013, TTB-016, TTB-017, TTB-039)', () => {
  const view = (o: Record<string, unknown> = {}) => ({
    gameId: 30, state: 'found', source: 'hltb', externalId: '1', externalTitle: 'Celeste', mainSeconds: 72000, mainExtraSeconds: 113400,
    completionistSeconds: 147600, fetchedAt: '2026-11-20T10:00:00Z', confirmed: false, stale: false, pending: false, unavailable: false,
    candidates: [], ...o,
  })

  it('TTB-016/os Detalhes do Big Box mostram "Tempo para zerar" com os três rótulos, valores e a fonte; pedem fetch true uma vez ao abrir; sem botão de atualizar ou confirmar', async () => {
    app.TimeToBeatGet.mockImplementation((id: number) => Promise.resolve(view({ gameId: id })))
    setup()
    await ready()
    expect(fetchTrue()).toHaveLength(0) // Faixa: Navegar só lê o cache
    key('d')
    expect(await screen.findByText('Tempo para zerar')).toBeInTheDocument()
    for (const s of ['História principal', '20 h', 'História + extras', '31½ h', '100%', '41 h']) {
      expect(await screen.findByText(s)).toBeInTheDocument()
    }
    expect(screen.getByText('20 h').closest('[data-bb-panel]')).not.toBeNull()
    expect(await screen.findByText('Fonte: HowLongToBeat')).toBeInTheDocument()
    await waitFor(() => expect(fetchTrue()).toHaveLength(1))
    expect(fetchTrue()[0]).toEqual([30, true])
    await flush()
    expect(fetchTrue()).toHaveLength(1)
    expect(screen.queryByRole('button', { name: /atualizar|confirmar|escolher|buscar/i })).toBeNull()
  })

  it('TTB-039/fonte IGDB: "Fonte: IGDB" nos Detalhes do Big Box', async () => {
    app.TimeToBeatGet.mockImplementation((id: number) => Promise.resolve(view({ gameId: id, source: 'igdb' })))
    setup()
    await ready()
    key('d')
    expect(await screen.findByText('Fonte: IGDB')).toBeInTheDocument()
    expect(screen.queryByText(/HowLongToBeat/)).toBeNull()
  })

  it('TTB-013/com o binding que falha os três rótulos ficam com "—", sem role alert, sem fonte e sem mudar o resto da tela', async () => {
    app.TimeToBeatGet.mockRejectedValue('agente desconectado')
    setup()
    await ready()
    key('d')
    expect(await screen.findByText('Tempo para zerar')).toBeInTheDocument()
    await flush()
    expect(screen.getAllByText('—')).toHaveLength(3)
    expect(screen.getByText('História principal')).toBeInTheDocument()
    expect(screen.queryByText(/Fonte:/)).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByRole('button', { name: 'Jogar sem contar tempo' })).toBeInTheDocument()
  })

  it('TTB-013/sem dado (not-found) os três rótulos ficam com "—"', async () => {
    app.TimeToBeatGet.mockImplementation((id: number) =>
      Promise.resolve(view({ gameId: id, state: 'not-found', source: '', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })),
    )
    setup()
    await ready()
    key('d')
    await screen.findByText('Tempo para zerar')
    await flush()
    expect(screen.getAllByText('—')).toHaveLength(3)
    expect(screen.queryByText(/Fonte:/)).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('TTB-017/timeToBeat:changed do jogo mostrado relê uma vez (fetch false) nos Detalhes; o aviso de outro jogo não relê', async () => {
    app.TimeToBeatGet.mockImplementation((id: number) => Promise.resolve(view({ gameId: id })))
    setup()
    await ready()
    key('d')
    await screen.findByText('20 h')
    await flush()
    const before = fetchFalse(30).length
    expect(before).toBeGreaterThan(0)

    app.TimeToBeatGet.mockImplementation((id: number) => Promise.resolve(view({ gameId: id, mainSeconds: 3600 })))
    emit('timeToBeat:changed', { gameId: 20 })
    await flush()
    expect(fetchFalse(30)).toHaveLength(before)

    emit('timeToBeat:changed', { gameId: 30 })
    await waitFor(() => expect(fetchFalse(30)).toHaveLength(before + 1))
    expect(await screen.findByText('1 h')).toBeInTheDocument()
    await flush()
    expect(fetchFalse(30)).toHaveLength(before + 1)
  })

  it('BBX-059/num tema sem timeToBeatInBrowse (Faixa) Navegar nunca pede fetch true, mesmo parado por mais de 1.000 ms', async () => {
    setup()
    await ready()
    await wait(1300)
    expect(ttbCalls().length).toBeGreaterThan(0) // the cache is read
    expect(fetchTrue()).toHaveLength(0)
  }, 10000)

  it('BBX-059/com timeToBeatInBrowse o fetch true sai depois de 1.000 ms com o mesmo jogo em foco, uma vez', async () => {
    useFake('mosaico', { timeToBeatInBrowse: true })
    setup({ themeId: 'mosaico' })
    await readySimple()
    const t0 = Date.now()
    expect(fetchTrue()).toHaveLength(0)
    await waitFor(() => expect(fetchFalse(30).length).toBeGreaterThan(0)) // cache read on focus
    await waitFor(() => expect(fetchTrue()).toHaveLength(1), { timeout: 4000 })
    expect(Date.now() - t0).toBeGreaterThanOrEqual(800)
    expect(fetchTrue()[0]).toEqual([30, true])
    await wait(300)
    expect(fetchTrue()).toHaveLength(1)
  }, 10000)

  it('BBX-059/trocar de jogo antes de 1.000 ms cancela o pedido do anterior e conta de novo para o novo', async () => {
    useFake('mosaico', { timeToBeatInBrowse: true })
    setup({ themeId: 'mosaico' })
    await readySimple()
    await wait(500)
    key('ArrowRight')
    await waitFor(() => expect(simple()).toHaveTextContent('Hades'))
    const t1 = Date.now()
    await waitFor(() => expect(fetchTrue().length).toBeGreaterThan(0), { timeout: 4000 })
    expect(Date.now() - t1).toBeGreaterThanOrEqual(800)
    await wait(300)
    expect(fetchTrue().map((c) => c[0])).toEqual([20])
  }, 10000)

  it('BBX-059/durante a suspensão (depois de lançar) nada é pedido, nem com o jogo parado por mais de 1.000 ms', async () => {
    useFake('mosaico', { timeToBeatInBrowse: true })
    setup({ themeId: 'mosaico' })
    await readySimple()
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    await wait(1400)
    expect(fetchTrue()).toHaveLength(0)
  }, 10000)

  it('BBX-059/abrir os Detalhes num tema com timeToBeatInBrowse pede na hora (sem esperar 1.000 ms)', async () => {
    useFake('mosaico', { timeToBeatInBrowse: true })
    setup({ themeId: 'mosaico' })
    await readySimple()
    key('d')
    await screen.findByTestId('simple-details')
    await waitFor(() => expect(fetchTrue()).toHaveLength(1), { timeout: 700 })
    expect(fetchTrue()[0]).toEqual([30, true])
  })

  it('BBX-059/o Big Box só lê o tempo para zerar: nenhum binding de escrita é chamado', async () => {
    app.TimeToBeatGet.mockImplementation((id: number) => Promise.resolve(view({ gameId: id })))
    setup()
    await ready()
    key('d')
    await screen.findByText('Tempo para zerar')
    await flush()
    for (const n of called) expect(n).not.toMatch(/^TimeToBeat(Refresh|Search|Choose|SetSettings)$/)
  })
})

// ---------------------------------------------------------------------------------------------------------
// Fase 17 (T07): shell com layout `columns`, rolagem pelas bordas, Detalhes em Navegar e onTab, com um tema falso
// registrado sobre a entrada 'roda' (restaurada em uninstall()). Marcação do tema falso: data-testid="cols" com
// data-{zone,screen,tab,focus,start,columns,view,layout,sortlabel}, abas role="tab", cartões data-testid="col-card".
// O Details falso fica em data-testid="col-details-node" com data-{active,game,icon,preview-*,has-labels}.
// ---------------------------------------------------------------------------------------------------------

function ColumnsTheme(p: BigBoxThemeProps) {
  const { model, details, onTab, onPick } = p
  modelLog.push(model)
  propKeys.push(Object.keys(p).sort())
  return (
    <div
      data-testid="cols"
      data-zone={model.zone}
      data-screen={model.screen}
      data-tab={model.tab}
      data-focus={model.focus}
      data-start={model.stripStart}
      data-columns={model.columns}
      data-view={model.view}
      data-layout={model.layout}
      data-sortlabel={model.sortLabel}
    >
      <div role="tablist">
        {model.tabs.map((t, i) => (
          <button key={t.key} role="tab" aria-selected={i === model.tab} onClick={() => onTab?.(i)}>
            {t.label}
          </button>
        ))}
      </div>
      <ol>
        {model.games.map((gm, i) => (
          <li key={gm.id} data-testid="col-card" data-index={model.stripStart + i} onClick={() => onPick(model.stripStart + i)}>
            {gm.title}
          </li>
        ))}
      </ol>
      <div data-testid="col-details">{details}</div>
    </div>
  )
}

function ColumnsDetails(p: BigBoxDetailsProps) {
  return (
    <div
      data-testid="col-details-node"
      data-active={String(p.active)}
      data-game={p.game.id}
      data-icon={p.platformIconUrl ?? ''}
      data-preview-game={p.preview?.gameId ?? ''}
      data-preview-url={p.preview?.videoUrl ?? ''}
      data-preview-active={String(p.preview?.active)}
      data-has-labels={String(p.labels !== undefined)}
    >
      <button onClick={p.onPlay}>Iniciar</button>
    </div>
  )
}

function useCols(patch: Partial<BigBoxThemeDef> = {}) {
  useFake('roda', {
    axis: 'grid', columns: 1, layout: 'columns', gridColumns: () => 5, gridRows: () => 1, maxNodes: 200, gridScroll: 'edges',
    sort: 'inherit', videoSlot: 'panel', tabs: 'platforms', backdrop: 'plain', timeToBeatInBrowse: true, listCapacity: () => 5,
    Component: ColumnsTheme, Details: ColumnsDetails, ...patch,
  })
}
const colCards = () => screen.queryAllByTestId('col-card')
const colTitles = () => colCards().map((c) => c.textContent)
const colsRoot = () => screen.getByTestId('cols')
const readyCols = () => waitFor(() => expect(colCards().length).toBeGreaterThan(0))
const colDetails = () => screen.findByTestId('col-details-node')
const tabButton = (name: string) => screen.getByRole('tab', { name })
const downTimes = async (n: number) => {
  for (let i = 0; i < n; i++) {
    key('ArrowDown')
    await settle()
  }
}

describe('BigBox fase 17: capacidade e páginas (BBX-061, BBX-062)', () => {
  it('BBX-061/tema sem os campos novos usa colunas, fileiras e 30 nós como hoje', async () => {
    bigLibrary(100)
    useFake('roda', { axis: 'grid', columns: 5, listCapacity: () => 5, gridRows: () => 9, Component: ColumnsTheme })
    setup({ themeId: 'roda' })
    await readyCols()
    // R' = min(9, ⌊30 / 5⌋ − 1) = 5: pages of (5 + 1 + 3) × 5 = 45 and at most 30 cards
    expect(limits()[0]).toBe(45)
    expect(colCards().length).toBeLessThanOrEqual(30)
    expect(colsRoot()).toHaveAttribute('data-layout', 'screens')
    expect(colsRoot()).toHaveAttribute('data-zone', 'games')
    expect(colsRoot()).toHaveAttribute('data-columns', '5')
  })

  it('BBX-062/o limit do ListGames é gridPageSize(C, R\', N) do tema falso (N = 200)', async () => {
    bigLibrary(100)
    useCols({ gridColumns: () => 6, gridRows: () => 2 })
    setup({ themeId: 'roda' })
    await readyCols()
    expect(limits()[0]).toBe(gridPageSize(6, 2, 200))
    expect(limits()[0]).toBe(36) // (2 + 1 + 3) × 6
  })

  it('BBX-062/nunca mais de (R\'+1)·C cartões no DOM', async () => {
    bigLibrary(120)
    useCols({ gridColumns: () => 6, gridRows: () => 2 })
    setup({ themeId: 'roda' })
    await readyCols()
    await downTimes(6)
    expect(colCards().length).toBeLessThanOrEqual(18)
    for (const m of modelLog) expect(m.games.length).toBeLessThanOrEqual(18)
  })
})

describe('BigBox fase 17: rolagem pelas bordas (BBX-063)', () => {
  it('BBX-063/andar para baixo só rola ao passar da última fileira inteira', async () => {
    bigLibrary(120)
    useCols({ gridColumns: () => 6, gridRows: () => 2 })
    setup({ themeId: 'roda' })
    await readyCols()
    expect(colTitles()[0]).toBe('Jogo 001')
    await downTimes(1) // row 1, still whole on screen
    expect(colsRoot()).toHaveAttribute('data-focus', '6')
    expect(colTitles()[0]).toBe('Jogo 001')
    await downTimes(1) // row 2 passes the last whole row: one row scrolls
    expect(colsRoot()).toHaveAttribute('data-focus', '12')
    expect(colTitles()[0]).toBe('Jogo 007')
    key('ArrowUp') // row 1 is still inside [1, 2]
    expect(colTitles()[0]).toBe('Jogo 007')
    key('ArrowUp') // row 0 is above the first whole row
    expect(colTitles()[0]).toBe('Jogo 001')
  })

  it('BBX-063/na entrada e na troca de aba a fileira do foco fica no meio', async () => {
    bigLibrary(120)
    useCols({ gridColumns: () => 6, gridRows: () => 3 })
    setup({ themeId: 'roda', initialGameId: 31 }) // index 30, row 5 (inside the first page of 42)
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-focus', '30'))
    expect(colTitles()[0]).toBe('Jogo 025') // centeredRow(5, 3, 20) = 4
    fireEvent.click(tabButton('Windows'))
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-tab', '1'))
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-focus', '0'))
    expect(colTitles()[0]).toBe('Jogo 001')
  })

  it('BBX-063/depois de game:closedByHotkey a fileira do foco fica no meio', async () => {
    bigLibrary(120)
    useCols({ gridColumns: () => 6, gridRows: () => 3 })
    setup({ themeId: 'roda' })
    await readyCols()
    await downTimes(4) // row 4: edges scroll to start row 2
    expect(colsRoot()).toHaveAttribute('data-focus', '24')
    expect(colTitles()[0]).toBe('Jogo 013')
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(25))
    await flush()

    emit('game:closedByHotkey', { gameId: 25 })

    await waitFor(() => expect(app.BringToFront).toHaveBeenCalled())
    await waitFor(() => expect(colTitles()[0]).toBe('Jogo 019')) // centeredRow(4, 3, 20) = 3
    expect(colsRoot()).toHaveAttribute('data-focus', '24')
  })

  it('BBX-063/clicar num cartão (onPick) não recentra', async () => {
    bigLibrary(120)
    useCols({ gridColumns: () => 6, gridRows: () => 3 })
    setup({ themeId: 'roda' })
    await readyCols()
    await downTimes(4)
    expect(colTitles()[0]).toBe('Jogo 013')

    fireEvent.click(screen.getByText('Jogo 015')) // row 2, inside the whole rows [2, 4]

    expect(colsRoot()).toHaveAttribute('data-focus', '14')
    expect(colTitles()[0]).toBe('Jogo 013') // centering would give 'Jogo 007'
  })
})

describe('BigBox fase 17: Detalhes em Navegar (BBX-064, TDK-010, TDK-011)', () => {
  it('BBX-064/layout columns monta o Details também em Navegar, com active=false', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    const d = await colDetails()
    expect(colsRoot()).toHaveAttribute('data-screen', 'browse')
    expect(d).toHaveAttribute('data-active', 'false')
    expect(d).toHaveAttribute('data-game', '30')
    expect(d).toHaveAttribute('data-has-labels', 'true')
    expect(d.getAttribute('data-icon')).toBeTruthy() // the tab of the platform of the game (p:1)
    key('ArrowRight')
    await waitFor(() => expect(screen.getByTestId('col-details-node')).toHaveAttribute('data-game', '20'))
    expect(screen.getByTestId('col-details-node')).toHaveAttribute('data-active', 'false')
  })

  it('BBX-064/em Detalhes active=true', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    await colDetails()
    key('d')
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'details'))
    expect(screen.getByTestId('col-details-node')).toHaveAttribute('data-active', 'true')
    key('Escape')
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'browse'))
    expect(screen.getByTestId('col-details-node')).toHaveAttribute('data-active', 'false')
  })

  it('BBX-064/mesmos bindings chamados que num tema screens (nenhuma leitura nova)', async () => {
    useFake('mosaico', { timeToBeatInBrowse: true })
    const a = setup({ themeId: 'mosaico' })
    await readySimple()
    key('d')
    await screen.findByTestId('simple-details')
    await flush()
    const screensCalls = new Set(called)
    a.unmount()
    called.splice(0)

    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    await colDetails()
    key('d')
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'details'))
    await flush()

    expect(screensCalls.size).toBeGreaterThan(0)
    expect(new Set(called)).toEqual(screensCalls)
  })

  it('TDK-010/videoSlot panel: o fundo nunca toca vídeo; preview.active é true em Navegar e false em Detalhes', async () => {
    app.MediaList.mockResolvedValue({ media: [SHOT, VIDEO] })
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    await colDetails()
    await waitFor(() => expect(screen.getByTestId('col-details-node')).toHaveAttribute('data-preview-url', '/m/v.mp4'))
    const d = () => screen.getByTestId('col-details-node')
    expect(d()).toHaveAttribute('data-preview-game', '30')
    expect(d()).toHaveAttribute('data-preview-active', 'true')
    await wait(2100) // longer than the 2.000 ms of the background video
    expect(screen.queryByTestId('bb-video')).toBeNull()

    key('d')
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'details'))
    expect(d()).toHaveAttribute('data-preview-active', 'false')
    expect(screen.queryByTestId('bb-video')).toBeNull()
  }, 10000)

  it('TDK-011/tempo para zerar: cache lido na hora, busca depois de 1000 ms em Navegar (timeToBeatInBrowse)', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    const t0 = Date.now()
    await waitFor(() => expect(fetchFalse(30).length).toBeGreaterThan(0))
    expect(fetchTrue()).toHaveLength(0)
    await waitFor(() => expect(fetchTrue()).toHaveLength(1), { timeout: 4000 })
    expect(Date.now() - t0).toBeGreaterThanOrEqual(800)
    expect(fetchTrue()[0]).toEqual([30, true])
  }, 10000)

  it('TDK-011/tempo para zerar: na hora ao entrar em Detalhes', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    await colDetails()
    key('d')
    await waitFor(() => expect(fetchTrue()).toHaveLength(1), { timeout: 700 })
    expect(fetchTrue()[0]).toEqual([30, true])
  })
})

describe('BigBox fase 17: onTab (BBX-065)', () => {
  it('BBX-065/onTab(i) ativa a aba, foco no primeiro jogo e lê a lista da aba nova', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    key('ArrowRight') // Hades
    expect(colsRoot()).toHaveAttribute('data-focus', '1')

    fireEvent.click(tabButton('PlayStation 2'))

    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-tab', '2'))
    await waitFor(() => expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ filter: { version: 1, platformIds: [2] } })))
    await waitFor(() => expect(colTitles()).toEqual(['Gran Turismo']))
    expect(colsRoot()).toHaveAttribute('data-focus', '0')
  })

  it('BBX-065/onTab da aba ativa não faz nada', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    key('ArrowRight')
    await flush()
    const before = app.ListGames.mock.calls.length

    fireEvent.click(tabButton('Todos'))
    await flush()

    expect(colsRoot()).toHaveAttribute('data-tab', '0')
    expect(colsRoot()).toHaveAttribute('data-focus', '1')
    expect(app.ListGames.mock.calls.length).toBe(before)
  })

  it('BBX-065/onTab durante a suspensão é ignorado', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    await flush()

    fireEvent.click(tabButton('Windows'))
    await flush()

    expect(colsRoot()).toHaveAttribute('data-tab', '0')
    expect(app.ListGames.mock.calls.some((c) => (c[0] as { filter: { platformIds?: number[] } }).filter.platformIds)).toBe(false)
  })

  it('BBX-065/onTab a partir de Detalhes volta a Navegar', async () => {
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    key('d')
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'details'))

    fireEvent.click(tabButton('PlayStation 2'))

    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'browse'))
    expect(colsRoot()).toHaveAttribute('data-tab', '2')
    expect(colsRoot()).toHaveAttribute('data-focus', '0')
  })
})

describe('BigBox fase 17: ordem e visualização herdadas (BBX-067, BBX-068)', () => {
  it('BBX-067/sort inherit envia o librarySort ao ListGames (totalPlayed) e o rótulo do modelo é o do seletor', async () => {
    useCols()
    setup({ themeId: 'roda', librarySort: 'totalPlayed' })
    await readyCols()
    expect(app.ListGames.mock.calls.length).toBeGreaterThan(0)
    for (const c of app.ListGames.mock.calls) expect((c[0] as { sort: string }).sort).toBe('totalPlayed')
    expect(colsRoot()).toHaveAttribute('data-sortlabel', 'Tempo jogado')
  })

  it('BBX-067/um tema com sort próprio ignora o librarySort', async () => {
    useCols({ sort: 'lastPlayed' })
    setup({ themeId: 'roda', librarySort: 'year' })
    await readyCols()
    for (const c of app.ListGames.mock.calls) expect((c[0] as { sort: string }).sort).toBe('lastPlayed')
  })

  it('BBX-068/libraryView list dá C=1 em gridColumns e gridRows', async () => {
    bigLibrary(30)
    const cols = vi.fn((_vp: unknown, view: LibraryView) => (view === 'list' ? 1 : 6))
    const rows = vi.fn((_vp: unknown, view?: LibraryView) => (view === 'list' ? 5 : 2))
    useCols({ gridColumns: cols, gridRows: rows })
    setup({ themeId: 'roda', libraryView: 'list' })
    await readyCols()
    expect(cols).toHaveBeenCalledWith({ width: 1920, height: 1080 }, 'list')
    expect(rows).toHaveBeenCalledWith({ width: 1920, height: 1080 }, 'list')
    expect(colsRoot()).toHaveAttribute('data-columns', '1')
    expect(colsRoot()).toHaveAttribute('data-view', 'list')
    expect(limits()[0]).toBe(gridPageSize(1, 5, 200)) // (5 + 1 + 3) × 1 = 9
    key('ArrowDown')
    expect(colsRoot()).toHaveAttribute('data-focus', '1') // one game per row
  })

  it('BBX-068/a visualização não é gravada pelo Big Box (localStorage intacto)', async () => {
    const before = JSON.stringify(Object.entries(localStorage).sort())
    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    try {
      useCols()
      setup({ themeId: 'roda', libraryView: 'list' })
      await readyCols()
      for (const k of ['ArrowDown', 'ArrowUp', 'd', 'Escape', 'PageDown']) key(k)
      await flush()
      expect(setItem).not.toHaveBeenCalled()
      expect(JSON.stringify(Object.entries(localStorage).sort())).toBe(before)
    } finally {
      setItem.mockRestore()
    }
  })
})

describe('BigBox fase 17: leitura por aba e entrada com controle (TDK-020, TDK-033)', () => {
  beforeEach(installPad)
  afterEach(uninstallPad)
  const SELECT = 8

  it('TDK-020/uma leitura em voo por aba, sem repetir nem cancelar ao mover o foco; troca de aba cancela; ↓ com a página seguinte pendente não passa de M', async () => {
    const lib = bigLibrary(60)
    const openPage2 = gate(lib, 25)
    useCols({ gridColumns: () => 5, gridRows: () => 1 })
    setup({ themeId: 'roda' })
    await readyCols()
    expect(offsets()).toEqual([0])
    expect(limits()).toEqual([25])
    await downTimes(4) // f = 20, the last loaded row (M = 24)
    await waitFor(() => expect(offsets()).toEqual([0, 25]))
    await downTimes(3)
    expect(colsRoot()).toHaveAttribute('data-focus', '20') // does not pass M while page 2 is pending
    expect(offsets()).toEqual([0, 25]) // no repeated read for the same tab
    expect(lib.inflight.max).toBe(1)

    fireEvent.click(tabButton('Windows')) // changing tab drops the pending read of the old one
    await waitFor(() => expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ filter: { version: 1, platformIds: [1] } })))
    openPage2()
    await settle()
    expect(colsRoot()).toHaveAttribute('data-tab', '1')
    expect(colsRoot()).toHaveAttribute('data-focus', '0')
    expect(offsets().filter((o) => o === 25)).toHaveLength(1)
    const windowsPages = app.ListGames.mock.calls.filter((c) => (c[0] as { filter: { platformIds?: number[] } }).filter.platformIds)
    expect(windowsPages.map((c) => (c[0] as { offset: number }).offset)).toEqual([0])
  }, 15000)

  it('TDK-033/entrada suspensa com o jogo lançado', async () => {
    bigLibrary(40)
    useCols()
    const { onExit } = setup({ themeId: 'roda' })
    await readyCols()
    frame()
    tap(PAD.RIGHT)
    tap(PAD.A)
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(2))
    await flush()

    tap(PAD.A)
    tap(PAD.RIGHT)
    tap(PAD.B)
    tap(PAD.DOWN)
    await flush()

    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(onExit).not.toHaveBeenCalled()
    expect(colsRoot()).toHaveAttribute('data-focus', '1')
    emit('session:changed', session({ gameId: 2, state: 'ended', endedAt: '2026-11-15T10:10:00.000Z' }))
    tap(PAD.RIGHT)
    expect(colsRoot()).toHaveAttribute('data-focus', '2')
  })

  it('TDK-033/Select segurado bloqueia A, B, Y e ↑/↓', async () => {
    bigLibrary(40)
    useCols()
    const { onExit } = setup({ themeId: 'roda' })
    await readyCols()
    frame()
    padPressed.add(SELECT)
    frame()

    tap(PAD.A)
    tap(PAD.B)
    tap(PAD.Y)
    tap(PAD.DOWN)
    await flush()

    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(onExit).not.toHaveBeenCalled()
    expect(colsRoot()).toHaveAttribute('data-focus', '0')
    padPressed.delete(SELECT)
    frame()
    tap(PAD.DOWN)
    expect(colsRoot()).toHaveAttribute('data-focus', '5')
  })

  it('TDK-033/depois de Select+B o foco volta ao jogo que rodava com zona games', async () => {
    bigLibrary(40)
    useCols()
    setup({ themeId: 'roda' })
    await readyCols()
    key('ArrowLeft') // column 0: the platforms column takes the selection
    expect(colsRoot()).toHaveAttribute('data-zone', 'platforms')
    key('d') // the panel opens from any zone
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'details'))
    key('Enter')
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(1))
    await flush()

    emit('game:closedByHotkey', { gameId: 1 })

    await waitFor(() => expect(app.BringToFront).toHaveBeenCalled())
    await waitFor(() => expect(colsRoot()).toHaveAttribute('data-screen', 'browse'))
    expect(colsRoot()).toHaveAttribute('data-zone', 'games')
    expect(colsRoot()).toHaveAttribute('data-focus', '0')
    key('ArrowRight')
    expect(colsRoot()).toHaveAttribute('data-focus', '1') // the entry is free again
  })
})
