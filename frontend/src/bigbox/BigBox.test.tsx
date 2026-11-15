import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { BigBoxViewModel } from './viewModel'
import { BIGBOX_THEMES } from './themes/registry'
import type { BigBoxThemeProps } from './themes/registry'
import { BigBox } from './BigBox'

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
}

function uninstall() {
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
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
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
    key('ArrowDown')
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
    setup({ themeId: 'roda' })
    await waitFor(() => expect(screen.getByTestId('vert')).toHaveTextContent('Celeste'))

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
