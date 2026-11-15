import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query'
import App from './App'
import type { SessionView } from './lib/games'
import { gameNotFoundMessage, type CatalogView, type GameDetailView, type GameListView, type GameView } from './lib/library'
import { readLibraryView, writeLibraryView } from './lib/libraryView'

// ---------------------------------------------------------------------------
// Fakes for window.go.main.App and window.runtime.EventsOn (as in lib/games.test.tsx)
// ---------------------------------------------------------------------------

type Fn = ReturnType<typeof vi.fn>
type FakeApp = {
  AddGame: Fn
  AgentStatus: Fn
  Catalog: Fn
  CurrentSession: Fn
  DeleteGame: Fn
  GameSessions: Fn
  GetGame: Fn
  LaunchGame: Fn
  LaunchGameUntracked: Fn
  ListGames: Fn
  PickGameExe: Fn
  PauseSession: Fn
  ResumeSession: Fn
  UpdateGame: Fn
  MediaList: Fn
  MetadataSearch: Fn
  MetadataApply: Fn
  StatsSummary: Fn
  StatsWeekly: Fn
}

let app: FakeApp
let handlers: Map<string, Set<(...data: unknown[]) => void>>
let rectSpy: ReturnType<typeof vi.spyOn> | undefined

// Every element reports this box, so the grid and the list compute their window from a
// fixed size [UI-029]. With 800 px the grid has 3 columns and about 4 visible rows.
const BOX_WIDTH = 800
const BOX_HEIGHT = 600

class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

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

function catalog(over: Partial<CatalogView> = {}): CatalogView {
  return {
    platforms: [{ id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 1 }],
    genres: [],
    companies: [],
    playModes: [],
    ...over,
  }
}

function install() {
  handlers = new Map()
  app = {
    AddGame: vi.fn(() => Promise.resolve(game())),
    AgentStatus: vi.fn(() => Promise.resolve(status(true))),
    Catalog: vi.fn(() => Promise.resolve(catalog())),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    DeleteGame: vi.fn(() => Promise.resolve()),
    GameSessions: vi.fn(() => Promise.resolve({ sessions: [], total: 0, offset: 0 })),
    GetGame: vi.fn(() => Promise.resolve(detail())),
    LaunchGame: vi.fn(() => Promise.resolve(session())),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    ListGames: vi.fn(() => Promise.resolve(page([]))),
    PickGameExe: vi.fn(() => Promise.resolve({ path: '', title: '' })),
    PauseSession: vi.fn(() => Promise.resolve(session({ state: 'paused' }))),
    ResumeSession: vi.fn(() => Promise.resolve(session({ state: 'running' }))),
    UpdateGame: vi.fn((_id: number, patch: Partial<GameDetailView>) => Promise.resolve(detail(patch))),
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
    MetadataSearch: vi.fn(() => Promise.resolve({ candidates: [], confidence: 'none', warnings: [] })),
    MetadataApply: vi.fn(() => Promise.resolve({ game: detail(), queued: 0, skippedByLimit: 0 })),
    StatsSummary: vi.fn(() => Promise.resolve({ games: [], platforms: [], totalSeconds: 0, importedSeconds: 0 })),
    StatsWeekly: vi.fn(() => Promise.resolve({ weeks: [] })),
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
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
  rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () =>
      ({
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        bottom: BOX_HEIGHT,
        right: BOX_WIDTH,
        width: BOX_WIDTH,
        height: BOX_HEIGHT,
        toJSON: () => ({}),
      }) as DOMRect,
  )
}

function uninstall() {
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
  rectSpy?.mockRestore()
  rectSpy = undefined
  vi.unstubAllGlobals()
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

function detail(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    ...game(),
    description: '',
    completedAt: null,
    developers: [],
    publishers: [],
    genres: [],
    playModes: [],
    launchType: 'exe',
    launchArgs: '',
    emulatorId: null,
    ...over,
  }
}

// page is one answer of game.list: the loaded games, the total and the offset [IPC-042].
function page(games: GameView[], total = games.length, offset = 0): GameListView {
  return { games, total, offset }
}

// makeGames builds n games titled "Jogo 0".."Jogo n-1".
function makeGames(n: number): GameView[] {
  return Array.from({ length: n }, (_, i) => game({ id: i + 1, title: `Jogo ${i}` }))
}

// libraryOf answers ListGames from a fixed set, page by page, like the agent does [UI-036].
function libraryOf(all: GameView[]) {
  return (args: { offset: number; limit: number }) =>
    Promise.resolve(page(all.slice(args.offset, args.offset + args.limit), all.length, args.offset))
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

function wrapperOf(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

function renderApp() {
  const qc = newClient()
  render(<App />, { wrapper: wrapperOf(qc) })
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

// rowOf returns the card (grid) or the row (list) of the game: the option that holds the
// title as an element of its own. Only the options of the library are looked at.
function rowOf(title: string): HTMLElement {
  const candidates = cards().filter((o) => within(o).queryAllByText(title).length > 0)
  expect(candidates, `one option with the title ${title}`).toHaveLength(1)
  return candidates[0]
}

// cards are the options of the library listbox only: the native sort select also has options.
function cards(): HTMLElement[] {
  const listbox = screen.queryByRole('listbox', { name: 'Jogos' })
  return listbox ? within(listbox).queryAllByRole('option') : []
}

function selectedCards(): HTMLElement[] {
  return cards().filter((o) => o.getAttribute('aria-selected') === 'true')
}

async function findRow(title: string): Promise<HTMLElement> {
  await waitFor(() => expect(cards().length).toBeGreaterThan(0))
  let row!: HTMLElement
  await waitFor(() => {
    row = rowOf(title)
  })
  return row
}

// asideOf is the details panel column [UIX-004].
function asideOf(): HTMLElement {
  return screen.getByRole('complementary', { name: 'Detalhes do jogo' })
}

// focusGame clicks the card of the game and waits for the panel heading [UIX-017].
async function focusGame(title: string): Promise<HTMLElement> {
  const card = await findRow(title)
  fireEvent.click(within(card).getByText(title))
  await within(asideOf()).findByRole('heading', { level: 2, name: title })
  return asideOf()
}

// pageCalls are the ListGames calls of the library pages; the recent-games read of the empty
// panel (limit 5, UIX-024) is not one of them.
function pageCalls(): unknown[][] {
  return app.ListGames.mock.calls.filter((c) => (c[0] as { limit: number }).limit !== 5)
}

// scrollGridTo moves the window of the grid to top px. Every ancestor of the first card gets the
// scrollTop and a scroll event, so the test does not depend on the markup.
function scrollGridTo(top: number): void {
  const anchor = cards()[0]
  for (let el: HTMLElement | null = anchor; el !== null; el = el.parentElement) {
    Object.defineProperty(el, 'scrollTop', { configurable: true, value: top })
    fireEvent.scroll(el)
  }
}

async function disconnectAgent() {
  app.AgentStatus.mockResolvedValue(status(false))
  emit('agent:heartbeat')
  await waitFor(() => expect(indicatorState()).toBe('disconnected'), { timeout: 3000 })
}

async function reconnectAgent() {
  app.AgentStatus.mockResolvedValue(status(true))
  emit('agent:heartbeat')
  await waitConnected()
}

// trackEmptyPanel records whether the empty panel showed up at any commit while it runs [UIX-021].
function trackEmptyPanel(): { sawEmpty: () => boolean; stop: () => void } {
  let saw = false
  const observer = new MutationObserver(() => {
    if (document.body.textContent?.includes('Nenhum jogo selecionado')) {
      saw = true
    }
  })
  observer.observe(document.body, { subtree: true, childList: true, characterData: true })
  return { sawEmpty: () => saw, stop: () => observer.disconnect() }
}

// A library of three games on two platforms; ListGames and GetGame answer from it.
function threePlatformLibrary() {
  const hk = game({ id: 10, title: 'Hollow Knight', platformId: 1 })
  const hades = game({ id: 20, title: 'Hades', platformId: 1 })
  const gt = game({ id: 30, title: 'Gran Turismo', platformId: 2, platform: 'PlayStation 2' })
  const all = [hk, hades, gt]
  app.Catalog.mockResolvedValue(
    catalog({
      platforms: [
        { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 2 },
        { id: 2, name: 'PlayStation 2', category: 'console', emulatorId: null, seedKey: 'ps2', iconUrl: '', gameCount: 1 },
      ],
    }),
  )
  app.ListGames.mockImplementation(
    ({ filter, offset, limit }: { filter: { platformIds?: number[] }; offset: number; limit: number }) => {
      const rows = filter.platformIds === undefined ? all : all.filter((g) => filter.platformIds?.includes(g.platformId))
      return Promise.resolve(page(rows.slice(offset, offset + limit), rows.length, offset))
    },
  )
  app.GetGame.mockImplementation((id: number) => {
    const g = all.find((x) => x.id === id)
    return g ? Promise.resolve(detail({ ...g })) : Promise.reject(gameNotFoundMessage)
  })
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

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 20))
  })
}

beforeEach(() => {
  localStorage.clear()
  install()
})

afterEach(() => {
  vi.useRealTimers()
  uninstall()
})

// ---------------------------------------------------------------------------
// UI-044 empty state, loading and count (replaces UI-010)
// ---------------------------------------------------------------------------

