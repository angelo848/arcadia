import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query'
import App from './App'
import type { GameView, PickedExe, SessionView } from './lib/games'

// ---------------------------------------------------------------------------
// Fakes for window.go.main.App and window.runtime.EventsOn (as in lib/games.test.tsx)
// ---------------------------------------------------------------------------

type Fn = ReturnType<typeof vi.fn>
type FakeApp = {
  AddGame: Fn
  AgentStatus: Fn
  CurrentSession: Fn
  LaunchGame: Fn
  LaunchGameUntracked: Fn
  LibrarySummary: Fn
  ListGames: Fn
  PickGameExe: Fn
  PauseSession: Fn
  ResumeSession: Fn
}

let app: FakeApp
let handlers: Map<string, Set<(...data: unknown[]) => void>>

function status(connected: boolean) {
  return {
    connected,
    version: connected ? '0.3.0-test' : '',
    pid: connected ? 4242 : 0,
    startedAt: connected ? '2026-11-09T09:00:00Z' : '',
    dataDir: connected ? 'C:\\Users\\test\\AppData\\Local\\Arcadia' : '',
    uptimeSeconds: 0,
  }
}

function install() {
  handlers = new Map()
  app = {
    AddGame: vi.fn(() => Promise.resolve(game())),
    AgentStatus: vi.fn(() => Promise.resolve(status(true))),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    LaunchGame: vi.fn(() => Promise.resolve(session())),
    LibrarySummary: vi.fn(() => Promise.resolve({ games: 0, platforms: 0 })),
    ListGames: vi.fn(() => Promise.resolve([])),
    PickGameExe: vi.fn(() => Promise.resolve({ path: '', title: '' })),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    PauseSession: vi.fn(() => Promise.resolve(session({ state: 'paused' }))),
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

function game(over: Partial<GameView> = {}): GameView {
  return {
    id: 10,
    title: 'Hollow Knight',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    totalPlaySeconds: 0,
    playCount: 0,
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

function renderApp() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
  render(<App />, { wrapper: Wrapper })
  return qc
}

const panelName = 'Sessão em andamento'

function queryPanel(): HTMLElement | null {
  return screen.queryByRole('status', { name: panelName })
}

function indicatorState(): string | null {
  return document.querySelector('[data-state]')?.getAttribute('data-state') ?? null
}

async function waitConnected() {
  await waitFor(() => expect(indicatorState()).toBe('connected'))
}

// rowOf returns the row of the game in the list: the smallest ancestor of the
// title (outside the session panel) that contains a button.
function rowOf(title: string): HTMLElement {
  const candidates = screen
    .getAllByText(title)
    .filter((el) => !el.closest(`[aria-label="${panelName}"]`))
  expect(candidates, `one list title ${title}`).toHaveLength(1)
  let el: HTMLElement | null = candidates[0]
  while (el && within(el).queryAllByRole('button').length === 0) {
    el = el.parentElement
  }
  if (!el) {
    throw new Error(`no row with a button for ${title}`)
  }
  return el
}

async function findRow(title: string): Promise<HTMLElement> {
  await waitFor(() => expect(screen.getAllByRole('button', { name: /Jogar|Iniciando…/ }).length).toBeGreaterThan(0))
  let row!: HTMLElement
  await waitFor(() => {
    row = rowOf(title)
  })
  return row
}

function precedes(a: Node, b: Node): boolean {
  return (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
}

// Fake-timer helpers: Testing Library's waitFor is not used while timers are faked.
const fakeTimerApis = ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date', 'performance'] as const

async function tick(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

async function settle() {
  for (let i = 0; i < 10; i++) {
    await tick(10)
  }
}

beforeEach(() => {
  install()
})

afterEach(() => {
  vi.useRealTimers()
  uninstall()
})

// ---------------------------------------------------------------------------
// UI-010 empty state and loading
// ---------------------------------------------------------------------------

describe('App: estado vazio e carregamento', () => {
  it('UI-010 [] mostra Nenhum jogo ainda sem contadores, com Adicionar jogo', async () => {
    app.ListGames.mockResolvedValue([])
    renderApp()

    expect(await screen.findByText('Nenhum jogo ainda')).toBeInTheDocument()
    expect(screen.queryByText('Conectando ao agente…')).toBeNull()
    expect(screen.queryByText(/jogos ·|plataformas/)).toBeNull()
    expect(screen.getByRole('button', { name: 'Adicionar jogo' })).toBeEnabled()
    expect(screen.queryByRole('button', { name: 'Jogar' })).toBeNull()
    // The library summary is no longer used by the screen (decision 10).
    expect(app.LibrarySummary).not.toHaveBeenCalled()
  })

  it('UI-010 carregando mostra Conectando ao agente…', async () => {
    const pending = deferred<GameView[]>()
    app.ListGames.mockImplementation(() => pending.promise)
    renderApp()

    await waitFor(() => expect(app.ListGames).toHaveBeenCalled())
    expect(screen.getByText('Conectando ao agente…')).toBeInTheDocument()
    expect(screen.queryByText('Nenhum jogo ainda')).toBeNull()

    await act(async () => {
      pending.resolve([game()])
      await pending.promise
    })
    await findRow('Hollow Knight')
    expect(screen.queryByText('Conectando ao agente…')).toBeNull()
  })

  it('UI-010 erro mostra Conectando ao agente… e tenta de novo em 2 s', async () => {
    vi.useFakeTimers({ toFake: [...fakeTimerApis] })
    // Disconnected agent: no reconnect reload interferes with the call count.
    app.AgentStatus.mockResolvedValue(status(false))
    app.ListGames.mockRejectedValue('O agente não está respondendo. Tente de novo.')
    renderApp()
    await settle()

    expect(screen.getByText('Conectando ao agente…')).toBeInTheDocument()
    expect(screen.queryByText('Nenhum jogo ainda')).toBeNull()
    expect(app.ListGames).toHaveBeenCalledTimes(1)

    await tick(1800)
    expect(app.ListGames).toHaveBeenCalledTimes(1)
    await tick(400)
    expect(app.ListGames).toHaveBeenCalledTimes(2)
    expect(screen.getByText('Conectando ao agente…')).toBeInTheDocument()
    await tick(2000)
    expect(app.ListGames).toHaveBeenCalledTimes(3)

    // When the call starts working the list replaces the loading text.
    app.ListGames.mockResolvedValue([game()])
    await tick(2000)
    await settle()
    expect(screen.queryByText('Conectando ao agente…')).toBeNull()
    expect(screen.getByRole('button', { name: 'Jogar' })).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// UI-007 add game
// ---------------------------------------------------------------------------

describe('App: adicionar jogo', () => {
  const picked: PickedExe = { path: 'C:\\Games\\HK\\hollow_knight.exe', title: 'hollow_knight' }

  it('UI-007 escolher exe abre o formulário; salvar fecha e o jogo aparece', async () => {
    let library: GameView[] = []
    app.ListGames.mockImplementation(() => Promise.resolve(library))
    app.PickGameExe.mockResolvedValue(picked)
    const added = game({ id: 77, title: 'Hollow Knight', path: picked.path })
    app.AddGame.mockImplementation(() => {
      library = [added]
      return Promise.resolve(added)
    })
    renderApp()
    await screen.findByText('Nenhum jogo ainda')

    fireEvent.click(screen.getByRole('button', { name: 'Adicionar jogo' }))
    const input = (await screen.findByLabelText('Título')) as HTMLInputElement
    expect(app.PickGameExe).toHaveBeenCalledTimes(1)
    expect(input).toHaveValue('hollow_knight')
    expect(screen.queryByText(picked.path) ?? screen.queryByDisplayValue(picked.path)).not.toBeNull()

    fireEvent.change(input, { target: { value: 'Hollow Knight' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(screen.queryByLabelText('Título')).toBeNull())
    expect(app.AddGame).toHaveBeenCalledTimes(1)
    expect(app.AddGame).toHaveBeenCalledWith(picked.path, 'Hollow Knight')
    const row = await findRow('Hollow Knight')
    expect(within(row).getByRole('button', { name: 'Jogar' })).toBeEnabled()
    expect(screen.queryByText('Nenhum jogo ainda')).toBeNull()
  })

  it('UI-007 Adicionar jogo fica desabilitado enquanto Salvar está pendente', async () => {
    app.ListGames.mockResolvedValue([])
    app.PickGameExe.mockResolvedValue(picked)
    const pending = deferred<GameView>()
    app.AddGame.mockImplementation(() => pending.promise)
    renderApp()
    await screen.findByText('Nenhum jogo ainda')

    fireEvent.click(screen.getByRole('button', { name: 'Adicionar jogo' }))
    await screen.findByLabelText('Título')
    expect(screen.getByRole('button', { name: 'Adicionar jogo' })).toBeEnabled()

    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(app.AddGame).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Adicionar jogo' })).toBeDisabled())

    // A click while disabled does not open the dialog again.
    fireEvent.click(screen.getByRole('button', { name: 'Adicionar jogo' }))
    await act(async () => {
      await Promise.resolve()
    })
    expect(app.PickGameExe).toHaveBeenCalledTimes(1)

    await act(async () => {
      pending.resolve(game({ id: 77, path: picked.path }))
      await pending.promise
    })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Adicionar jogo' })).toBeEnabled())
  })

  it('UI-007 cancelar no diálogo não muda nada', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Celeste', totalPlaySeconds: 60 })])
    app.PickGameExe.mockResolvedValue({ path: '', title: '' })
    renderApp()
    await findRow('Celeste')
    await waitConnected()
    // Let the reconnect reload (if any) finish before counting.
    await waitFor(() => expect(screen.queryByText('Conectando ao agente…')).toBeNull())
    const listCalls = app.ListGames.mock.calls.length

    fireEvent.click(screen.getByRole('button', { name: 'Adicionar jogo' }))
    await waitFor(() => expect(app.PickGameExe).toHaveBeenCalledTimes(1))
    // Give a pending state update a chance to happen.
    await act(async () => {
      await Promise.resolve()
    })

    expect(screen.queryByLabelText('Título')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Salvar' })).toBeNull()
    expect(app.AddGame).not.toHaveBeenCalled()
    expect(app.ListGames).toHaveBeenCalledTimes(listCalls)
    expect(rowOf('Celeste')).toHaveTextContent('1 min')
    expect(screen.getByRole('button', { name: 'Adicionar jogo' })).toBeEnabled()
  })

  it('UI-007 cancelar no formulário não muda nada', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Celeste' })])
    app.PickGameExe.mockResolvedValue(picked)
    renderApp()
    await findRow('Celeste')

    fireEvent.click(screen.getByRole('button', { name: 'Adicionar jogo' }))
    await screen.findByLabelText('Título')
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))

    await waitFor(() => expect(screen.queryByLabelText('Título')).toBeNull())
    expect(app.AddGame).not.toHaveBeenCalled()
    expect(rowOf('Celeste')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// UI-008 visible errors and pending Jogar
// ---------------------------------------------------------------------------

describe('App: erros', () => {
  const launchMsg = 'Já há um jogo em andamento: Celeste. Feche-o antes de iniciar outro.'
  const addMsg = 'Escolha um arquivo .exe.'

  it('UI-008 falha de Jogar mostra a mensagem no aviso', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.LaunchGame.mockRejectedValue(launchMsg)
    renderApp()
    const row = await findRow('Hollow Knight')

    fireEvent.click(within(row).getByRole('button', { name: 'Jogar' }))
    const msg = await screen.findByText(launchMsg)
    expect(app.LaunchGame).toHaveBeenCalledWith(10)
    // The notice is at the top of the list.
    expect(precedes(msg, rowOf('Hollow Knight'))).toBe(true)
    // The button is back to Jogar after the failure.
    expect(within(rowOf('Hollow Knight')).getByRole('button', { name: 'Jogar' })).toBeEnabled()

    fireEvent.click(screen.getByRole('button', { name: 'Fechar' }))
    await waitFor(() => expect(screen.queryByText(launchMsg)).toBeNull())
    expect(screen.queryByRole('button', { name: 'Fechar' })).toBeNull()
  })

  it('UI-008 Jogar pendente mostra Iniciando… desabilitado no App', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })])
    const pending = deferred<SessionView>()
    app.LaunchGame.mockImplementation(() => pending.promise)
    renderApp()
    const row = await findRow('Hollow Knight')

    fireEvent.click(within(row).getByRole('button', { name: 'Jogar' }))
    await waitFor(() => {
      const btn = within(rowOf('Hollow Knight')).getByRole('button', { name: 'Iniciando…' })
      expect(btn).toHaveAccessibleName('Iniciando…')
      expect(btn).toBeDisabled()
    })
    expect(within(rowOf('Hades')).getByRole('button', { name: 'Jogar' })).toBeInTheDocument()

    await act(async () => {
      pending.resolve(session({ gameId: 10, state: 'running' }))
      await pending.promise
    })
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Iniciando…' })).toBeNull())
  })

  it('UI-008 erro novo substitui o anterior', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.LaunchGame.mockRejectedValue(launchMsg)
    app.PickGameExe.mockResolvedValue({ path: 'C:\\Games\\readme.txt', title: 'readme' })
    app.AddGame.mockRejectedValue(addMsg)
    renderApp()
    const row = await findRow('Hollow Knight')

    fireEvent.click(within(row).getByRole('button', { name: 'Jogar' }))
    await screen.findByText(launchMsg)

    fireEvent.click(screen.getByRole('button', { name: 'Adicionar jogo' }))
    await screen.findByLabelText('Título')
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await screen.findByText(addMsg)
    expect(screen.queryByText(launchMsg)).toBeNull()
    expect(screen.getAllByRole('button', { name: 'Fechar' })).toHaveLength(1)
  })

  it('UI-008 ação bem-sucedida remove o aviso', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.LaunchGame.mockRejectedValueOnce(launchMsg).mockResolvedValueOnce(session({ gameId: 10 }))
    renderApp()
    const row = await findRow('Hollow Knight')

    fireEvent.click(within(row).getByRole('button', { name: 'Jogar' }))
    await screen.findByText(launchMsg)

    await waitFor(() =>
      expect(within(rowOf('Hollow Knight')).getByRole('button', { name: 'Jogar' })).toBeEnabled(),
    )
    fireEvent.click(within(rowOf('Hollow Knight')).getByRole('button', { name: 'Jogar' }))
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.queryByText(launchMsg)).toBeNull())
    expect(screen.queryByRole('button', { name: 'Fechar' })).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// UI-006 running game, UI-012 panel
// ---------------------------------------------------------------------------

describe('App: sessão', () => {
  it('UI-006 CurrentSession ao carregar marca Em execução', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })])
    app.CurrentSession.mockResolvedValue(session({ gameId: 20, title: 'Hades' }))
    renderApp()
    await findRow('Hollow Knight')

    await waitFor(() => expect(rowOf('Hades')).toHaveTextContent('Em execução'))
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')
  })

  it('UI-006 ended remove a marcação e recarrega a lista', async () => {
    let total = 0
    app.ListGames.mockImplementation(() => Promise.resolve([game({ id: 10, totalPlaySeconds: total })]))
    app.CurrentSession.mockResolvedValue(session({ gameId: 10 }))
    renderApp()
    await waitConnected()
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveTextContent('Em execução'))
    expect(rowOf('Hollow Knight')).toHaveTextContent('Nunca jogado')
    const listCalls = app.ListGames.mock.calls.length

    total = 600
    emit('session:changed', session({ gameId: 10, state: 'ended', endedAt: '2026-11-09T10:10:00.000Z', activeMs: 600000 }))

    await waitFor(() => expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução'))
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBeGreaterThan(listCalls))
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveTextContent('10 min'))
  })

  it('UI-006 voltar o foco ou a visibilidade da janela não chama CurrentSession nem ListGames', async () => {
    app.ListGames.mockResolvedValue([game()])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10 }))
    renderApp()
    await waitConnected()
    await screen.findByRole('status', { name: panelName })
    // Let the reconnect reload (if any) finish before counting.
    await waitFor(() => expect(screen.queryByText('Conectando ao agente…')).toBeNull())
    await act(async () => {
      await new Promise((r) => setTimeout(r, 20))
    })
    const listCalls = app.ListGames.mock.calls.length
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
      await act(async () => {
        await new Promise((r) => setTimeout(r, 20))
      })
    } finally {
      focusManager.setFocused(undefined)
    }

    expect(app.ListGames).toHaveBeenCalledTimes(listCalls)
    expect(app.CurrentSession).toHaveBeenCalledTimes(sessionCalls)
    expect(queryPanel()).not.toBeNull()
  })

  it('UI-012 sem sessão o painel não existe', async () => {
    app.ListGames.mockResolvedValue([game()])
    app.CurrentSession.mockResolvedValue(null)
    renderApp()
    await waitConnected()
    await findRow('Hollow Knight')
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalled())
    await act(async () => {
      await Promise.resolve()
    })
    expect(queryPanel()).toBeNull()

    // A running event makes it appear above the list, with the session title.
    emit('session:changed', session({ gameId: 10, title: 'Hollow Knight', activeMs: 252400 }))
    const panel = await screen.findByRole('status', { name: panelName })
    expect(panel).toHaveTextContent('Hollow Knight')
    expect(panel).toHaveTextContent('0:04:1')
    expect(precedes(panel, rowOf('Hollow Knight'))).toBe(true)
  })

  it('UI-012 LaunchGame bem-sucedido mostra o painel', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.LaunchGame.mockResolvedValue(session({ gameId: 10, title: 'Hollow Knight', activeMs: 0 }))
    renderApp()
    await waitConnected()
    const row = await findRow('Hollow Knight')
    fireEvent.click(within(row).getByRole('button', { name: 'Jogar' }))

    const panel = await screen.findByRole('status', { name: panelName })
    expect(panel).toHaveTextContent('Hollow Knight')
    expect(panel).toHaveTextContent('0:00:0')
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveTextContent('Em execução'))
  })

  it('UI-012 discarded remove o painel', async () => {
    app.ListGames.mockResolvedValue([game()])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, activeMs: 3000 }))
    renderApp()
    await waitConnected()
    await screen.findByRole('status', { name: panelName })

    emit('session:changed', session({ gameId: 10, state: 'discarded', endedAt: '2026-11-09T10:00:03.000Z' }))
    await waitFor(() => expect(queryPanel()).toBeNull())
    await waitFor(() => expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução'))
  })
})

