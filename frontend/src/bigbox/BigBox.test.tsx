import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { stripWindow } from './viewModel'
import type { BigBoxViewModel } from './viewModel'
import { BIGBOX_THEMES } from './themes/registry'
import type { BigBoxThemeProps } from './themes/registry'
import { BigBox } from './BigBox'
import { useMediaEvents } from '../lib/metadata'

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
  playModes: [], launchType: 'exe', launchArgs: '', emulatorId: null,
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

type Props = { initialPlatformId?: number | null; initialGameId?: number | null; themeId?: 'faixa' | 'mosaico' | 'roda' }
function setup(p: Props = {}) {
  const onExit = vi.fn()
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const MediaEvents = ({ children }: { children: ReactNode }) => {
    useMediaEvents()
    return <>{children}</>
  }
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <MediaEvents>{children}</MediaEvents>
    </QueryClientProvider>
  )
  const r = render(
    <BigBox initialPlatformId={p.initialPlatformId ?? null} initialGameId={p.initialGameId ?? null} themeId={p.themeId ?? 'faixa'} onExit={onExit} />,
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
      expect(['ListGames', 'GetGame', 'Catalog', 'MediaList', 'CurrentSession', 'GameSessions']).toContain(n)
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
    expect((video as HTMLVideoElement).muted).toBe(true)
    expect(await screen.findByText('Som')).toBeInTheDocument()

    key('m')

    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(false))
    key('m')
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(true))
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
    Promise.resolve({ ...rows[id - 1], description: 'Descrição.', completedAt: null, developers: [], publishers: [], genres: [], playModes: [], launchType: 'exe', launchArgs: '', emulatorId: null }),
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