describe('App: estado vazio e carregamento', () => {
  it('UI-010 [] mostra Nenhum jogo ainda sem contadores, com a dica de Importar', async () => {
    app.ListGames.mockResolvedValue(page([]))
    renderApp()

    expect(await screen.findByText('Nenhum jogo ainda')).toBeInTheDocument()
    expect(screen.queryByText('Conectando ao agente…')).toBeNull()
    expect(screen.queryByText(/jogos ·|plataformas/)).toBeNull()
    expect(screen.getByText('Use Importar para adicionar jogos.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Adicionar jogo' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Jogar' })).toBeNull()
  })

  it('UI-010 carregando mostra Conectando ao agente…', async () => {
    const pending = deferred<GameListView>()
    app.ListGames.mockImplementation(() => pending.promise)
    renderApp()

    await waitFor(() => expect(app.ListGames).toHaveBeenCalled())
    expect(screen.getByText('Conectando ao agente…')).toBeInTheDocument()
    expect(screen.queryByText('Nenhum jogo ainda')).toBeNull()

    await act(async () => {
      pending.resolve(page([game()]))
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
    expect(pageCalls()).toHaveLength(1)

    await tick(1800)
    expect(pageCalls()).toHaveLength(1)
    await tick(400)
    expect(pageCalls()).toHaveLength(2)
    expect(screen.getByText('Conectando ao agente…')).toBeInTheDocument()
    await tick(2000)
    expect(pageCalls()).toHaveLength(3)

    // When the call starts working the grid replaces the loading text.
    app.ListGames.mockResolvedValue(page([game()]))
    await tick(2000)
    await settle()
    expect(screen.queryByText('Conectando ao agente…')).toBeNull()
    expect(cards()).toHaveLength(1)
  })

  it('UIX-013/"Conectando ao agente…", "Nenhum jogo ainda" e "Nenhum jogo com estes filtros." aparecem no centro', async () => {
    const centerOf = () => screen.getByRole('region', { name: 'Jogos' })

    // Connecting: the first page has no answer yet.
    const pending = deferred<GameListView>()
    app.ListGames.mockImplementation(() => pending.promise)
    const first = render(<App />, { wrapper: wrapperOf(newClient()) })
    await waitFor(() => expect(app.ListGames).toHaveBeenCalled())
    expect(within(centerOf()).getByText('Conectando ao agente…')).toBeInTheDocument()
    first.unmount()

    // Empty library.
    app.ListGames.mockResolvedValue(page([]))
    const second = render(<App />, { wrapper: wrapperOf(newClient()) })
    expect(await within(centerOf()).findByText('Nenhum jogo ainda')).toBeInTheDocument()
    second.unmount()

    // A filter without results.
    app.ListGames.mockImplementation(({ filter, ...rest }: { filter: { platformIds?: number[] } }) =>
      libraryOf(filter.platformIds ? [] : [game()])(rest as { offset: number; limit: number }),
    )
    render(<App />, { wrapper: wrapperOf(newClient()) })
    await findRow('Hollow Knight')
    fireEvent.click(screen.getByRole('button', { name: 'Windows, 1 jogos' }))
    expect(await within(centerOf()).findByText('Nenhum jogo com estes filtros.')).toBeInTheDocument()
    expect(within(centerOf()).getByRole('button', { name: 'Limpar filtros' })).toBeInTheDocument()
  })

  it('UIX-004/três marcos: nav "Filtros da biblioteca", região "Jogos" e aside "Detalhes do jogo"; main com h-screen e overflow-hidden', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    await findRow('Hollow Knight')

    expect(screen.getByRole('navigation', { name: 'Filtros da biblioteca' })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Jogos' })).toBeInTheDocument()
    expect(screen.getByRole('complementary', { name: 'Detalhes do jogo' })).toBeInTheDocument()
    expect(screen.getByRole('main')).toHaveClass('h-screen', 'overflow-hidden')
  })

  it('UIX-024/ao abrir, nenhum jogo em foco e o painel mostra o resumo', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight', lastPlayedAt: '2026-11-09T10:00:00.000Z' })]))
    renderApp()
    await findRow('Hollow Knight')

    const aside = asideOf()
    expect(await within(aside).findByText('Nenhum jogo selecionado')).toBeInTheDocument()
    expect(selectedCards()).toHaveLength(0)
    expect(app.GetGame).not.toHaveBeenCalled()

    // The recent games come from the five last played [UIX-024].
    expect(app.ListGames).toHaveBeenCalledWith(
      expect.objectContaining({ filter: { version: 1, played: 'played' }, sort: 'lastPlayed', offset: 0, limit: 5 }),
    )
    expect(await within(aside).findByText('Jogados recentemente')).toBeInTheDocument()

    // Choosing one only puts it in focus; nothing is launched.
    fireEvent.click(within(aside).getByRole('button', { name: /Hollow Knight/ }))
    expect(await within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
    expect(app.GetGame).toHaveBeenCalledWith(10)
    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
  })

  it('UI-044/total maior que zero mostra "N jogos" com o total da resposta', async () => {
    app.ListGames.mockImplementation(libraryOf(makeGames(37)))
    renderApp()

    expect(await screen.findByText('37 jogos')).toBeInTheDocument()
    expect(screen.queryByText('Nenhum jogo ainda')).toBeNull()
  })

  it('UI-044/um único jogo usa o plural fixo "1 jogos"', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()

    expect(await screen.findByText('1 jogos')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// UI-004 grid replaces the list, UI-029 virtualized cards, UI-030 toggle
// ---------------------------------------------------------------------------

describe('App: grade e lista', () => {
  it('UI-004/a biblioteca mostra a grade de cards no lugar da lista (substitui o teste de UI-004)', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight', totalPlaySeconds: 300 }), game({ id: 20, title: 'Hades' })]))
    renderApp()

    const card = await findRow('Hollow Knight')
    expect(cards()).toHaveLength(2)
    // The card has the cover and the title only: no buttons, platform or time [UIX-014].
    expect(within(card).queryAllByRole('button')).toHaveLength(0)
    expect(within(card).queryByText('Windows')).toBeNull()
    expect(card).not.toHaveTextContent('5 min')
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    expect(screen.getByRole('button', { name: 'Mostrar em lista' })).toBeInTheDocument()
  })

  it('AJU-015/jogo sem capa e com screenshot mostra o screenshot no cartão', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.MediaList.mockResolvedValue({
      media: [{ id: 1, kind: 'screenshot', provider: 'x', path: 'p', thumbPath: 't', url: '/m/shot.jpg', thumbUrl: '/m/shot-t.jpg' }],
    })
    renderApp()

    const card = await findRow('Hollow Knight')
    await waitFor(() => expect(card.querySelector('img')).toHaveAttribute('src', '/m/shot-t.jpg'))
    expect(card.querySelector('img')).toHaveAttribute('data-fit', 'cover')
  })

  it('AJU-015/MediaList com erro mostra o placeholder no cartão', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.MediaList.mockRejectedValue(new Error('boom'))
    renderApp()

    const card = await findRow('Hollow Knight')
    await waitFor(() => expect(app.MediaList).toHaveBeenCalled())
    expect(card.querySelector('img')).not.toHaveAttribute('data-fit')
  })

  it('UIX-016/a grade renderiza no máximo 100 cartões mesmo com 10 000 jogos na biblioteca', async () => {
    app.ListGames.mockImplementation(libraryOf(makeGames(10_000)))
    renderApp()

    await findRow('Jogo 0')
    const shown = cards()
    expect(shown.length).toBeGreaterThan(0)
    expect(shown.length).toBeLessThanOrEqual(100)
    expect(screen.getByText('10000 jogos')).toBeInTheDocument()
  })

  it('UIX-042/grade com role listbox "Jogos" e cartões role option com o título como nome', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    renderApp()
    await findRow('Hollow Knight')

    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    expect(within(listbox).getByRole('option', { name: 'Hollow Knight' })).toBeInTheDocument()
    expect(within(listbox).getByRole('option', { name: 'Hades' })).toBeInTheDocument()
  })

  it('UI-030/o botão alterna grade e lista com os mesmos jogos, e a escolha fica salva na UI', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    renderApp()
    await findRow('Hollow Knight')
    // In the grid the card has no platform; the list row has it.
    expect(within(rowOf('Hollow Knight')).queryByText('Windows')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Mostrar em lista' }))
    await waitFor(() => expect(within(rowOf('Hollow Knight')).getByText('Windows')).toBeInTheDocument())
    expect(within(rowOf('Hades')).getByText('Windows')).toBeInTheDocument()
    expect(readLibraryView()).toBe('list')
    expect(cards()).toHaveLength(2)

    fireEvent.click(screen.getByRole('button', { name: 'Mostrar em grade' }))
    await waitFor(() => expect(within(rowOf('Hollow Knight')).queryByText('Windows')).toBeNull())
    expect(cards()).toHaveLength(2)
    expect(readLibraryView()).toBe('grid')
  })

  it('UI-030/a lista salva é usada ao abrir a biblioteca de novo', async () => {
    writeLibraryView('list')
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    renderApp()

    await findRow('Hollow Knight')
    expect(within(rowOf('Hollow Knight')).getByText('Windows')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Mostrar em grade' })).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// UI-032..UI-036 filters, search, sort, empty result and paging
// ---------------------------------------------------------------------------

describe('App: filtros, busca, ordenação e paginação', () => {
  it('UI-032/mudar a plataforma chama ListGames com o FilterSpec e a primeira página, e o contador mostra o total da resposta', async () => {
    const two = [game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]
    const many = makeGames(37)
    app.Catalog.mockResolvedValue(
      catalog({
        platforms: [
          { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 2 },
          { id: 2, name: 'PlayStation 2', category: 'console', emulatorId: null, seedKey: 'ps2', iconUrl: '', gameCount: 37 },
        ],
      }),
    )
    app.ListGames.mockImplementation(({ filter, ...rest }: { filter: { platformIds?: number[] } }) =>
      libraryOf(filter.platformIds ? many : two)(rest as { offset: number; limit: number }),
    )
    renderApp()
    await findRow('Hollow Knight')
    expect(screen.getByText('2 jogos')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'PlayStation 2, 37 jogos' }))
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(
        expect.objectContaining({ filter: { version: 1, platformIds: [2] }, offset: 0 }),
      ),
    )
    expect(await screen.findByText('37 jogos')).toBeInTheDocument()
  })

  it('UIX-010/clicar numa plataforma envia ListGames com platformIds=[P] e mantém as outras facetas', async () => {
    app.Catalog.mockResolvedValue(
      catalog({
        platforms: [
          { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 2 },
          { id: 2, name: 'PlayStation 2', category: 'console', emulatorId: null, seedKey: 'ps2', iconUrl: '', gameCount: 3 },
        ],
        genres: [{ id: 5, name: 'Ação' }],
      }),
    )
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    await findRow('Hollow Knight')

    fireEvent.click(screen.getByRole('button', { name: /^Filtros/ }))
    fireEvent.change(await screen.findByLabelText(/gênero/i), { target: { value: '5' } })
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(
        expect.objectContaining({ filter: { version: 1, genreIds: [5] } }),
      ),
    )

    fireEvent.click(screen.getByRole('button', { name: 'PlayStation 2, 3 jogos' }))
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(
        expect.objectContaining({ filter: { version: 1, genreIds: [5], platformIds: [2] }, offset: 0 }),
      ),
    )
  })

  it('UI-033/a busca é enviada 300 ms depois da última tecla, uma tecla isolada não é enviada e apagar o texto remove o filtro', async () => {
    vi.useFakeTimers({ toFake: [...fakeTimerApis] })
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    await settle()

    const box = screen.getByRole('searchbox', { name: 'Buscar na biblioteca' })
    const before = app.ListGames.mock.calls.length
    fireEvent.change(box, { target: { value: 'H' } })
    await tick(200)
    fireEvent.change(box, { target: { value: 'Ho' } })
    await tick(299)
    expect(app.ListGames.mock.calls.length).toBe(before)
    await tick(1)
    expect(app.ListGames.mock.calls.length).toBe(before + 1)
    expect(app.ListGames).toHaveBeenLastCalledWith(
      expect.objectContaining({ filter: { version: 1, text: 'Ho' }, offset: 0 }),
    )

    fireEvent.change(box, { target: { value: '' } })
    await tick(300)
    expect(app.ListGames).toHaveBeenLastCalledWith(expect.objectContaining({ filter: { version: 1 }, offset: 0 }))
  })

  it('UI-034/as quatro opções de ordenação recarregam a lista com o sort e voltam à primeira página', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    await findRow('Hollow Knight')

    const select = screen.getByLabelText('Ordenar por')
    expect(within(select).getAllByRole('option').map((o) => o.textContent)).toEqual([
      'Título',
      'Última vez jogado',
      'Tempo jogado',
      'Ano',
    ])
    fireEvent.change(select, { target: { value: 'totalPlayed' } })
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'totalPlayed', offset: 0 })),
    )
    expect(screen.getByLabelText('Ordenar por')).toHaveValue('totalPlayed')
  })

  it('UI-035/Limpar filtros volta o FilterSpec para {version: 1} e a ordenação ao padrão', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    await findRow('Hollow Knight')

    fireEvent.change(screen.getByLabelText('Ordenar por'), { target: { value: 'year' } })
    fireEvent.click(screen.getByRole('button', { name: 'Windows, 1 jogos' }))
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(
        expect.objectContaining({ filter: { version: 1, platformIds: [1] }, sort: 'year' }),
      ),
    )

    fireEvent.click(screen.getByRole('button', { name: /^Filtros/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Limpar filtros' }))
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(
        expect.objectContaining({ filter: { version: 1 }, sort: 'title', offset: 0 }),
      ),
    )
    expect(screen.getByLabelText('Ordenar por')).toHaveValue('title')
    expect(screen.queryByLabelText('Plataforma')).toBeNull()
  })

  it('UIX-011/"Limpar filtros" volta a {version: 1}, à ordenação padrão e a "Todos"', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    await findRow('Hollow Knight')

    fireEvent.change(screen.getByLabelText('Ordenar por'), { target: { value: 'year' } })
    fireEvent.click(screen.getByRole('button', { name: 'Windows, 1 jogos' }))
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(
        expect.objectContaining({ filter: { version: 1, platformIds: [1] }, sort: 'year' }),
      ),
    )
    expect(screen.getByRole('button', { name: 'Windows, 1 jogos' })).toHaveAttribute('aria-current', 'true')

    fireEvent.click(screen.getByRole('button', { name: /^Filtros/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Limpar filtros' }))
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(
        expect.objectContaining({ filter: { version: 1 }, sort: 'title', offset: 0 }),
      ),
    )
    expect(screen.getByRole('button', { name: /^Todos/ })).toHaveAttribute('aria-current', 'true')
    expect(screen.getByLabelText('Ordenar por')).toHaveValue('title')
  })

  it('UI-035/filtro sem resultado mostra "Nenhum jogo com estes filtros." e o botão Limpar filtros', async () => {
    app.ListGames.mockImplementation(({ filter, ...rest }: { filter: { platformIds?: number[] } }) =>
      libraryOf(filter.platformIds ? [] : [game()])(rest as { offset: number; limit: number }),
    )
    renderApp()
    await findRow('Hollow Knight')

    fireEvent.click(screen.getByRole('button', { name: 'Windows, 1 jogos' }))
    const message = await screen.findByText('Nenhum jogo com estes filtros.')
    const empty = message.closest('section') as HTMLElement
    expect(empty).not.toBeNull()

    fireEvent.click(within(empty).getByRole('button', { name: 'Limpar filtros' }))
    await findRow('Hollow Knight')
    expect(screen.queryByText('Nenhum jogo com estes filtros.')).toBeNull()
  })

  it('UI-036/a próxima página é pedida uma vez quando a janela chega ao fim e é anexada sem repetir jogos', async () => {
    app.ListGames.mockImplementation(libraryOf(makeGames(120)))
    renderApp()
    await findRow('Jogo 0')

    const offsets = () => pageCalls().map((c) => (c[0] as { offset: number }).offset)
    expect(offsets()).toEqual([0])

    scrollGridTo(2800)
    await waitFor(() => expect(offsets()).toContain(50))
    await flush()
    expect(offsets().filter((o) => o === 50)).toHaveLength(1)
    expect(offsets().filter((o) => o === 0)).toHaveLength(1)
    expect(screen.getByText('120 jogos')).toBeInTheDocument()
  })

  it('UI-036/mudar a ordenação descarta as páginas carregadas e pede a primeira página', async () => {
    app.ListGames.mockImplementation(libraryOf(makeGames(120)))
    renderApp()
    await findRow('Jogo 0')

    scrollGridTo(2800)
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ offset: 50 })),
    )
    await flush()

    fireEvent.change(screen.getByLabelText('Ordenar por'), { target: { value: 'year' } })
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'year', offset: 0 })),
    )
  })
})