// ---------------------------------------------------------------------------
// UI-016 / UI-009 disconnect and reconnect
// ---------------------------------------------------------------------------

describe('App: desconexão e reconexão', () => {
  async function disconnect() {
    app.AgentStatus.mockResolvedValue(status(false))
    emit('agent:heartbeat')
    await waitFor(() => expect(indicatorState()).toBe('disconnected'), { timeout: 3000 })
  }

  async function reconnect() {
    app.AgentStatus.mockResolvedValue(status(true))
    emit('agent:heartbeat')
    await waitConnected()
  }

  it('UI-016 Desconectado esconde o painel', async () => {
    app.ListGames.mockResolvedValue([game()])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, activeMs: 60000 }))
    renderApp()
    await waitConnected()
    await screen.findByRole('status', { name: panelName })

    await disconnect()
    await waitFor(() => expect(queryPanel()).toBeNull())
  })

  it('UI-016 ao reconectar o painel volta só com o valor recarregado', async () => {
    app.ListGames.mockResolvedValue([game()])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, activeMs: 60000 }))
    renderApp()
    await waitConnected()
    expect(await screen.findByRole('status', { name: panelName })).toHaveTextContent('0:01:0')

    await disconnect()
    await waitFor(() => expect(queryPanel()).toBeNull())

    const reload = deferred<SessionView | null>()
    app.CurrentSession.mockImplementation(() => reload.promise)
    const sessionCalls = app.CurrentSession.mock.calls.length
    await reconnect()
    await waitFor(() => expect(app.CurrentSession.mock.calls.length).toBeGreaterThan(sessionCalls))

    // Connected again but the reload is pending: the old value must not come back.
    await act(async () => {
      await Promise.resolve()
    })
    expect(queryPanel()).toBeNull()

    await act(async () => {
      reload.resolve(session({ gameId: 10, activeMs: 180000 }))
      await reload.promise
    })
    const panel = await screen.findByRole('status', { name: panelName })
    expect(panel).toHaveTextContent('0:03:0')
    expect(panel).not.toHaveTextContent('0:01:0')
  })

  it('UI-016 o painel antigo não aparece nem na renderização da reconexão', async () => {
    app.ListGames.mockResolvedValue([game()])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, activeMs: 60000 }))
    renderApp()
    await waitConnected()
    expect(await screen.findByRole('status', { name: panelName })).toHaveTextContent('0:01:0')

    await disconnect()
    await waitFor(() => expect(queryPanel()).toBeNull())

    // The reload stays pending, so any panel seen while connected is the old one.
    const reload = deferred<SessionView | null>()
    app.CurrentSession.mockImplementation(() => reload.promise)

    // Record every DOM commit in which the indicator says connected and a panel exists.
    const frames: string[] = []
    const observer = new MutationObserver(() => {
      const panel = queryPanel()
      if (indicatorState() === 'connected' && panel) {
        frames.push(panel.textContent ?? '')
      }
    })
    observer.observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true })
    try {
      app.AgentStatus.mockResolvedValue(status(true))
      emit('agent:heartbeat')
      await waitFor(() => expect(indicatorState()).toBe('connected'))
      // Right after the reconnect commit, before more timers run.
      expect(queryPanel()).toBeNull()
      await act(async () => {
        await Promise.resolve()
      })
      expect(queryPanel()).toBeNull()
    } finally {
      observer.disconnect()
    }
    expect(frames, 'commits with the old panel while connected').toEqual([])

    await act(async () => {
      reload.resolve(session({ gameId: 10, activeMs: 180000 }))
      await reload.promise
    })
    expect(await screen.findByRole('status', { name: panelName })).toHaveTextContent('0:03:0')
  })

  it('UI-009 reconectar chama ListGames e CurrentSession', async () => {
    app.ListGames.mockResolvedValue([game()])
    app.CurrentSession.mockResolvedValue(null)
    renderApp()
    await waitConnected()
    await findRow('Hollow Knight')
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalled())

    await disconnect()
    // Let any in-flight load finish before counting.
    await act(async () => {
      await Promise.resolve()
    })
    const listCalls = app.ListGames.mock.calls.length
    const sessionCalls = app.CurrentSession.mock.calls.length

    await reconnect()
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBeGreaterThan(listCalls))
    await waitFor(() => expect(app.CurrentSession.mock.calls.length).toBeGreaterThan(sessionCalls))
  })
})

// ---------------------------------------------------------------------------
// UI-017 the clock only displays; UI-018 event beats a late response
// ---------------------------------------------------------------------------

describe('App: cronômetro e ordem', () => {
  it('UI-017 10 min com painel não chamam bindings extras e o total só muda depois de ended', async () => {
    vi.useFakeTimers({ toFake: [...fakeTimerApis] })
    let total = 300
    app.ListGames.mockImplementation(() => Promise.resolve([game({ id: 10, totalPlaySeconds: total })]))
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, activeMs: 252400 }))
    renderApp()
    await settle()

    const panel = screen.getByRole('status', { name: panelName })
    expect(panel).toHaveTextContent('0:04:1')
    expect(rowOf('Hollow Knight')).toHaveTextContent('5 min')

    const counted = ['CurrentSession', 'LaunchGame', 'AddGame', 'ListGames'] as const
    const before = Object.fromEntries(counted.map((n) => [n, app[n].mock.calls.length]))

    // 10 min in 10 s steps (the agent status poll keeps running meanwhile).
    for (let i = 0; i < 60; i++) {
      await tick(10_000)
    }

    for (const n of counted) {
      expect(app[n].mock.calls.length, `binding ${n}`).toBe(before[n])
    }
    // 252.4 s + ~600 s: the clock advanced about 10 minutes ("0:14:1x").
    expect(screen.getByRole('status', { name: panelName })).toHaveTextContent('0:14:1')
    // The list total is still the one from game.list.
    expect(rowOf('Hollow Knight')).toHaveTextContent('5 min')
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('14 min')

    total = 900
    emit('session:changed', session({ gameId: 10, state: 'ended', endedAt: '2026-11-09T10:15:00.000Z', activeMs: 852400 }))
    await settle()

    expect(app.ListGames.mock.calls.length).toBeGreaterThan(before.ListGames)
    expect(rowOf('Hollow Knight')).toHaveTextContent('15 min')
    expect(queryPanel()).toBeNull()
  })

  it('UI-018 evento antes da resposta atrasada de CurrentSession vence', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    const late = deferred<SessionView | null>()
    app.CurrentSession.mockImplementation(() => late.promise)
    renderApp()
    // Connected first so a reconnect reload does not happen after the event.
    await waitConnected()
    await findRow('Hollow Knight')
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalled())

    emit('session:changed', session({ gameId: 10, state: 'ended', endedAt: '2026-11-09T10:30:00.000Z' }))
    await act(async () => {
      late.resolve(session({ gameId: 10, state: 'running', activeMs: 99000 }))
      await late.promise
    })
    await act(async () => {
      await Promise.resolve()
    })

    expect(queryPanel()).toBeNull()
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')
  })
})