// ---------------------------------------------------------------------------
// UI-037 game page, UI-038 sessions, UI-039/040 edit, UI-041 removal
// ---------------------------------------------------------------------------

describe('App: jogo em foco, painel, edição e remoção', () => {
  it('UIX-017/um clique marca o cartão (aria-selected) e desmarca o anterior; o painel mostra G; só leituras chamadas', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    app.GetGame.mockImplementation((id: number) =>
      Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Hades' })),
    )
    renderApp()

    const hk = await findRow('Hollow Knight')
    expect(hk).toHaveAttribute('aria-selected', 'false')
    fireEvent.click(within(hk).getByText('Hollow Knight'))
    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight' })
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'false')
    expect(app.GetGame).toHaveBeenCalledWith(10)

    fireEvent.click(within(rowOf('Hades')).getByText('Hades'))
    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'true')
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'false')
    expect(selectedCards()).toHaveLength(1)

    // Only reads happened [UIX-046].
    for (const name of [
      'LaunchGame',
      'LaunchGameUntracked',
      'AddGame',
      'UpdateGame',
      'DeleteGame',
      'PauseSession',
      'ResumeSession',
    ] as const) {
      expect(app[name], name).not.toHaveBeenCalled()
    }
  })

  it('UIX-018/duplo clique chama LaunchGame(G) exatamente uma vez e nunca LaunchGameUntracked', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    renderApp()
    const card = await findRow('Hollow Knight')

    fireEvent.doubleClick(within(card).getByText('Hollow Knight'))
    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight' })
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    expect(app.LaunchGame).toHaveBeenCalledWith(10)
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
  })

  it('UIX-018/com lançamento pendente, o duplo clique só põe em foco', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    app.GetGame.mockImplementation((id: number) =>
      Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Hades' })),
    )
    const pending = deferred<SessionView>()
    app.LaunchGame.mockImplementation(() => pending.promise)
    renderApp()
    const first = await findRow('Hollow Knight')

    fireEvent.doubleClick(within(first).getByText('Hollow Knight'))
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))

    fireEvent.doubleClick(within(rowOf('Hades')).getByText('Hades'))
    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'true')
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()

    await act(async () => {
      pending.resolve(session({ gameId: 10, state: 'running' }))
      await pending.promise
    })
  })

  it('UIX-018/erro do lançamento por duplo clique vai para o aviso do topo (UI-008)', async () => {
    const msg = 'Já há um jogo em andamento: Celeste. Feche-o antes de iniciar outro.'
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.LaunchGame.mockRejectedValue(msg)
    renderApp()
    const card = await findRow('Hollow Knight')

    fireEvent.doubleClick(within(card).getByText('Hollow Knight'))
    const notice = await screen.findByText(msg)
    expect(precedes(notice, rowOf('Hollow Knight'))).toBe(true)
    expect(within(screen.getByRole('alert')).getByRole('button', { name: 'Fechar' })).toBeInTheDocument()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
  })

  it('UIX-021/filtro que mantém G: painel com G sem passar pelo vazio e cartão marcado', async () => {
    threePlatformLibrary()
    renderApp()
    await focusGame('Hollow Knight')

    const tracker = trackEmptyPanel()
    try {
      fireEvent.click(screen.getByRole('button', { name: 'Windows, 2 jogos' }))
      await waitFor(() =>
        expect(app.ListGames).toHaveBeenLastCalledWith(
          expect.objectContaining({ filter: { version: 1, platformIds: [1] }, offset: 0 }),
        ),
      )
      await waitFor(() => expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true'))
      await flush()
    } finally {
      tracker.stop()
    }
    expect(tracker.sawEmpty()).toBe(false)
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'false')
  })

  it('UIX-021/filtro que tira G: painel com G e nenhum cartão marcado; filtro que o traz de volta: marcado de novo', async () => {
    threePlatformLibrary()
    renderApp()
    await focusGame('Hollow Knight')

    fireEvent.click(screen.getByRole('button', { name: 'PlayStation 2, 1 jogos' }))
    await findRow('Gran Turismo')
    await waitFor(() => expect(cards().length).toBe(1))
    expect(selectedCards()).toHaveLength(0)
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Windows, 2 jogos' }))
    const back = await findRow('Hollow Knight')
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true'))
    expect(back).toBeInTheDocument()
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
  })

  it('UIX-022/alternar grade e lista, abrir e fechar estatísticas e abrir e fechar um modal mantêm G no painel', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    app.StatsSummary.mockResolvedValue({ games: [], platforms: [], totalSeconds: 0, importedSeconds: 0 })
    renderApp()
    await focusGame('Hollow Knight')
    const heading = () => within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight' })

    // Grid to list and back.
    fireEvent.click(screen.getByRole('button', { name: 'Mostrar em lista' }))
    await heading()
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true'))
    fireEvent.click(screen.getByRole('button', { name: 'Mostrar em grade' }))
    await heading()
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true'))

    // Statistics open and close.
    fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
    expect(await screen.findByRole('heading', { name: 'Estatísticas' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Voltar' }))
    await heading()
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true'))

    // A modal opens and closes.
    fireEvent.click(screen.getByRole('button', { name: 'Plataformas' }))
    const dialog = await screen.findByRole('dialog', { name: 'Plataformas' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Fechar' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    await heading()
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
  })

  it('UIX-022/reconectar mantém G e relê GetGame(G)', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    renderApp()
    await waitConnected()
    await focusGame('Hollow Knight')
    await flush()
    const reads = app.GetGame.mock.calls.length

    const tracker = trackEmptyPanel()
    try {
      await disconnectAgent()
      await reconnectAgent()
      await waitFor(() => expect(app.GetGame.mock.calls.length).toBeGreaterThan(reads))
      await flush()
    } finally {
      tracker.stop()
    }
    expect(tracker.sawEmpty()).toBe(false)
    expect(await within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
    expect(app.GetGame).toHaveBeenLastCalledWith(10)
  })

  it('UIX-023/GetGame(G) com -32001: painel vazio e ListGames chamado de novo uma vez', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    renderApp()
    const card = await findRow('Hollow Knight')
    await flush()
    const before = pageCalls().length

    app.GetGame.mockRejectedValue(gameNotFoundMessage)
    fireEvent.click(within(card).getByText('Hollow Knight'))

    expect(await within(asideOf()).findByText('Nenhum jogo selecionado')).toBeInTheDocument()
    await waitFor(() => expect(pageCalls().length).toBe(before + 1))
    await flush()
    expect(pageCalls().length).toBe(before + 1)
    expect(selectedCards()).toHaveLength(0)
  })

  it('UIX-027/pendência de outro jogo: clicar em Jogar de G não chama nada', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    app.GetGame.mockImplementation((id: number) =>
      Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Hades' })),
    )
    const pending = deferred<SessionView>()
    app.LaunchGame.mockImplementation(() => pending.promise)
    renderApp()

    const aside = await focusGame('Hollow Knight')
    fireEvent.click(within(aside).getByRole('button', { name: 'Jogar' }))
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))

    // Hades is not the pending game: its buttons are active but do nothing until the launch ends.
    const other = await focusGame('Hades')
    expect(within(other).getByRole('button', { name: 'Jogar' })).toBeEnabled()
    expect(within(other).getByRole('button', { name: 'Jogar sem contar tempo' })).toBeEnabled()
    fireEvent.click(within(other).getByRole('button', { name: 'Jogar' }))
    fireEvent.click(within(other).getByRole('button', { name: 'Jogar sem contar tempo' }))
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()

    await act(async () => {
      pending.resolve(session({ gameId: 10, state: 'running' }))
      await pending.promise
    })
  })

  it('UIX-031/ended de G relê GetGame(G) e GameSessions(G) uma vez e o painel mostra o total novo; sessão de outro jogo não relê G', async () => {
    let total = 0
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    app.GetGame.mockImplementation(() => Promise.resolve(detail({ id: 10, title: 'Hollow Knight', totalPlaySeconds: total })))
    renderApp()
    await waitConnected()
    const aside = await focusGame('Hollow Knight')
    expect(within(aside).getByText('Nunca jogado')).toBeInTheDocument()
    await waitFor(() => expect(app.GameSessions).toHaveBeenCalled())
    await flush()
    const reads = app.GetGame.mock.calls.length
    const sessionReads = app.GameSessions.mock.calls.length

    total = 600
    emit('session:changed', session({ gameId: 10, state: 'ended', endedAt: '2026-11-09T10:10:00.000Z', activeMs: 600000 }))
    await waitFor(() => expect(within(asideOf()).getByText('10 min')).toBeInTheDocument())
    await flush()
    expect(app.GetGame.mock.calls.length).toBe(reads + 1)
    expect(app.GameSessions.mock.calls.length).toBe(sessionReads + 1)

    emit('session:changed', session({ sessionId: 2, gameId: 20, title: 'Hades', state: 'ended', endedAt: '2026-11-09T10:20:00.000Z' }))
    await flush()
    expect(app.GetGame.mock.calls.length).toBe(reads + 1)
    expect(app.GameSessions.mock.calls.length).toBe(sessionReads + 1)
  })

  // alterado em 2026-11-15 (phase-12-metadados-unificados, T34): fluxo novo, Obter dados + MetadataSearch/MetadataApply [MET-043].
  it('UIX-033/MET-043/Buscar metadados abre o diálogo de G; depois de aplicar, o painel mostra os dados novos e MediaList(G) é lido de novo', async () => {
    let saved = detail({ title: 'Hollow Knight', description: 'Antiga.' })
    const candidate = {
      refs: [{ provider: 'igdb', providerId: '7' }],
      title: 'Hollow Knight',
      year: 2017,
      platform: 'PC',
      score: 0.95,
    }
    app.ListGames.mockResolvedValue(page([game()]))
    app.GetGame.mockImplementation(() => Promise.resolve(saved))
    app.MetadataSearch.mockResolvedValue({ candidates: [candidate], confidence: 'match', warnings: [] })
    app.MetadataApply.mockImplementation(() => {
      saved = { ...saved, description: 'Descrição nova do provedor.' }
      return Promise.resolve({ game: saved, queued: 1, skippedByLimit: 0 })
    })
    renderApp()
    const aside = await focusGame('Hollow Knight')
    expect(within(aside).getByText('Antiga.')).toBeInTheDocument()

    fireEvent.click(within(aside).getByRole('button', { name: 'Buscar metadados' }))
    const dialog = await screen.findByRole('dialog', { name: 'Buscar metadados e mídia' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Obter dados' }))
    await waitFor(() => expect(app.MetadataSearch).toHaveBeenCalledWith('Hollow Knight', 'Windows', 0))
    const apply = await within(dialog).findAllByRole('button', { name: 'Aplicar' })
    await flush()
    const mediaReads = app.MediaList.mock.calls.length

    fireEvent.click(apply[0])
    await waitFor(() => expect(app.MetadataApply).toHaveBeenCalledWith(10, candidate.refs, false))
    await waitFor(() => expect(within(asideOf()).getByText('Descrição nova do provedor.')).toBeInTheDocument())
    await waitFor(() => expect(app.MediaList.mock.calls.length).toBeGreaterThan(mediaReads))
    expect(app.MediaList).toHaveBeenLastCalledWith(10)
  })

  it('UIX-045/escolher G nas estatísticas volta à biblioteca com os filtros e G no painel', async () => {
    threePlatformLibrary()
    app.StatsSummary.mockResolvedValue({
      games: [
        { gameId: 20, title: 'Hades', platform: 'Windows', sessions: 1, activeSeconds: 3600, importedSeconds: 0, lastPlayedAt: '' },
      ],
      platforms: [],
      totalSeconds: 3600,
      importedSeconds: 0,
    })
    renderApp()
    await findRow('Hollow Knight')
    fireEvent.click(screen.getByRole('button', { name: 'Windows, 2 jogos' }))
    await waitFor(() =>
      expect(app.ListGames).toHaveBeenLastCalledWith(expect.objectContaining({ filter: { version: 1, platformIds: [1] } })),
    )
    await findRow('Hades')

    fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Hades' }))

    // The page closes, the filters are the same and Hades is in the panel and marked.
    expect(await within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Estatísticas' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Windows, 2 jogos' })).toHaveAttribute('aria-current', 'true')
    await findRow('Hades')
    await waitFor(() => expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'true'))
    expect(app.GetGame).toHaveBeenCalledWith(20)
  })

  it('UI-038/o painel do jogo mostra a seção Sessões com as sessões do jogo', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()

    await focusGame('Hollow Knight')
    await waitFor(() => expect(app.GameSessions).toHaveBeenCalledWith(10, 0, expect.any(Number)))
    expect(within(asideOf()).getByRole('heading', { name: 'Sessões' })).toBeInTheDocument()
  })

  it('UIX-032/Editar abre "Editar jogo" e envia só o título alterado; Salvar com sucesso fecha e o painel mostra o detalhe', async () => {
    let saved = detail({ title: 'Hollow Knight' })
    app.ListGames.mockResolvedValue(page([game()]))
    app.GetGame.mockImplementation(() => Promise.resolve(saved))
    app.UpdateGame.mockImplementation((_id: number, patch: Partial<GameDetailView>) => {
      saved = { ...saved, ...patch }
      return Promise.resolve(saved)
    })
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(dialog).getByLabelText('Título'), { target: { value: 'Hollow Knight II' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(app.UpdateGame).toHaveBeenCalledTimes(1))
    expect(app.UpdateGame).toHaveBeenCalledWith(10, { title: 'Hollow Knight II' })
    expect(await within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight II' })).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(screen.queryByLabelText('Título')).toBeNull()
  })

  it('UIX-032/Cancelar descarta as alterações sem chamar UpdateGame', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(dialog).getByLabelText('Título'), { target: { value: 'Outro nome' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(app.UpdateGame).not.toHaveBeenCalled()
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
  })

  it('UIX-032/erro de UpdateGame mostra a mensagem e mantém o modal com os valores digitados', async () => {
    const msg = 'Não foi possível salvar o jogo. Tente de novo.'
    app.ListGames.mockResolvedValue(page([game()]))
    app.UpdateGame.mockRejectedValue(msg)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(dialog).getByLabelText('Título'), { target: { value: 'Hollow Knight II' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))

    expect(await within(dialog).findByRole('alert')).toHaveTextContent(msg)
    expect(screen.getByRole('dialog', { name: 'Editar jogo' })).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Título')).toHaveValue('Hollow Knight II')
    expect(within(dialog).getByRole('button', { name: 'Salvar' })).toBeEnabled()
  })

  it('UIX-032/Esc durante o salvamento não fecha; sucesso fecha e o painel mostra o detalhe devolvido', async () => {
    const pending = deferred<GameDetailView>()
    app.ListGames.mockResolvedValue(page([game()]))
    app.UpdateGame.mockImplementation(() => pending.promise)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(dialog).getByLabelText('Título'), { target: { value: 'Hollow Knight II' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(app.UpdateGame).toHaveBeenCalledTimes(1))

    fireEvent.keyDown(dialog, { key: 'Escape' })
    await flush()
    expect(screen.getByRole('dialog', { name: 'Editar jogo' })).toBeInTheDocument()

    await act(async () => {
      pending.resolve(detail({ title: 'Hollow Knight II' }))
    })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(await within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight II' })).toBeInTheDocument()
  })

  it('UIX-032/Esc durante o salvamento não fecha; erro mantém o modal com os valores e a mensagem', async () => {
    const msg = 'Não foi possível salvar o jogo. Tente de novo.'
    const pending = deferred<GameDetailView>()
    app.ListGames.mockResolvedValue(page([game()]))
    app.UpdateGame.mockImplementation(() => pending.promise)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(dialog).getByLabelText('Título'), { target: { value: 'Hollow Knight II' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(app.UpdateGame).toHaveBeenCalledTimes(1))

    fireEvent.keyDown(dialog, { key: 'Escape' })
    await flush()
    expect(screen.getByRole('dialog', { name: 'Editar jogo' })).toBeInTheDocument()

    await act(async () => {
      pending.reject(msg)
    })
    const again = screen.getByRole('dialog', { name: 'Editar jogo' })
    expect(await within(again).findByRole('alert')).toHaveTextContent(msg)
    expect(within(again).getByLabelText('Título')).toHaveValue('Hollow Knight II')
  })

  // The panel only offers "Editar" with the game loaded, so the read is taken away after the click
  // (the queries are reset) to leave the open modal without its game.
  it('UIX-032/Editar com GetGame pendente abre o modal com "Carregando jogo…" e depois o formulário', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    const qc = renderApp()
    const aside = await focusGame('Hollow Knight')

    const read = deferred<GameDetailView>()
    app.GetGame.mockImplementation(() => read.promise)
    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    act(() => {
      void qc.resetQueries({ queryKey: [] })
    })
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    expect(within(dialog).getByText('Carregando jogo…')).toBeInTheDocument()
    expect(within(dialog).queryByLabelText('Título')).toBeNull()

    await act(async () => {
      read.resolve(detail())
    })
    expect(await within(dialog).findByLabelText('Título')).toHaveValue('Hollow Knight')
    expect(within(dialog).queryByText('Carregando jogo…')).toBeNull()
  })

  it('UIX-032/Editar com GetGame em erro abre o modal com a mensagem de erro', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    const qc = renderApp()
    const aside = await focusGame('Hollow Knight')

    app.GetGame.mockRejectedValue('Falha ao ler o jogo.')
    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    act(() => {
      void qc.resetQueries({ queryKey: [] })
    })
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Falha ao ler o jogo.')
    expect(within(dialog).queryByLabelText('Título')).toBeNull()
  })

  it('UIX-032/erro de edição de A não aparece no modal de edição de B (GameDialogs reinicia por jogo)', async () => {
    const msg = 'Não foi possível salvar o jogo. Tente de novo.'
    const a = game({ id: 10, title: 'Hollow Knight' })
    const b = game({ id: 20, title: 'Hades' })
    app.ListGames.mockResolvedValue(page([a, b]))
    app.GetGame.mockImplementation((id: number) => Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Hades' })))
    app.UpdateGame.mockRejectedValue(msg)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(dialog).getByLabelText('Título'), { target: { value: 'Outro' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(msg)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await focusGame('Hades')
    fireEvent.click(within(asideOf()).getByRole('button', { name: 'Editar' }))
    const dialogB = await screen.findByRole('dialog', { name: 'Editar jogo' })
    expect(within(dialogB).queryByRole('alert')).toBeNull()
    expect(within(dialogB).queryByText(msg)).toBeNull()
    expect(within(dialogB).getByLabelText('Título')).toHaveValue('Hades')
  })

  it('UI-040/título vazio desabilita Salvar e nada é enviado ao agente', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(dialog).getByLabelText('Título'), { target: { value: '   ' } })
    expect(within(dialog).getByRole('button', { name: 'Salvar' })).toBeDisabled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    await flush()
    expect(app.UpdateGame).not.toHaveBeenCalled()
  })

  it('UI-041/Remover abre a confirmação com o texto de UI-041; Cancelar não chama DeleteGame', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Remover' }))
    const dialog = await screen.findByRole('dialog', { name: 'Remover jogo' })
    expect(
      within(dialog).getByText(
        'Remover «Hollow Knight»? As sessões e as pausas deste jogo também serão apagadas. Esta ação não pode ser desfeita.',
      ),
    ).toBeInTheDocument()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(app.DeleteGame).not.toHaveBeenCalled()
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
  })

  it('UIX-034/Remover abre a confirmação; sucesso esvazia o painel e a grade é recarregada sem G', async () => {
    let library = [game()]
    app.ListGames.mockImplementation(() => Promise.resolve(page(library)))
    app.DeleteGame.mockImplementation(() => {
      library = []
      return Promise.resolve()
    })
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Remover' }))
    const dialog = await screen.findByRole('dialog', { name: 'Remover jogo' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remover' }))

    expect(await screen.findByText('Nenhum jogo ainda')).toBeInTheDocument()
    expect(app.DeleteGame).toHaveBeenCalledTimes(1)
    expect(app.DeleteGame).toHaveBeenCalledWith(10)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(await within(asideOf()).findByText('Nenhum jogo selecionado')).toBeInTheDocument()
    expect(within(asideOf()).queryByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeNull()
    expect(cards()).toHaveLength(0)
  })

  it('UIX-034/-32002: aviso de DSK-018 no topo, a confirmação fecha e G continua em foco', async () => {
    const msg = 'Não é possível remover: o jogo está em uma sessão ativa. Feche a sessão antes de remover.'
    app.ListGames.mockResolvedValue(page([game()]))
    app.DeleteGame.mockRejectedValue(msg)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Remover' }))
    const dialog = await screen.findByRole('dialog', { name: 'Remover jogo' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remover' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(msg)
    expect(within(alert).getByRole('button', { name: 'Fechar' })).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).toBeNull()
    const heading = within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })
    expect(precedes(alert, heading)).toBe(true)
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
  })

  it('UI-041/Remover fica desabilitado enquanto DeleteGame está pendente', async () => {
    const pending = deferred<void>()
    app.ListGames.mockResolvedValue(page([game()]))
    app.DeleteGame.mockImplementation(() => pending.promise)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Remover' }))
    const dialog = await screen.findByRole('dialog', { name: 'Remover jogo' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remover' }))

    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Remover' })).toBeDisabled())
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remover' }))
    await flush()
    expect(app.DeleteGame).toHaveBeenCalledTimes(1)

    await act(async () => {
      pending.resolve()
      await pending.promise
    })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })
})

// ---------------------------------------------------------------------------
// UI-042 reload after a write (integration)
// ---------------------------------------------------------------------------

describe('App: atualização depois de escrita (UI-042)', () => {
  it('UI-042/após UpdateGame e DeleteGame a grade e o catálogo são recarregados uma vez', async () => {
    let title = 'Hollow Knight'
    let library: GameView[] = [game({ title })]
    app.ListGames.mockImplementation(() => Promise.resolve(page(library)))
    app.GetGame.mockImplementation(() => Promise.resolve(detail({ title })))
    app.UpdateGame.mockImplementation((_id: number, patch: Partial<GameDetailView>) => {
      title = patch.title ?? title
      library = [game({ title })]
      return Promise.resolve(detail({ title }))
    })
    app.DeleteGame.mockImplementation(() => {
      library = []
      return Promise.resolve()
    })
    renderApp()
    await findRow('Hollow Knight')
    await waitFor(() => expect(app.Catalog).toHaveBeenCalledTimes(1))
    await flush()
    const lists0 = pageCalls().length
    const catalog0 = app.Catalog.mock.calls.length

    // Update: the modal saves and the grid is reloaded once, the panel stays on the game.
    let aside = await focusGame('Hollow Knight')
    fireEvent.click(within(aside).getByRole('button', { name: 'Editar' }))
    const editDialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    fireEvent.change(within(editDialog).getByLabelText('Título'), { target: { value: 'Hollow Knight II' } })
    fireEvent.click(within(editDialog).getByRole('button', { name: 'Salvar' }))
    await waitFor(() => expect(app.UpdateGame).toHaveBeenCalledTimes(1))
    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hollow Knight II' })
    await findRow('Hollow Knight II')
    await flush()
    expect(pageCalls().length).toBe(lists0 + 1)
    expect(app.Catalog.mock.calls.length).toBe(catalog0 + 1)

    // Delete: the game leaves the grid, and the lists are reloaded once more.
    aside = asideOf()
    fireEvent.click(within(aside).getByRole('button', { name: 'Remover' }))
    const dialog = await screen.findByRole('dialog', { name: 'Remover jogo' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remover' }))
    expect(await screen.findByText('Nenhum jogo ainda')).toBeInTheDocument()
    await flush()
    expect(app.DeleteGame).toHaveBeenCalledTimes(1)
    expect(pageCalls().length).toBe(lists0 + 2)
    expect(app.Catalog.mock.calls.length).toBe(catalog0 + 2)
  })
})

describe('App: importação recarrega o catálogo (PLT-032)', () => {
  it('PLT-032/depois de importar o catálogo é recarregado e a plataforma com jogos aparece no filtro', async () => {
    const extra = {
      ImportScan: vi.fn(() =>
        Promise.resolve({
          candidates: [
            {
              source: 'steam', title: 'Counter-Strike 2', platform: 'Windows', launchType: 'uri',
              launchTarget: 'steam://rungameid/730', installDir: '', developers: [], publishers: [], genres: [],
              year: null, description: '', playModes: [], playedSeconds: null, lastPlayed: null, exists: false,
            },
          ],
          warnings: [],
        }),
      ),
      ImportApply: vi.fn(() => Promise.resolve({ applied: [{ gameId: 11, title: 'Counter-Strike 2' }], skipped: [] })),
    }
    Object.assign(app, extra)
    app.ListGames.mockResolvedValue(page([game()]))
    renderApp()
    await findRow('Hollow Knight')
    await waitFor(() => expect(app.Catalog).toHaveBeenCalledTimes(1))
    await flush()
    const catalog0 = app.Catalog.mock.calls.length

    fireEvent.click(screen.getByRole('button', { name: 'Importar' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Buscar' }))
    // IMV-041: the row comes already checked; no click on it.
    await within(dialog).findByRole('checkbox', { name: /Counter-Strike 2/ })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Importar selecionados' }))
    await waitFor(() => expect(extra.ImportApply).toHaveBeenCalledTimes(1))
    await within(dialog).findByText('Importados: 1')
    await flush()

    expect(app.Catalog.mock.calls.length).toBeGreaterThan(catalog0)
  })
})

// ---------------------------------------------------------------------------
// UI-008 visible errors and pending Jogar
// ---------------------------------------------------------------------------

describe('App: erros', () => {
  const launchMsg = 'Já há um jogo em andamento: Celeste. Feche-o antes de iniciar outro.'

  it('UI-008 falha de Jogar mostra a mensagem no aviso', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.LaunchGame.mockRejectedValue(launchMsg)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Jogar' }))
    const msg = await screen.findByText(launchMsg)
    expect(app.LaunchGame).toHaveBeenCalledWith(10)
    // The notice is at the top of the grid.
    expect(precedes(msg, rowOf('Hollow Knight'))).toBe(true)
    // The button is back to Jogar after the failure.
    expect(within(asideOf()).getByRole('button', { name: 'Jogar' })).toBeEnabled()

    fireEvent.click(screen.getByRole('button', { name: 'Fechar' }))
    await waitFor(() => expect(screen.queryByText(launchMsg)).toBeNull())
    expect(screen.queryByRole('button', { name: 'Fechar' })).toBeNull()
  })

  it('UI-008 Jogar pendente mostra Iniciando… desabilitado no painel', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    const pending = deferred<SessionView>()
    app.LaunchGame.mockImplementation(() => pending.promise)
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Jogar' }))
    await waitFor(() => {
      const btn = within(asideOf()).getByRole('button', { name: 'Iniciando…' })
      expect(btn).toHaveAccessibleName('Iniciando…')
      expect(btn).toBeDisabled()
    })

    await act(async () => {
      pending.resolve(session({ gameId: 10, state: 'running' }))
      await pending.promise
    })
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Iniciando…' })).toBeNull())
  })

  it('UI-008 ação bem-sucedida remove o aviso', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.LaunchGame.mockRejectedValueOnce(launchMsg).mockResolvedValueOnce(session({ gameId: 10 }))
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Jogar' }))
    await screen.findByText(launchMsg)

    await waitFor(() => expect(within(asideOf()).getByRole('button', { name: 'Jogar' })).toBeEnabled())
    fireEvent.click(within(asideOf()).getByRole('button', { name: 'Jogar' }))
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
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    app.CurrentSession.mockResolvedValue(session({ gameId: 20, title: 'Hades' }))
    renderApp()
    await findRow('Hollow Knight')

    await waitFor(() => expect(rowOf('Hades')).toHaveTextContent('Em execução'))
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')
  })

  it('UI-006 ended remove a marcação e recarrega a lista', async () => {
    // The list rows show the play time; the cards of the grid do not [UIX-014, UIX-020].
    writeLibraryView('list')
    let total = 0
    app.ListGames.mockImplementation(() => Promise.resolve(page([game({ id: 10, totalPlaySeconds: total })])))
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
    app.ListGames.mockResolvedValue(page([game()]))
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
    app.ListGames.mockResolvedValue(page([game()]))
    app.CurrentSession.mockResolvedValue(null)
    renderApp()
    await waitConnected()
    await findRow('Hollow Knight')
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalled())
    await act(async () => {
      await Promise.resolve()
    })
    expect(queryPanel()).toBeNull()

    // A running event makes it appear above the grid, with the session title.
    emit('session:changed', session({ gameId: 10, title: 'Hollow Knight', activeMs: 252400 }))
    const panel = await screen.findByRole('status', { name: panelName })
    expect(panel).toHaveTextContent('Hollow Knight')
    expect(panel).toHaveTextContent('0:04:1')
    expect(precedes(panel, rowOf('Hollow Knight'))).toBe(true)
  })

  it('UI-012 LaunchGame bem-sucedido mostra o painel', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.LaunchGame.mockResolvedValue(session({ gameId: 10, title: 'Hollow Knight', activeMs: 0 }))
    renderApp()
    await waitConnected()
    const aside = await focusGame('Hollow Knight')
    fireEvent.click(within(aside).getByRole('button', { name: 'Jogar' }))

    const panel = await screen.findByRole('status', { name: panelName })
    expect(panel).toHaveTextContent('Hollow Knight')
    expect(panel).toHaveTextContent('0:00:0')
    await waitFor(() => expect(rowOf('Hollow Knight')).toHaveTextContent('Em execução'))
  })

  it('UI-012 discarded remove o painel', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
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
    app.ListGames.mockResolvedValue(page([game()]))
    app.CurrentSession.mockResolvedValue(session({ gameId: 10, activeMs: 60000 }))
    renderApp()
    await waitConnected()
    await screen.findByRole('status', { name: panelName })

    await disconnect()
    await waitFor(() => expect(queryPanel()).toBeNull())
  })

  it('UI-016 ao reconectar o painel volta só com o valor recarregado', async () => {
    app.ListGames.mockResolvedValue(page([game()]))
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
    app.ListGames.mockResolvedValue(page([game()]))
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
    app.ListGames.mockResolvedValue(page([game()]))
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
    writeLibraryView('list')
    vi.useFakeTimers({ toFake: [...fakeTimerApis] })
    let total = 300
    app.ListGames.mockImplementation(() => Promise.resolve(page([game({ id: 10, totalPlaySeconds: total })])))
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
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
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

// untrackedBtn is "Jogar sem contar tempo" of the panel of the game in focus [UIX-027].
function untrackedBtn(): HTMLElement {
  return within(asideOf()).getByRole('button', { name: /^(Jogar sem contar tempo|Iniciando…)$/ })
}

describe('App: sem contar tempo', () => {
  it('UI-020/Sem contar tempo chama LaunchGameUntracked(G) uma vez e não LaunchGame; sem painel, sem Em execução, sem recarregar a lista', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.CurrentSession.mockResolvedValue(null)
    app.LaunchGameUntracked.mockResolvedValue(undefined)
    renderApp()
    await waitConnected()
    await focusGame('Hollow Knight')
    await waitFor(() => expect(app.CurrentSession).toHaveBeenCalled())
    await waitFor(() => expect(screen.queryByText('Conectando ao agente…')).toBeNull())
    await flush()
    const listCalls = app.ListGames.mock.calls.length

    fireEvent.click(within(asideOf()).getByRole('button', { name: 'Jogar sem contar tempo' }))
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
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.LaunchGameUntracked.mockRejectedValueOnce(msg).mockResolvedValueOnce(undefined)
    renderApp()
    await focusGame('Hollow Knight')

    fireEvent.click(untrackedBtn())
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(msg)
    expect(within(alert).getByRole('button', { name: 'Fechar' })).toBeInTheDocument()
    expect(app.LaunchGame).not.toHaveBeenCalled()

    await waitFor(() => expect(untrackedBtn()).toBeEnabled())
    fireEvent.click(untrackedBtn())
    await waitFor(() => expect(app.LaunchGameUntracked).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
    expect(screen.queryByRole('button', { name: 'Fechar' })).toBeNull()
  })

  it('UI-021/lançamento pendente sem contar tempo desabilita os dois botões do jogo no App', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    const pending = deferred<void>()
    app.GetGame.mockImplementation((id: number) =>
      Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Hades' })),
    )
    app.LaunchGameUntracked.mockImplementation(() => pending.promise)
    renderApp()
    await focusGame('Hollow Knight')

    fireEvent.click(untrackedBtn())
    await waitFor(() => {
      const aside = asideOf()
      expect(within(aside).getByRole('button', { name: 'Iniciando…' })).toBeDisabled()
      expect(within(aside).getByRole('button', { name: 'Jogar' })).toBeDisabled()
    })

    // Another game in focus has its buttons active [UIX-027].
    const other = await focusGame('Hades')
    expect(within(other).getByRole('button', { name: 'Jogar' })).toBeEnabled()
    expect(within(other).getByRole('button', { name: 'Jogar sem contar tempo' })).toBeEnabled()

    await act(async () => {
      pending.resolve()
      await pending.promise
    })
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Iniciando…' })).toBeNull())
  })

  it('UI-022/painel carregado por CurrentSession em paused mostra Pausado', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
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
    writeLibraryView('list')
    let total = 0
    app.ListGames.mockImplementation(() => Promise.resolve(page([game({ id: 10, totalPlaySeconds: total })])))
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
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
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
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
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
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
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

describe('App: diálogo de plataformas', () => {
  it('PLT-023/o botão Plataformas do cabeçalho abre o diálogo e Fechar o fecha', async () => {
    renderApp()
    await waitConnected()

    fireEvent.click(screen.getByRole('button', { name: 'Plataformas' }))
    const dialog = await screen.findByRole('dialog', { name: 'Plataformas' })

    fireEvent.click(within(dialog).getByRole('button', { name: 'Fechar' }))
    expect(screen.queryByRole('dialog', { name: 'Plataformas' })).not.toBeInTheDocument()
  })
})

// tabStops lists the elements the Tab key visits, in document order (jsdom has no Tab navigation).
function tabStops(): HTMLElement[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>('a[href], button, input, select, textarea, [tabindex]'),
  ).filter((e) => e.tabIndex >= 0 && !e.hasAttribute('disabled'))
}

describe('App: teclado (T19)', () => {
  it('UIX-041/Tab percorre cabeçalho → coluna esquerda (busca, plataformas, filtros) → barra do centro → grade (uma parada) → painel', async () => {
    threePlatformLibrary()
    renderApp()
    await focusGame('Hollow Knight')

    const stops = tabStops()
    const nav = screen.getByRole('navigation', { name: 'Filtros da biblioteca' })
    const center = screen.getByRole('region', { name: 'Jogos' })
    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    const aside = asideOf()
    const header = document.querySelector<HTMLElement>('main > header') as HTMLElement

    const indexIn = (container: HTMLElement) =>
      stops.map((s, i) => (container.contains(s) ? i : -1)).filter((i) => i >= 0)
    const headerStops = indexIn(header)
    const navStops = indexIn(nav)
    const gridStops = stops.map((s, i) => (listbox.contains(s) ? i : -1)).filter((i) => i >= 0)
    const asideStops = indexIn(aside)
    const toolbarStops = stops
      .map((s, i) => (center.contains(s) && !listbox.contains(s) ? i : -1))
      .filter((i) => i >= 0)

    expect(headerStops.length).toBeGreaterThan(0)
    expect(navStops.length).toBeGreaterThan(0)
    expect(toolbarStops.length).toBeGreaterThan(0)
    expect(asideStops.length).toBeGreaterThan(0)
    // The grid is a single stop: the card of the game in focus.
    expect(gridStops).toHaveLength(1)
    expect(stops[gridStops[0]]).toBe(rowOf('Hollow Knight'))

    expect(Math.max(...headerStops)).toBeLessThan(Math.min(...navStops))
    // Inside the left column: search first, then platforms, then filters.
    const search = nav.querySelector('input')
    expect(search).not.toBeNull()
    expect(stops.indexOf(search as HTMLElement)).toBe(navStops[0])
    const todos = within(nav).getByRole('button', { name: /^Todos/ })
    const gran = within(nav).getByRole('button', { name: /PlayStation 2/ })
    expect(stops.indexOf(todos)).toBeLessThan(stops.indexOf(gran))
    expect(stops.indexOf(search as HTMLElement)).toBeLessThan(stops.indexOf(todos))
    expect(Math.max(...navStops)).toBeLessThan(Math.min(...toolbarStops))
    expect(Math.max(...toolbarStops)).toBeLessThan(gridStops[0])
    expect(gridStops[0]).toBeLessThan(Math.min(...asideStops))
  })

  it('UIX-043/pôr em foco e lançar só pelo teclado, sem mouse', async () => {
    app.ListGames.mockResolvedValue(
      page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]),
    )
    app.GetGame.mockImplementation((id: number) =>
      Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Hades' })),
    )
    renderApp()
    const first = await findRow('Hollow Knight')

    // Tab lands on the first card; the arrow puts the next game in focus.
    first.focus()
    expect(first).toHaveFocus()
    fireEvent.keyDown(first, { key: 'ArrowRight' })

    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'true')
    expect(rowOf('Hades')).toHaveFocus()
    expect(app.LaunchGame).not.toHaveBeenCalled()

    fireEvent.keyDown(rowOf('Hades'), { key: 'Enter' })
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    expect(app.LaunchGame).toHaveBeenCalledWith(20)
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
  })

  it('UIX-019/Enter com lançamento pendente: o App não chama LaunchGame de novo', async () => {
    app.ListGames.mockResolvedValue(
      page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]),
    )
    app.GetGame.mockImplementation((id: number) =>
      Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Hades' })),
    )
    const pending = deferred<SessionView>()
    app.LaunchGame.mockImplementation(() => pending.promise)
    renderApp()
    const first = await findRow('Hollow Knight')

    first.focus()
    fireEvent.keyDown(first, { key: 'Enter' })
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))

    fireEvent.keyDown(first, { key: 'ArrowRight' })
    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })
    fireEvent.keyDown(rowOf('Hades'), { key: 'Enter' })
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()

    await act(async () => {
      pending.resolve(session({ gameId: 10, state: 'running' }))
      await pending.promise
    })
  })
})

// ---------------------------------------------------------------------------
// Live images: media:changed [IMG-007, IMG-008, IMG-011] (phase 10.2, T10)
// ---------------------------------------------------------------------------

type MediaItem = { id: number; kind: string; provider: string; path: string; thumbPath: string; url: string; thumbUrl: string }

function mediaItem(id: number, kind = 'cover'): MediaItem {
  return { id, kind, provider: 'x', path: 'p', thumbPath: 't', url: `/m/${id}.jpg`, thumbUrl: `/m/${id}-t.jpg` }
}

// mediaByGame is what the fake agent holds; MediaList answers from it.
function serveMedia(byGame: Record<number, MediaItem[]>) {
  app.MediaList.mockImplementation((id: number) => Promise.resolve({ media: byGame[id] ?? [] }))
}

function cardImg(title: string): HTMLImageElement {
  return rowOf(title).querySelector('img') as HTMLImageElement
}

function mediaListIds(): number[] {
  return app.MediaList.mock.calls.map((c) => c[0] as number)
}

describe('App: imagens ao vivo (media:changed)', () => {
  it('IMG-007/App montado tem exatamente um handler de media:changed; desmontar o remove', async () => {
    const qc = newClient()
    const view = render(<App />, { wrapper: wrapperOf(qc) })
    await waitConnected()
    expect(handlers.get('media:changed')?.size).toBe(1)

    view.unmount()
    expect(handlers.get('media:changed')?.size ?? 0).toBe(0)
  })

  it('IMG-008/grade com G em foco e H visível sem imagem: media:changed(H) põe a capa no cartão de H; foco, marcação, activeElement e scrollTop iguais', async () => {
    threePlatformLibrary()
    const media: Record<number, MediaItem[]> = {}
    serveMedia(media)
    renderApp()
    await focusGame('Hollow Knight')
    const gCard = rowOf('Hollow Knight')
    gCard.focus()
    scrollGridTo(0)
    const scroller = cards()[0].parentElement as HTMLElement
    const top = scroller.scrollTop
    await waitFor(() => expect(cardImg('Hades')).not.toHaveAttribute('data-fit'))

    media[20] = [mediaItem(5)]
    emit('media:changed', { gameId: 20 })

    await waitFor(() => expect(cardImg('Hades')).toHaveAttribute('src', '/m/5-t.jpg'))
    expect(cardImg('Hades')).toHaveAttribute('data-fit', 'contain')
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
    expect(selectedCards()).toHaveLength(1)
    expect(selectedCards()[0]).toBe(rowOf('Hollow Knight'))
    expect(document.activeElement).toBe(rowOf('Hollow Knight'))
    expect(scroller.scrollTop).toBe(top)
  })

  it('IMG-008/cartão de H com substituta passa para a capa quando ela chega', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 20, title: 'Hades' })]))
    const media: Record<number, MediaItem[]> = { 20: [mediaItem(1, 'screenshot')] }
    serveMedia(media)
    renderApp()
    await findRow('Hades')
    await waitFor(() => expect(cardImg('Hades')).toHaveAttribute('src', '/m/1-t.jpg'))
    expect(cardImg('Hades')).toHaveAttribute('data-fit', 'cover')

    media[20] = [mediaItem(1, 'screenshot'), mediaItem(2, 'cover')]
    emit('media:changed', { gameId: 20 })

    await waitFor(() => expect(cardImg('Hades')).toHaveAttribute('src', '/m/2-t.jpg'))
    expect(cardImg('Hades')).toHaveAttribute('data-fit', 'contain')
  })

  it('IMG-008/media:changed(H) não relê ListGames nem MediaList de outras linhas', async () => {
    threePlatformLibrary()
    serveMedia({})
    renderApp()
    await focusGame('Hollow Knight')
    await flush()
    const lists = app.ListGames.mock.calls.length
    const games = app.GetGame.mock.calls.length
    const reads = app.MediaList.mock.calls.length

    emit('media:changed', { gameId: 20 })
    await waitFor(() => expect(app.MediaList.mock.calls.length).toBeGreaterThan(reads))
    await flush()

    expect(mediaListIds().slice(reads).every((id) => id === 20)).toBe(true)
    expect(app.ListGames.mock.calls.length).toBe(lists)
    expect(app.GetGame.mock.calls.length).toBe(games)
  })

  it('IMG-008/H = G: o cartão em foco troca a imagem e continua em foco', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' })]))
    const media: Record<number, MediaItem[]> = {}
    serveMedia(media)
    renderApp()
    await focusGame('Hollow Knight')
    rowOf('Hollow Knight').focus()

    media[10] = [mediaItem(7)]
    emit('media:changed', { gameId: 10 })

    await waitFor(() => expect(cardImg('Hollow Knight')).toHaveAttribute('src', '/m/7-t.jpg'))
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
    expect(document.activeElement).toBe(rowOf('Hollow Knight'))
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
  })

  it('IMG-008/lista e Big Box: o mesmo cartão atualiza sem mudar o foco', async () => {
    layoutBigBox()
    bigBoxLibrary()
    const media: Record<number, MediaItem[]> = {}
    serveMedia(media)
    renderApp()
    await focusGame('Hollow Knight')

    // List mode: the foco and the marking stay; the image shows when the grid comes back.
    fireEvent.click(screen.getByRole('button', { name: 'Mostrar em lista' }))
    await waitFor(() => expect(within(rowOf('Hollow Knight')).getByText('Windows')).toBeInTheDocument())
    media[20] = [mediaItem(3)]
    emit('media:changed', { gameId: 20 })
    await flush()
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
    fireEvent.click(screen.getByRole('button', { name: 'Mostrar em grade' }))
    await waitFor(() => expect(cardImg('Hades')).toHaveAttribute('src', '/m/3-t.jpg'))

    // Big Box: the focused card keeps the focus when another card changes.
    enterBigBoxByKey()
    rowOf('Hollow Knight').focus()
    media[30] = [mediaItem(4)]
    emit('media:changed', { gameId: 30 })
    await waitFor(() => expect(cardImg('Celeste')).toHaveAttribute('src', '/m/4-t.jpg'))
    expect(document.activeElement).toBe(rowOf('Hollow Knight'))
    expect(screen.getByRole('button', { name: /Sair do Big Box/ })).toBeInTheDocument()
    expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
  })

  // alterado em 2026-11-15 (phase-12-metadados-unificados, T34): fluxo novo, Obter dados + MetadataSearch/MetadataApply [MET-043].
  it('IMG-011/MET-043/MetadataDialog aberto, MetadataApply responde queued = 3, chegam três media:changed(G) com a lista crescendo: carrossel e cartão de G mostram as imagens a cada evento, G em foco, diálogo aberto', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    const media: Record<number, MediaItem[]> = {}
    serveMedia(media)
    const candidate = {
      refs: [{ provider: 'igdb', providerId: '7' }],
      title: 'Hollow Knight',
      year: 2017,
      platform: 'PC',
      score: 0.95,
    }
    app.MetadataSearch.mockResolvedValue({ candidates: [candidate], confidence: 'match', warnings: [] })
    app.MetadataApply.mockResolvedValue({ game: detail(), queued: 3, skippedByLimit: 0 })
    renderApp()
    const aside = await focusGame('Hollow Knight')

    fireEvent.click(within(aside).getByRole('button', { name: 'Buscar metadados' }))
    const dialog = await screen.findByRole('dialog', { name: 'Buscar metadados e mídia' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Obter dados' }))
    const apply = await within(dialog).findAllByRole('button', { name: 'Aplicar' })
    fireEvent.click(apply[0])
    await waitFor(() => expect(app.MetadataApply).toHaveBeenCalledWith(10, candidate.refs, false))
    await flush()

    const all = [mediaItem(1, 'screenshot'), mediaItem(2, 'screenshot'), mediaItem(3, 'screenshot')]
    for (let n = 1; n <= 3; n++) {
      media[10] = all.slice(0, n)
      emit('media:changed', { gameId: 10 })
      await waitFor(() => expect(within(asideOf()).getByText(`1 de ${n}`)).toBeInTheDocument())
      await waitFor(() => expect(cardImg('Hollow Knight')).toHaveAttribute('src', '/m/1-t.jpg'))
      expect(screen.getByRole('dialog', { name: 'Buscar metadados e mídia' })).toBeInTheDocument()
      expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hollow Knight' })).toBeInTheDocument()
      expect(rowOf('Hollow Knight')).toHaveAttribute('aria-selected', 'true')
    }
  })

  // alterado em 2026-11-15 (phase-12-metadados-unificados, T34): fluxo novo, Obter dados + MetadataSearch/MetadataApply [MET-043].
  it('IMG-011/MET-043/aplicar continua invalidando mediaKey(G)', async () => {
    app.ListGames.mockResolvedValue(page([game({ id: 10, title: 'Hollow Knight' })]))
    app.MetadataSearch.mockResolvedValue({
      candidates: [
        {
          refs: [{ provider: 'igdb', providerId: '7' }],
          title: 'Hollow Knight',
          year: 2017,
          platform: 'PC',
          score: 0.95,
        },
      ],
      confidence: 'match',
      warnings: [],
    })
    app.MetadataApply.mockResolvedValue({ game: detail(), queued: 3, skippedByLimit: 0 })
    const qc = renderApp()
    const aside = await focusGame('Hollow Knight')
    const invalidate = vi.spyOn(qc, 'invalidateQueries')

    fireEvent.click(within(aside).getByRole('button', { name: 'Buscar metadados' }))
    const dialog = await screen.findByRole('dialog', { name: 'Buscar metadados e mídia' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Obter dados' }))
    const apply = await within(dialog).findAllByRole('button', { name: 'Aplicar' })
    await flush()
    const reads = app.MediaList.mock.calls.length
    fireEvent.click(apply[0])

    await waitFor(() => expect(app.MediaList.mock.calls.length).toBeGreaterThan(reads))
    expect(app.MediaList).toHaveBeenLastCalledWith(10)
    expect(invalidate.mock.calls.some((c) => JSON.stringify((c[0] as { queryKey?: unknown })?.queryKey) === JSON.stringify(['media', 10]))).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// Big Box on the three-column layout (T20)
// ---------------------------------------------------------------------------

// layoutBigBox gives the items a screen position (jsdom has none): the left column at x = 0, the
// cards of the grid three per row from x = 300, the details panel at x = 900. Everything else keeps
// the fixed box, so the grid still computes its window from it.
function layoutBigBox() {
  rectSpy?.mockImplementation(function (this: HTMLElement) {
    const at = (left: number, top: number) =>
      ({ x: left, y: top, left, top, right: left + 100, bottom: top + 30, width: 100, height: 30, toJSON: () => ({}) }) as DOMRect
    const option = this.closest('[role="option"]')
    if (option) {
      const all = cards()
      const i = Math.max(all.indexOf(option as HTMLElement), 0)
      return at(300 + (i % 3) * 120, 100 + Math.floor(i / 3) * 200)
    }
    const nav = this.closest('nav')
    if (nav) {
      const list = Array.from(nav.querySelectorAll('[data-bigbox-item]'))
      return at(0, 20 + Math.max(list.indexOf(this), 0) * 40)
    }
    const aside = this.closest('aside')
    if (aside) {
      const list = Array.from(aside.querySelectorAll('[data-bigbox-item]'))
      return at(900, 20 + Math.max(list.indexOf(this), 0) * 40)
    }
    return {
      x: 0, y: 0, top: 0, left: 0, bottom: BOX_HEIGHT, right: BOX_WIDTH, width: BOX_WIDTH, height: BOX_HEIGHT, toJSON: () => ({}),
    } as DOMRect
  })
}

function enterBigBoxByKey() {
  fireEvent.keyDown(window, { key: 'F11' })
  expect(screen.getByRole('button', { name: /Sair do Big Box/ })).toBeInTheDocument()
}

function playButton(): HTMLElement {
  return within(asideOf()).getByRole('button', { name: 'Jogar' })
}

function bigBoxLibrary() {
  app.ListGames.mockResolvedValue(
    page([game({ id: 10, title: 'Hollow Knight' }), game({ id: 20, title: 'Hades' }), game({ id: 30, title: 'Celeste' })]),
  )
  app.GetGame.mockImplementation((id: number) =>
    Promise.resolve(detail({ id, title: ({ 10: 'Hollow Knight', 20: 'Hades', 30: 'Celeste' } as Record<number, string>)[id] })),
  )
}

describe('App: Big Box sobre o layout novo (T20)', () => {
  beforeEach(() => {
    layoutBigBox()
  })

  it('UIX-044/cada cartão é um item; direções movem cartão a cartão', async () => {
    bigBoxLibrary()
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    const all = cards()
    expect(all).toHaveLength(3)
    for (const c of all) {
      expect(c).toHaveAttribute('data-bigbox-item')
      expect(c).toHaveAttribute('data-bigbox-card')
      expect(c.querySelectorAll('[data-bigbox-item]')).toHaveLength(0)
    }
    all[0].focus()
    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(all[1]).toHaveFocus()
    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(all[2]).toHaveFocus()
    fireEvent.keyDown(window, { key: 'ArrowLeft' })
    expect(all[1]).toHaveFocus()
  })

  it('UIX-044/na borda direita da grade o foco vai ao painel e na borda esquerda à coluna esquerda', async () => {
    bigBoxLibrary()
    renderApp()
    await focusGame('Hollow Knight')
    await within(asideOf()).findByRole('button', { name: 'Jogar' })
    enterBigBoxByKey()

    const all = cards()
    all[2].focus()
    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(asideOf().contains(document.activeElement)).toBe(true)

    all[0].focus()
    fireEvent.keyDown(window, { key: 'ArrowLeft' })
    expect(screen.getByRole('navigation', { name: 'Filtros da biblioteca' }).contains(document.activeElement)).toBe(true)
  })

  it('UIX-044/busca, Todos, plataformas e filtros são itens navegáveis', async () => {
    threePlatformLibrary()
    renderApp()
    await findRow('Hollow Knight')
    const nav = screen.getByRole('navigation', { name: 'Filtros da biblioteca' })

    expect(nav.querySelector('input')).toHaveAttribute('data-bigbox-item')
    expect(within(nav).getByRole('button', { name: /^Todos/ })).toHaveAttribute('data-bigbox-item')
    expect(within(nav).getByRole('button', { name: 'Windows, 2 jogos' })).toHaveAttribute('data-bigbox-item')
    expect(within(nav).getByRole('button', { name: 'PlayStation 2, 1 jogos' })).toHaveAttribute('data-bigbox-item')
    // Header and controls of the filters.
    const controls = Array.from(nav.querySelectorAll('button[aria-expanded], select, input'))
    expect(controls.length).toBeGreaterThan(1)
    for (const c of controls) {
      expect(c).toHaveAttribute('data-bigbox-item')
    }
  })

  it('UIX-044/A ou Enter num cartão põe o jogo em foco e leva o foco a "Jogar" do painel', async () => {
    bigBoxLibrary()
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    rowOf('Hades').focus()
    fireEvent.keyDown(window, { key: 'Enter' })

    await within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })
    await waitFor(() => expect(playButton()).toHaveFocus())
    expect(playButton()).toHaveAttribute('data-bigbox-play')
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'true')
    await flush()
    expect(app.LaunchGame).not.toHaveBeenCalled()
  })

  it('UIX-044/Y ou Shift+Enter num cartão chama LaunchGame do jogo do cartão uma vez', async () => {
    bigBoxLibrary()
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    rowOf('Celeste').focus()
    fireEvent.keyDown(window, { key: 'Enter', shiftKey: true })

    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledTimes(1))
    expect(app.LaunchGame).toHaveBeenCalledWith(30)
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
  })

  it('UIX-044/B com foco no painel volta ao cartão do jogo em foco', async () => {
    bigBoxLibrary()
    renderApp()
    await focusGame('Hades')
    const jogar = await within(asideOf()).findByRole('button', { name: 'Jogar' })
    enterBigBoxByKey()

    jogar.focus()
    fireEvent.keyDown(window, { key: 'Escape' })

    expect(rowOf('Hades')).toHaveFocus()
    expect(screen.getByRole('button', { name: /Sair do Big Box/ })).toBeInTheDocument()
  })

  it('UIX-044/B nas estatísticas volta à biblioteca sem sair do Big Box', async () => {
    bigBoxLibrary()
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
    expect(await screen.findByRole('heading', { name: 'Estatísticas' })).toBeInTheDocument()
    fireEvent.keyDown(window, { key: 'Escape' })

    expect(screen.queryByRole('heading', { name: 'Estatísticas' })).toBeNull()
    await findRow('Hollow Knight')
    expect(screen.getByRole('button', { name: /Sair do Big Box/ })).toBeInTheDocument()
  })

  it('UIX-044/B com foco na grade sai do Big Box', async () => {
    bigBoxLibrary()
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    rowOf('Hades').focus()
    fireEvent.keyDown(window, { key: 'Escape' })

    expect(screen.queryByRole('button', { name: /Sair do Big Box/ })).toBeNull()
  })

  it('UIX-044/no Big Box uma seta move o foco uma vez só (sem o handler da grade)', async () => {
    bigBoxLibrary()
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    const all = cards()
    all[0].focus()
    // The event starts on the card, so it also bubbles through any handler of the grid.
    fireEvent.keyDown(all[0], { key: 'ArrowRight' })
    expect(all[1]).toHaveFocus()
    expect(all[2]).not.toHaveFocus()
  })

  it('UIX-045/no Big Box, escolher G nas estatísticas põe G em foco e o foco em "Jogar"', async () => {
    threePlatformLibrary()
    app.StatsSummary.mockResolvedValue({
      games: [
        { gameId: 20, title: 'Hades', platform: 'Windows', sessions: 1, activeSeconds: 3600, importedSeconds: 0, lastPlayedAt: '' },
      ],
      platforms: [],
      totalSeconds: 3600,
      importedSeconds: 0,
    })
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Hades' }))

    expect(await within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })).toBeInTheDocument()
    await waitFor(() => expect(playButton()).toHaveFocus())
    expect(screen.getByRole('button', { name: /Sair do Big Box/ })).toBeInTheDocument()
  })

  const titleOf = (id: number) => ({ 10: 'Hollow Knight', 20: 'Hades', 30: 'Celeste' } as Record<number, string>)[id]

  it('UIX-037/Big Box com Editar aberto: setas, Enter, Shift+Enter, Y e B não mudam o jogo em foco, não lançam e não saem do Big Box', async () => {
    bigBoxLibrary()
    renderApp()
    await focusGame('Hades')
    enterBigBoxByKey()

    fireEvent.click(within(asideOf()).getByRole('button', { name: 'Editar' }))
    const dialog = await screen.findByRole('dialog', { name: 'Editar jogo' })
    expect(dialog.contains(document.activeElement)).toBe(true)

    for (const init of [
      { key: 'ArrowRight' },
      { key: 'ArrowLeft' },
      { key: 'ArrowDown' },
      { key: 'ArrowUp' },
      { key: 'Enter' },
      { key: 'Enter', shiftKey: true },
      { key: 'y' },
      { key: 'Escape' },
    ]) {
      fireEvent.keyDown(window, init)
    }
    await flush()

    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /Sair do Big Box/ })).toBeInTheDocument()
    expect(within(asideOf()).getByRole('heading', { level: 2, name: 'Hades' })).toBeInTheDocument()
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'true')
    expect(dialog.contains(document.activeElement)).toBe(true)
  })

  it('UIX-044/A num cartão de G e depois clique em H antes de G carregar: o foco não vai a "Jogar" de H', async () => {
    bigBoxLibrary()
    const g = deferred<GameDetailView>()
    app.GetGame.mockImplementation((id: number) =>
      id === 20 ? g.promise : Promise.resolve(detail({ id, title: titleOf(id) })),
    )
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    rowOf('Hades').focus()
    fireEvent.keyDown(window, { key: 'Enter' })
    fireEvent.click(within(rowOf('Celeste')).getByText('Celeste'))
    await within(asideOf()).findByRole('heading', { level: 2, name: 'Celeste' })
    await within(asideOf()).findByRole('button', { name: 'Jogar' })
    await act(async () => {
      g.resolve(detail({ id: 20, title: 'Hades' }))
    })
    await flush()

    expect(document.activeElement?.hasAttribute('data-bigbox-play')).toBe(false)
  })

  it('UIX-044/A num cartão de G com GetGame em erro: o foco não vai a "Jogar" (nem depois de "Tentar de novo")', async () => {
    bigBoxLibrary()
    let fail = true
    app.GetGame.mockImplementation((id: number) =>
      id === 20 && fail ? Promise.reject('Falha ao ler o jogo.') : Promise.resolve(detail({ id, title: titleOf(id) })),
    )
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    rowOf('Hades').focus()
    fireEvent.keyDown(window, { key: 'Enter' })
    await within(asideOf()).findByRole('alert')
    await flush()
    expect(document.activeElement?.hasAttribute('data-bigbox-play')).toBe(false)

    fail = false
    fireEvent.click(within(asideOf()).getByRole('button', { name: 'Tentar de novo' }))
    await within(asideOf()).findByRole('button', { name: 'Jogar' })
    await flush()
    expect(document.activeElement?.hasAttribute('data-bigbox-play')).toBe(false)
  })

  it('UIX-045/no Big Box, escolher G nas estatísticas com GetGame(G) em -32001: o foco não vai a "Jogar"', async () => {
    threePlatformLibrary()
    app.GetGame.mockImplementation((id: number) =>
      id === 20 ? Promise.reject(gameNotFoundMessage) : Promise.resolve(detail({ id, title: titleOf(id) ?? 'Jogo' })),
    )
    app.StatsSummary.mockResolvedValue({
      games: [
        { gameId: 20, title: 'Hades', platform: 'Windows', sessions: 1, activeSeconds: 3600, importedSeconds: 0, lastPlayedAt: '' },
      ],
      platforms: [],
      totalSeconds: 3600,
      importedSeconds: 0,
    })
    renderApp()
    await findRow('Hollow Knight')
    enterBigBoxByKey()

    fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Hades' }))
    await flush()
    await flush()

    expect(document.activeElement?.hasAttribute('data-bigbox-play')).toBe(false)
  })

  it('UIX-022/entrar e sair do Big Box mantém G no painel', async () => {
    bigBoxLibrary()
    renderApp()
    await focusGame('Hades')
    const heading = () => within(asideOf()).findByRole('heading', { level: 2, name: 'Hades' })

    enterBigBoxByKey()
    await heading()
    expect(within(asideOf()).queryByText('Nenhum jogo selecionado')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /Sair do Big Box/ }))
    expect(screen.queryByRole('button', { name: /Sair do Big Box/ })).toBeNull()
    await heading()
    expect(rowOf('Hades')).toHaveAttribute('aria-selected', 'true')
  })
})