// ---------------------------------------------------------------------------
// UI-020/021 untracked launch, UI-022/024/027/028 pause and resume
// ---------------------------------------------------------------------------

function toggleIn(panelEl: HTMLElement | null): HTMLElement {
  expect(panelEl, 'session panel').not.toBeNull()
  return within(panelEl as HTMLElement).getByRole('button', { name: /Pausar|Retomar/ })
}

function untrackedOf(title: string): HTMLElement {
  return within(rowOf(title)).getByRole('button', { name: 'Sem contar tempo' })
}

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 20))
  })
}

describe('App: sem contar tempo', () => {
  it('UI-020/Sem contar tempo chama LaunchGameUntracked(G) uma vez e não LaunchGame; sem painel, sem Em execução, sem recarregar a lista', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.CurrentSession.mockResolvedValue(null)
    app.LaunchGameUntracked.mockResolvedValue(undefined)
    renderApp()
    await waitConnected()
    await findRow('Hollow Knight')
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalled())
    await waitFor(() => expect(screen.queryByText('Conectando ao agente…')).toBeNull())
    await flush()
    const listCalls = app.ListGames.mock.calls.length

    fireEvent.click(untrackedOf('Hollow Knight'))
    await waitFor(() => expect(app.LaunchGameUntracked).toHaveBeenCalledTimes(1))
    expect(app.LaunchGameUntracked).toHaveBeenCalledWith(10)
    await flush()

    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(queryPanel()).toBeNull()
    expect(screen.queryByText(/Em execução|Pausado/)).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(app.ListGames.mock.calls.length).toBe(listCalls)
  })

  it('UI-020/erro -32002 mostra o aviso do topo com Fechar e a próxima ação bem-sucedida o remove', async () => {
    const msg = 'Já há um jogo em andamento: Celeste. Feche-o antes de iniciar outro.'
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.LaunchGameUntracked.mockRejectedValueOnce(msg).mockResolvedValueOnce(undefined)
    renderApp()
    await findRow('Hollow Knight')

    fireEvent.click(untrackedOf('Hollow Knight'))
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(msg)
    expect(within(alert).getByRole('button', { name: 'Fechar' })).toBeInTheDocument()
    expect(app.LaunchGame).not.toHaveBeenCalled()

    fireEvent.click(untrackedOf('Hollow Knight'))
    await waitFor(() => expect(app.LaunchGameUntracked).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
    expect(screen.queryByRole('button', { name: 'Fechar' })).toBeNull()
  })

  it('UI-021/lançamento pendente sem contar tempo desabilita os dois botões do jogo no App', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })])
    const pending = deferred<void>()
    app.LaunchGameUntracked.mockImplementation(() => pending.promise)
    renderApp()
    await findRow('Hollow Knight')

    fireEvent.click(untrackedOf('Hollow Knight'))
    await waitFor(() => {
      const row = rowOf('Hollow Knight')
      expect(within(row).getByRole('button', { name: 'Iniciando…' })).toBeDisabled()
      expect(within(row).getByRole('button', { name: 'Jogar' })).toBeDisabled()
    })
    expect(within(rowOf('Hades')).getByRole('button', { name: 'Jogar' })).toBeEnabled()
    expect(within(rowOf('Hades')).getByRole('button', { name: 'Sem contar tempo' })).toBeEnabled()

    await act(async () => {
      pending.resolve()
      await pending.promise
    })
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Iniciando…' })).toBeNull())
  })

  it('UI-022/painel carregado por CurrentSession em paused mostra Pausado', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, state: 'paused', activeMs: 100000 }))
    renderApp()
    await waitConnected()

    const p = await screen.findByRole('status', { name: panelName })
    expect(p).toHaveTextContent('Hollow Knight')
    expect(p).toHaveTextContent('0:01:40')
    expect(within(p).getByText('Pausado')).toBeInTheDocument()
    expect(p).not.toHaveTextContent('Encerrando…')
    expect(within(p).getByRole('button', { name: 'Retomar' })).toBeEnabled()
    expect(rowOf('Hollow Knight')).toHaveTextContent('Pausado')
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')
  })

  it('UI-024/sessão paused mostra Pausado no lugar de Em execução; ended ou discarded: a marcação some e a lista é recarregada', async () => {
    let total = 0
    app.ListGames.mockImplementation(() => Promise.resolve([game({ id: 10, totalPlaySeconds: total })]))
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, state: 'running' }))
    renderApp()
    await waitConnected()
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveTextContent('Em execução'))

    emit('session:changed', session({ gameId: 10, state: 'paused', activeMs: 100000 }))
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveTextContent('Pausado'))
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')

    const listCalls = app.ListGames.mock.calls.length
    total = 100
    emit('session:changed', session({ gameId: 10, state: 'ended', endedAt: '2026-11-09T10:10:00.000Z', activeMs: 100000 }))
    await waitFor(() => expect(rowOf('Hollow Knight')).not.toHaveTextContent('Pausado'))
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBeGreaterThan(listCalls))
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveTextContent('1 min'))
    expect(screen.queryByText(/Em execução|Pausado/)).toBeNull()
    expect(queryPanel()).toBeNull()
  })

  it('UI-027/botão desabilitado durante a chamada pendente (sem duplo clique)', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, state: 'running', activeMs: 100000 }))
    const pending = deferred<SessionView>()
    app.PauseSession.mockImplementation(() => pending.promise)
    renderApp()
    await waitConnected()
    await screen.findByRole('status', { name: panelName })

    fireEvent.click(toggleIn(queryPanel()))
    await waitFor(() => expect(toggleIn(queryPanel())).toBeDisabled())

    fireEvent.click(toggleIn(queryPanel()))
    await flush()
    expect(app.PauseSession).toHaveBeenCalledTimes(1)
    expect(app.ResumeSession).not.toHaveBeenCalled()

    await act(async () => {
      pending.resolve(session({ gameId: 10, state: 'paused', activeMs: 100000 }))
      await pending.promise
    })
    await waitFor(() => expect(toggleIn(queryPanel())).toBeEnabled())
    expect(toggleIn(queryPanel())).toHaveAccessibleName('Retomar')
  })

  it('UI-028/clicar Pausar chama PauseSession uma vez e Retomar chama ResumeSession uma vez', async () => {
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, state: 'running', activeMs: 100000 }))
    app.PauseSession.mockResolvedValue(session({ gameId: 10, state: 'paused', activeMs: 100000 }))
    app.ResumeSession.mockResolvedValue(session({ gameId: 10, state: 'running', activeMs: 100000 }))
    renderApp()
    await waitConnected()
    await screen.findByRole('status', { name: panelName })

    fireEvent.click(within(queryPanel() as HTMLElement).getByRole('button', { name: 'Pausar' }))
    await waitFor(() => expect(app.PauseSession).toHaveBeenCalledTimes(1))
    expect(app.PauseSession).toHaveBeenCalledWith()
    await waitFor(() => expect(within(queryPanel() as HTMLElement).getByText('Pausado')).toBeInTheDocument())
    expect(app.ResumeSession).not.toHaveBeenCalled()

    fireEvent.click(within(queryPanel() as HTMLElement).getByRole('button', { name: 'Retomar' }))
    await waitFor(() => expect(app.ResumeSession).toHaveBeenCalledTimes(1))
    expect(app.ResumeSession).toHaveBeenCalledWith()
    expect(app.PauseSession).toHaveBeenCalledTimes(1)
    await waitFor(() =>
      expect(within(queryPanel() as HTMLElement).getByRole('button', { name: 'Pausar' })).toBeEnabled(),
    )
  })

  it('UI-028/erro (-32005, conexão perdida) mostra o aviso, o painel não muda e a próxima ação bem-sucedida o remove', async () => {
    const notPausable = 'Não há jogo em andamento para pausar.'
    const lost = 'O agente não está respondendo. Tente de novo.'
    app.ListGames.mockResolvedValue([game({ id: 10, title: 'Hollow Knight' })])
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, state: 'running', activeMs: 100000 }))
    app.PauseSession.mockRejectedValueOnce(notPausable)
      .mockRejectedValueOnce(lost)
      .mockResolvedValueOnce(session({ gameId: 10, state: 'paused', activeMs: 100000 }))
    renderApp()
    await waitConnected()
    await screen.findByRole('status', { name: panelName })

    fireEvent.click(toggleIn(queryPanel()))
    expect(await screen.findByRole('alert')).toHaveTextContent(notPausable)
    expect(queryPanel()).toHaveTextContent('Hollow Knight')
    expect(toggleIn(queryPanel())).toHaveAccessibleName('Pausar')
    expect(queryPanel()).not.toHaveTextContent('Pausado')

    fireEvent.click(toggleIn(queryPanel()))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(lost))
    expect(screen.getByRole('alert')).not.toHaveTextContent(notPausable)
    expect(toggleIn(queryPanel())).toHaveAccessibleName('Pausar')
    expect(queryPanel()).not.toHaveTextContent('Pausado')

    // The next successful action removes the banner.
    fireEvent.click(toggleIn(queryPanel()))
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
    expect(app.PauseSession).toHaveBeenCalledTimes(3)
    await waitFor(() => expect(toggleIn(queryPanel())).toHaveAccessibleName('Retomar'))
  })
})
