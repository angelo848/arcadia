import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'

// UIX-046 (the UI never influences play time) and UIX-047 (no new binding).
// Every binding of window.go.main.App is a spy; any property read creates one, so a call to a
// binding that is not in the phase 9 list is seen too.

const dts = import.meta.glob('../wailsjs/go/main/App.d.ts', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>
// The phase 9 bindings: the generated App.d.ts (it did not change in phase 10).
const phase9 = new Set(
  Object.values(dts).flatMap((src) => Array.from(src.matchAll(/export function (\w+)\(/g), (m) => m[1])),
)

// Bindings that start a game, change the session or write anything.
const writes = [
  'LaunchGame',
  'LaunchGameUntracked',
  'PauseSession',
  'ResumeSession',
  'UpdateGame',
  'DeleteGame',
  'AddPlatform',
  'PlatformDelete',
  'PlatformSetEmulator',
  'PlatformSetIcon',
  'PlatformUpdate',
  'EmulatorAdd',
  'EmulatorDelete',
  'EmulatorUpdate',
  'ProviderSetKey',
  'ProviderDeleteKey',
  'ImportApply',
  'ScrapeApply',
  'MediaQueue',
  'MediaCancel',
  // alterado em 2026-11-22 (fase 15): escritas do tempo para zerar (TTB-030).
  'TimeToBeatRefresh',
  'TimeToBeatSearch',
  'TimeToBeatChoose',
  'TimeToBeatSetSettings',
]

type Spy = ReturnType<typeof vi.fn>
let spies: Map<string, Spy>
let overrides: Record<string, (...a: unknown[]) => unknown>
let rectSpy: ReturnType<typeof vi.spyOn> | undefined

class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

function game(id: number, title: string) {
  return {
    id,
    platformId: 1,
    title,
    platform: 'Windows',
    path: `C:\\Games\\${title}.exe`,
    status: 'not_played',
    favorite: false,
    year: null,
    totalPlaySeconds: 0,
    playCount: 0,
    firstPlayedAt: '',
    lastPlayedAt: '',
  }
}

const titles: Record<number, string> = { 10: 'Hollow Knight', 20: 'Hades', 30: 'Celeste' }

function session(state: 'running' | 'paused' = 'running') {
  return {
    sessionId: 1,
    gameId: 10,
    title: 'Hollow Knight',
    state,
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '',
    activeSeconds: 0,
    activeMs: 0,
    asOf: '2026-11-09T10:00:00.000Z',
  }
}

function defaults(name: string, args: unknown[]): unknown {
  switch (name) {
    case 'AgentStatus':
      return {
        connected: true,
        version: '0.0.0-test',
        pid: 1,
        startedAt: '2026-11-09T09:00:00Z',
        dataDir: 'C:\\data',
        uptimeSeconds: 0,
      }
    case 'AgentElevated':
      return true
    case 'Catalog':
      return {
        platforms: [
          { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 3 },
          { id: 2, name: 'PlayStation 2', category: 'console', emulatorId: null, seedKey: 'ps2', iconUrl: '', gameCount: 0 },
        ],
        genres: [],
        companies: [],
        playModes: [],
      }
    case 'CurrentSession':
      return null
    case 'ListGames': {
      const q = args[0] as { offset?: number; limit?: number }
      const rows = [game(10, 'Hollow Knight'), game(20, 'Hades'), game(30, 'Celeste')]
      const offset = q?.offset ?? 0
      return { games: rows.slice(offset, offset + (q?.limit ?? 100)), total: rows.length, offset }
    }
    case 'GetGame': {
      const id = args[0] as number
      return {
        ...game(id, titles[id] ?? 'Jogo'),
        description: '',
        completedAt: null,
        developers: [],
        publishers: [],
        genres: [],
        playModes: [],
        launchType: 'exe',
        launchArgs: '',
        sessionPlaySeconds: 0, // alterado em 2026-11-25 (fase 16)
        emulatorId: null,
      }
    }
    case 'GameSessions':
      return { sessions: [], total: 0, offset: 0 }
    case 'MediaList':
      return {
        media: [1, 2, 3].map((id) => ({
          id,
          kind: 'screenshot',
          provider: 'p',
          path: '',
          thumbPath: '',
          url: `/media/${id}.jpg`,
          thumbUrl: `/thumb/${id}.jpg`,
        })),
      }
    case 'LaunchGame':
      return session()
    case 'PauseSession':
      return session('paused')
    case 'ResumeSession':
      return session('running')
    case 'EmulatorList':
      return { emulators: [] }
    case 'ProviderKeyStatus':
      return { provider: args[0], configured: false }
    case 'StatsSummary':
      return {
        games: [
          { gameId: 20, title: 'Hades', platform: 'Windows', sessions: 1, activeSeconds: 3600, importedSeconds: 0, lastPlayedAt: '' },
        ],
        platforms: [],
        totalSeconds: 3600,
        importedSeconds: 0,
      }
    case 'StatsWeekly':
      return { weeks: [] }
    case 'ScrapeSearch':
      return { candidates: [] }
    default:
      return undefined
  }
}

function install() {
  spies = new Map()
  overrides = {}
  const target = new Proxy(
    {},
    {
      get(_t, prop) {
        if (typeof prop !== 'string' || prop === 'then') {
          return undefined
        }
        let spy = spies.get(prop)
        if (!spy) {
          spy = vi.fn((...args: unknown[]) => Promise.resolve((overrides[prop] ?? ((...a) => defaults(prop, a)))(...args)))
          spies.set(prop, spy)
        }
        return spy
      },
    },
  )
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: target } }
  w.runtime = { EventsOn: vi.fn(() => () => {}) }
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
  rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () =>
      ({ x: 0, y: 0, top: 0, left: 0, bottom: 600, right: 800, width: 800, height: 600, toJSON: () => ({}) }) as DOMRect,
  )
}

function uninstall() {
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
  rectSpy?.mockRestore()
  vi.unstubAllGlobals()
}

function calls(name: string): unknown[][] {
  return spies.get(name)?.mock.calls ?? []
}

function writeCalls(): string[] {
  return writes.filter((n) => calls(n).length > 0)
}

function renderApp() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  return render(<App />, { wrapper })
}

function cards(): HTMLElement[] {
  const listbox = screen.queryByRole('listbox', { name: 'Jogos' })
  return listbox ? within(listbox).queryAllByRole('option') : []
}

async function findCard(title: string): Promise<HTMLElement> {
  await waitFor(() => expect(cards().length).toBe(3))
  const card = cards().find((c) => within(c).queryAllByText(title).length > 0)
  expect(card, `card of ${title}`).toBeDefined()
  return card as HTMLElement
}

// topBar is the header with the title "Arcadia" and the modal buttons.
function topBar(): HTMLElement {
  return screen.getByRole('heading', { level: 1, name: 'Arcadia' }).closest('header') as HTMLElement
}

function aside(): HTMLElement {
  return screen.getByRole('complementary', { name: 'Detalhes do jogo' })
}

async function focusGame(title: string): Promise<void> {
  const card = await findCard(title)
  fireEvent.click(within(card).getByText(title))
  await within(aside()).findByRole('heading', { level: 2, name: title })
  await within(aside()).findByRole('button', { name: 'Jogar' })
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
  cleanup()
  uninstall()
})

describe('invariantes da UI', () => {
  it('sanidade: a lista de bindings da fase 9 foi lida do App.d.ts', () => {
    expect(phase9.has('LaunchGame')).toBe(true)
    expect(phase9.has('ListGames')).toBe(true)
    expect(phase9.size).toBeGreaterThan(30)
  })

  it('UIX-046/pôr em foco, setas, carrossel, plataformas, filtros, seção recolhível, abrir e fechar cada modal, entrar e sair do Big Box e estatísticas chamam só leituras', async () => {
    renderApp()
    await focusGame('Hollow Knight')

    // Arrows on the grid.
    const first = await findCard('Hollow Knight')
    first.focus()
    fireEvent.keyDown(first, { key: 'ArrowRight' })
    await within(aside()).findByRole('heading', { level: 2, name: 'Hades' })
    fireEvent.keyDown(await findCard('Hades'), { key: 'ArrowLeft' })

    // Carousel.
    const strip = within(aside()).queryByRole('group', { name: 'Miniaturas' })
    if (strip) {
      const thumbs = within(strip).getAllByRole('button')
      fireEvent.click(thumbs[thumbs.length - 1])
      fireEvent.click(within(strip).getAllByRole('button')[0])
    }

    // Platforms and the collapsible filters section.
    fireEvent.click(screen.getByRole('button', { name: 'Windows, 3 jogos' }))
    await flush()
    fireEvent.click(screen.getByRole('button', { name: /^Todos/ }))
    await flush()
    const filtros = screen.getByRole('button', { name: /^Filtros/ })
    fireEvent.click(filtros)
    fireEvent.click(screen.getByRole('button', { name: /^Filtros/ }))

    // Every modal: open and close with Escape.
    const openers = ['Provedores', 'Emuladores', 'Plataformas', 'Importar']
    for (const name of openers) {
      fireEvent.click(within(topBar()).getByRole('button', { name }))
      const dialog = await screen.findByRole('dialog')
      fireEvent.keyDown(dialog, { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    }
    for (const name of ['Editar', 'Buscar metadados', 'Remover']) {
      fireEvent.click(within(aside()).getByRole('button', { name }))
      const dialog = await screen.findByRole('dialog')
      fireEvent.keyDown(dialog, { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    }

    // Big Box in and out, and the statistics.
    // alterado em 2026-11-15 (fase 13): Big Box novo, entra pelo botão e sai com Escape (B).
    fireEvent.click(screen.getByRole('button', { name: 'Big Box' }))
    await waitFor(() => expect(document.querySelector('[data-bigbox-theme]')).not.toBeNull())
    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => expect(document.querySelector('[data-bigbox-theme]')).toBeNull())
    fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
    await screen.findByRole('heading', { name: 'Estatísticas' })
    fireEvent.keyDown(window, { key: 'Escape' })
    await flush()

    expect(writeCalls(), 'bindings que escrevem ou lançam').toEqual([])
  })

  describe('UIX-046/duplo clique, Enter, Jogar e Y chamam LaunchGame exatamente uma vez cada; Jogar sem contar tempo chama LaunchGameUntracked uma vez', () => {
    it('UIX-046/duplo clique', async () => {
      renderApp()
      fireEvent.doubleClick(await findCard('Hades'))
      await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
      await flush()
      expect(calls('LaunchGame')).toEqual([[20]])
      expect(calls('LaunchGameUntracked')).toHaveLength(0)
    })

    it('UIX-046/Enter no cartão', async () => {
      renderApp()
      const card = await findCard('Hades')
      card.focus()
      fireEvent.keyDown(card, { key: 'Enter' })
      await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
      await flush()
      expect(calls('LaunchGame')).toEqual([[20]])
      expect(calls('LaunchGameUntracked')).toHaveLength(0)
    })

    it('UIX-046/Jogar no painel', async () => {
      renderApp()
      await focusGame('Hades')
      expect(calls('LaunchGame')).toHaveLength(0)
      fireEvent.click(within(aside()).getByRole('button', { name: 'Jogar' }))
      await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
      await flush()
      expect(calls('LaunchGame')).toEqual([[20]])
      expect(calls('LaunchGameUntracked')).toHaveLength(0)
    })

    // alterado em 2026-11-15 (fase 13): A (Enter) no Big Box novo lança o jogo em foco uma vez.
    it('UIX-046/A (Enter) no Big Box', async () => {
      renderApp()
      await findCard('Hades')
      fireEvent.keyDown(window, { key: 'F11' })
      await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThan(0))
      fireEvent.keyDown(window, { key: 'Enter' })
      await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
      await flush()
      expect(calls('LaunchGame')).toHaveLength(1)
      expect(calls('LaunchGameUntracked')).toHaveLength(0)
    })

    it('UIX-046/Jogar sem contar tempo chama LaunchGameUntracked uma vez e não LaunchGame', async () => {
      renderApp()
      await focusGame('Hades')
      fireEvent.click(within(aside()).getByRole('button', { name: 'Jogar sem contar tempo' }))
      await waitFor(() => expect(calls('LaunchGameUntracked')).toHaveLength(1))
      await flush()
      expect(calls('LaunchGameUntracked')).toEqual([[20]])
      expect(calls('LaunchGame')).toHaveLength(0)
    })
  })

  // alterado em 2026-11-15 (fase 13): no Big Box novo o repeat do Enter não lança de novo.
  it('UIX-046/Enter mantido na grade e Enter mantido no Big Box chamam LaunchGame exatamente uma vez cada', async () => {
    renderApp()
    const card = await findCard('Hades')
    card.focus()
    fireEvent.keyDown(card, { key: 'Enter' })
    fireEvent.keyDown(card, { key: 'Enter', repeat: true })
    fireEvent.keyDown(card, { key: 'Enter', repeat: true })
    await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
    await flush()
    expect(calls('LaunchGame')).toEqual([[20]])

    fireEvent.keyDown(window, { key: 'F11' })
    await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThan(0))
    fireEvent.keyDown(window, { key: 'Enter' })
    fireEvent.keyDown(window, { key: 'Enter', repeat: true })
    fireEvent.keyDown(window, { key: 'Enter', repeat: true })
    await waitFor(() => expect(calls('LaunchGame')).toHaveLength(2))
    await flush()
    expect(calls('LaunchGame')).toHaveLength(2)
  })

  it('UIX-046/ToggleSession só é chamado pelo painel de sessão', async () => {
    // ToggleSession is Pause/Resume of the running session.
    overrides.CurrentSession = () => session('running')
    renderApp()
    await screen.findByRole('status', { name: 'Sessão em andamento' })

    // Everything else in the UI leaves the session alone.
    await focusGame('Hades')
    expect(screen.queryByRole('status', { name: 'Sessão em andamento' }), 'after focus').not.toBeNull()
    const card = await findCard('Celeste')
    fireEvent.keyDown(card, { key: 'ArrowLeft' })
    expect(screen.queryByRole('status', { name: 'Sessão em andamento' }), 'after arrow').not.toBeNull()
    // alterado em 2026-11-15 (fase 13): sai do Big Box novo com Escape (B).
    fireEvent.keyDown(window, { key: 'F11' })
    await waitFor(() => expect(document.querySelector('[data-bigbox-theme]')).not.toBeNull())
    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => expect(document.querySelector('[data-bigbox-theme]')).toBeNull())
    await flush()
    expect(calls('PauseSession')).toHaveLength(0)
    expect(calls('ResumeSession')).toHaveLength(0)

    const livePanel = await screen.findByRole('status', { name: 'Sessão em andamento' })
    fireEvent.click(within(livePanel).getByRole('button', { name: 'Pausar' }))
    await waitFor(() => expect(calls('PauseSession')).toHaveLength(1))
    await flush()
    expect(calls('PauseSession')).toHaveLength(1)
    expect(calls('ResumeSession')).toHaveLength(0)
  })

  it('IMG-012/eventos media:changed e reconexão chamam só leituras; nenhum LaunchGame, LaunchGameUntracked, Pause/Resume nem escrita; jogo em foco e sessão exibida não mudam', async () => {
    const handlers = new Map<string, Set<(...d: unknown[]) => void>>()
    ;(window as unknown as { runtime: unknown }).runtime = {
      EventsOn: vi.fn((name: string, cb: (...d: unknown[]) => void) => {
        const set = handlers.get(name) ?? new Set()
        handlers.set(name, set)
        set.add(cb)
        return () => set.delete(cb)
      }),
    }
    const emit = (name: string, ...data: unknown[]) =>
      act(() => {
        for (const cb of handlers.get(name) ?? []) cb(...data)
      })
    const defaultStatus = (overrides.AgentStatus ?? ((...a: unknown[]) => defaults('AgentStatus', a))) as () => unknown
    let up = true
    overrides.AgentStatus = () => (up ? defaultStatus() : { connected: false, version: '', pid: 0, startedAt: '', dataDir: '', uptimeSeconds: 0 })
    overrides.CurrentSession = () => session('running')
    renderApp()
    await screen.findByRole('status', { name: 'Sessão em andamento' })
    await focusGame('Hades')
    await flush()
    const state = () => document.querySelector('[data-state]')?.getAttribute('data-state')
    await waitFor(() => expect(state()).toBe('connected'))

    // media:changed events: a visible game, a game that is not on screen and a bad payload.
    emit('media:changed', { gameId: 20 })
    emit('media:changed', { gameId: 30 })
    emit('media:changed', { gameId: 999 })
    emit('media:changed', {})
    await flush()

    // Disconnect and reconnect.
    up = false
    emit('agent:heartbeat')
    await waitFor(() => expect(state()).toBe('disconnected'), { timeout: 3000 })
    up = true
    emit('agent:heartbeat')
    await waitFor(() => expect(state()).toBe('connected'))
    emit('media:changed', { gameId: 20 })
    await flush()

    expect(writeCalls(), 'bindings que escrevem ou lançam').toEqual([])
    for (const n of ['LaunchGame', 'LaunchGameUntracked', 'PauseSession', 'ResumeSession']) {
      expect(calls(n), n).toHaveLength(0)
    }
    expect(within(aside()).getByRole('heading', { level: 2, name: 'Hades' })).toBeInTheDocument()
    const panel = screen.getByRole('status', { name: 'Sessão em andamento' })
    expect(within(panel).getByText(/Hollow Knight/)).toBeInTheDocument()
    expect(within(panel).getByRole('button', { name: 'Pausar' })).toBeInTheDocument()
  })

  it('AJU-026/clicar e navegar nas miniaturas, rolar áreas e passar o mouse chamam só leituras', async () => {
    renderApp()
    await focusGame('Hollow Knight')
    const strip = await within(aside()).findByRole('group', { name: 'Miniaturas' })
    const thumbs = within(strip).getAllByRole('button')
    expect(thumbs.length).toBeGreaterThan(1)
    for (const t of thumbs) {
      fireEvent.mouseEnter(t)
      fireEvent.mouseOver(t)
      fireEvent.click(t)
    }
    fireEvent.keyDown(within(strip).getAllByRole('button')[0], { key: 'ArrowRight' })
    fireEvent.keyDown(within(strip).getAllByRole('button')[0], { key: 'ArrowLeft' })
    fireEvent.scroll(strip)
    fireEvent.scroll(aside())
    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    fireEvent.scroll(listbox)
    for (const c of cards()) {
      fireEvent.mouseEnter(c)
      fireEvent.mouseOver(c)
      fireEvent.mouseMove(c)
    }
    await flush()
    expect(writeCalls(), 'bindings que escrevem ou lançam').toEqual([])
    expect(calls('LaunchGame')).toHaveLength(0)
    expect(calls('LaunchGameUntracked')).toHaveLength(0)
  })

  it('AJU-026/importar 60 candidatos em lotes chama só ImportScan e ImportApply 3 vezes, sem lançamento nem pausa', async () => {
    const candidates = Array.from({ length: 60 }, (_, i) => ({
      source: 'steam',
      title: `Jogo ${i}`,
      platform: 'Windows',
      launchType: 'uri',
      launchTarget: `steam://rungameid/${i}`,
      installDir: '',
      developers: [],
      publishers: [],
      genres: [],
      year: null,
      description: '',
      playModes: [],
      playedSeconds: null,
      lastPlayed: null,
      exists: false,
    }))
    overrides.ImportScan = () => ({ candidates, warnings: [] })
    overrides.ImportApply = () => ({ applied: [], skipped: [] })
    renderApp()
    await findCard('Hades')
    fireEvent.click(within(topBar()).getByRole('button', { name: 'Importar' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Fonte'), { target: { value: 'steam' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Buscar' }))
    // IMV-041: all rows come already checked; the checkbox name is "Selecionar <título>" (IMV-052).
    await within(dialog).findByRole('checkbox', { name: 'Selecionar Jogo 0' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Importar selecionados' }))
    await waitFor(() => expect(calls('ImportApply')).toHaveLength(3))
    await flush()

    expect(calls('ImportScan')).toHaveLength(1)
    expect(calls('ImportApply')).toHaveLength(3)
    for (const n of ['LaunchGame', 'LaunchGameUntracked', 'PauseSession', 'ResumeSession']) {
      expect(calls(n), n).toHaveLength(0)
    }
    const others = writeCalls().filter((n) => n !== 'ImportApply')
    expect(others, 'outras escritas').toEqual([])
  })

  it('IMV-081/ler pasta, marcar, editar título, escolher plataforma e importar em lotes chamam só ImportScan, ImportApply, PickFile, PickFolder e a leitura do catálogo; nenhum LaunchGame, LaunchGameUntracked, ToggleSession, Pause nem Resume', async () => {
    // Fonte "Arquivo": título editável e select "Plataforma do arquivo" (IMV-060, IMV-024).
    const candidates = Array.from({ length: 3 }, (_, i) => ({
      source: 'file',
      title: `Jogo ${i}`,
      platform: 'Windows',
      launchType: 'exe',
      launchTarget: `C:\\Games\\jogo${i}.z64`,
      installDir: '',
      file: `jogo${i}.z64`,
      extension: 'z64',
      developers: [],
      publishers: [],
      genres: [],
      year: null,
      description: '',
      playModes: [],
      playedSeconds: null,
      lastPlayed: null,
      exists: false,
    }))
    overrides.PickFile = () => 'C:\\Games\\jogo0.z64'
    overrides.ImportScan = () => ({ candidates, warnings: [] })
    overrides.ImportApply = () => ({ applied: [], skipped: [] })
    renderApp()
    await findCard('Hades')
    fireEvent.click(within(topBar()).getByRole('button', { name: 'Importar' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Fonte'), { target: { value: 'file' } })
    // Escolher arquivo chama PickFile; depois Buscar chama ImportScan.
    fireEvent.click(within(dialog).getByRole('button', { name: 'Escolher arquivo' }))
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Arquivo' })).toHaveValue('C:\\Games\\jogo0.z64'))
    // Escolher plataforma antes de ler: trocar plataforma depois descarta a lista (IMV-049).
    const platform = within(dialog).getByRole('combobox', { name: 'Plataforma do arquivo' }) as HTMLSelectElement
    fireEvent.change(platform, { target: { value: platform.options[platform.options.length - 1].value } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Buscar' }))
    const box = await within(dialog).findByRole('checkbox', { name: 'Selecionar Jogo 0' })
    // Marcar e desmarcar.
    fireEvent.click(box)
    fireEvent.click(box)
    // Editar um título.
    const titleField = within(dialog).getByRole('textbox', { name: 'Título de jogo1.z64' })
    fireEvent.change(titleField, { target: { value: 'Jogo 1 editado' } })
    // Importar em lote.
    fireEvent.click(within(dialog).getByRole('button', { name: 'Importar selecionados' }))
    await waitFor(() => expect(calls('ImportApply').length).toBeGreaterThan(0))
    await flush()

    expect(calls('ImportScan')).toHaveLength(1)
    for (const n of ['LaunchGame', 'LaunchGameUntracked', 'ToggleSession', 'PauseSession', 'ResumeSession']) {
      expect(calls(n), n).toHaveLength(0)
    }
    const allowed = new Set(['ImportScan', 'ImportApply', 'PickFile', 'PickFolder'])
    const others = writeCalls().filter((n) => !allowed.has(n))
    expect(others, 'escritas fora de ImportScan/ImportApply').toEqual([])
  })

  it('AJU-028/as chamadas da UI nas interações da fase 10.1 estão na lista de bindings da fase 9', async () => {
    renderApp()
    await focusGame('Hollow Knight')
    const strip = await within(aside()).findByRole('group', { name: 'Miniaturas' })
    for (const t of within(strip).getAllByRole('button')) {
      fireEvent.click(t)
    }
    fireEvent.scroll(screen.getByRole('listbox', { name: 'Jogos' }))
    fireEvent.click(within(topBar()).getByRole('button', { name: 'Importar' }))
    fireEvent.keyDown(await screen.findByRole('dialog'), { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await flush()

    const called = Array.from(spies.entries())
      .filter(([, spy]) => spy.mock.calls.length > 0)
      .map(([name]) => name)
    expect(called.length).toBeGreaterThan(3)
    // alterado em 2026-11-15 (phase-12-metadados-unificados, T35): leituras da fase 12 permitidas (MET-080); proibições não mudam.
    // alterado em 2026-11-22 (fase 15): leituras do tempo para zerar permitidas (TTB-030).
    // alterado em 2026-11-22 (fase 15): TakeBigBoxRequest é leitura feita em toda montagem (ATB-008).
    const phase12Reads = new Set(['MetadataBatchStatus', 'TimeToBeatGet', 'TimeToBeatSettings', 'TakeBigBoxRequest'])
    const unknown = called.filter((n) => !phase9.has(n) && !phase12Reads.has(n))
    expect(unknown, `bindings fora da fase 9: ${unknown.join(', ')}`).toEqual([])
  })

  it('UIX-047/nenhum binding novo: as chamadas feitas pela UI estão na lista de bindings da fase 9', async () => {
    renderApp()
    await focusGame('Hades')
    fireEvent.click(within(aside()).getByRole('button', { name: 'Jogar' }))
    await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
    fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
    await screen.findByRole('heading', { name: 'Estatísticas' })
    fireEvent.keyDown(window, { key: 'Escape' })
    for (const name of ['Provedores', 'Emuladores', 'Plataformas', 'Importar']) {
      fireEvent.click(within(topBar()).getByRole('button', { name }))
      fireEvent.keyDown(await screen.findByRole('dialog'), { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    }
    await flush()

    const called = Array.from(spies.entries())
      .filter(([, spy]) => spy.mock.calls.length > 0)
      .map(([name]) => name)
    expect(called.length).toBeGreaterThan(3)
    // alterado em 2026-11-15 (phase-12-metadados-unificados, T35): leituras da fase 12 permitidas (MET-080); proibições não mudam.
    // alterado em 2026-11-22 (fase 15): leituras do tempo para zerar permitidas (TTB-030).
    // alterado em 2026-11-22 (fase 15): TakeBigBoxRequest é leitura feita em toda montagem (ATB-008).
    const phase12Reads = new Set(['MetadataBatchStatus', 'TimeToBeatGet', 'TimeToBeatSettings', 'TakeBigBoxRequest'])
    const unknown = called.filter((n) => !phase9.has(n) && !phase12Reads.has(n))
    expect(unknown, `bindings fora da fase 9: ${unknown.join(', ')}`).toEqual([])
  })

  it('MET-080/obter dados, escolher opção, marcar a caixa, importar, acompanhar o progresso, confirmar, cancelar e descartar chamam só MetadataSearch, MetadataApply, MetadataBatch*, ImportScan, ImportApply, PickFile, PickFolder, ProviderKeyStatus e leituras; nenhum LaunchGame, LaunchGameUntracked, ToggleSession, Pause nem Resume', async () => {
    const handlers = new Map<string, Set<(...d: unknown[]) => void>>()
    ;(window as unknown as { runtime: unknown }).runtime = {
      EventsOn: vi.fn((name: string, cb: (...d: unknown[]) => void) => {
        const set = handlers.get(name) ?? new Set()
        handlers.set(name, set)
        set.add(cb)
        return () => set.delete(cb)
      }),
    }
    const emit = (name: string, ...data: unknown[]) =>
      act(() => {
        for (const cb of handlers.get(name) ?? []) cb(...data)
      })

    // The batch the fake agent holds (MET-052); the UI reads it through MetadataBatchStatus.
    type FakeBatch = {
      batchId: number
      state: string
      total: number
      done: number
      failed: number
      warnings: string[]
      result?: unknown
    }
    // The cast keeps TypeScript from narrowing the variable to null after the initializer.
    let batch = null as FakeBatch | null
    // notify is the metadata:batch event that tells the UI the batch changed (MET-055).
    const notify = () => {
      if (batch) emit('metadata:batch', { batchId: batch.batchId, state: batch.state, done: batch.done, total: batch.total, failed: batch.failed })
    }
    const option = (title: string) => ({
      refs: [{ provider: 'igdb', providerId: `igdb-${title}` }],
      title,
      year: 2001,
      platform: 'Windows',
      score: 90,
    })
    const candidates = Array.from({ length: 2 }, (_, i) => ({
      source: 'folder',
      title: `Jogo ${i}`,
      platform: 'Windows',
      launchType: 'exe',
      launchTarget: `E:\\Games\\ps1\\jogo${i}.bin`,
      installDir: '',
      file: `jogo${i}.bin`,
      extension: 'bin',
      developers: [],
      publishers: [],
      genres: [],
      year: null,
      description: '',
      playModes: [],
      playedSeconds: null,
      lastPlayed: null,
      exists: false,
    }))
    const rows = [40, 41].map((gameId, i) => ({
      gameId,
      title: `Jogo ${i}`,
      platform: 'Windows',
      confidence: 'match',
      selected: 0,
      candidates: [option(`Jogo ${i} (oficial)`), option(`Jogo ${i} (outra edição)`)],
    }))

    overrides.ProviderKeyStatus = (provider) => ({ provider, configured: true })
    overrides.PickFile = () => 'C:\\Games\\alfa.bin'
    overrides.PickFolder = () => 'E:\\Games\\ps1'
    overrides.MetadataSearch = () => ({ candidates: [option('Jogo Alfa')], confidence: 'match', warnings: [] })
    overrides.ImportScan = () => ({ candidates, warnings: [] })
    overrides.ImportApply = () => ({
      applied: [
        { gameId: 40, title: 'Jogo 0' },
        { gameId: 41, title: 'Jogo 1' },
      ],
      skipped: [],
    })
    overrides.MetadataBatchStart = (ids: unknown) => {
      const n = (ids as number[]).length
      batch = { batchId: 7, state: 'searching', total: n, done: 0, failed: 0, warnings: [] }
      return { batchId: 7, total: n }
    }
    overrides.MetadataBatchStatus = () => ({ batch })
    overrides.MetadataBatchResults = () => ({ rows, total: rows.length, next: null })
    overrides.MetadataBatchCancel = () => {
      if (batch) batch = { ...batch, state: 'cancelled' }
      return 'cancelled'
    }
    overrides.MetadataBatchApply = () => {
      if (batch) batch = { ...batch, state: 'applying', done: 0 }
      return 1
    }
    overrides.MetadataBatchDiscard = () => {
      batch = null
    }

    renderApp()
    await findCard('Hades')
    fireEvent.click(within(topBar()).getByRole('button', { name: 'Importar' }))
    const dialog = await screen.findByRole('dialog')

    // Obter dados e escolher uma opção na fonte Arquivo (MET-042).
    fireEvent.change(within(dialog).getByLabelText('Fonte'), { target: { value: 'file' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Escolher arquivo' }))
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Arquivo' })).toHaveValue('C:\\Games\\alfa.bin'))
    // Plataforma antes de ler, como no IMV-081 (sem ela "Obter dados" fica desabilitado).
    const filePlatform = within(dialog).getByRole('combobox', { name: 'Plataforma do arquivo' }) as HTMLSelectElement
    fireEvent.change(filePlatform, { target: { value: filePlatform.options[filePlatform.options.length - 1].value } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Buscar' }))
    await within(dialog).findByRole('checkbox', { name: 'Selecionar Jogo 0' })
    // "Nome para busca" vazio desabilita "Obter dados": preenche o nome do arquivo.
    fireEvent.change(within(dialog).getByLabelText('Nome para busca'), { target: { value: 'Jogo Alfa' } })
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Obter dados' })).toBeEnabled())
    fireEvent.click(within(dialog).getByRole('button', { name: 'Obter dados' }))
    fireEvent.click(await within(dialog).findByRole('radio', { name: /Jogo Alfa/ }))
    await flush()

    // Pasta com a caixa marcada, lote de busca e importação.
    fireEvent.change(within(dialog).getByLabelText('Fonte'), { target: { value: 'folder' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Escolher pasta' }))
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Pasta' })).toHaveValue('E:\\Games\\ps1'))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Buscar' }))
    await within(dialog).findByRole('checkbox', { name: 'Selecionar Jogo 0' })
    const box = within(dialog).getByRole('checkbox', { name: 'Buscar metadados e imagens' })
    expect(box).toBeChecked()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Importar selecionados' }))
    await waitFor(() => expect(calls('MetadataBatchStart')).toHaveLength(1))
    expect(calls('MetadataBatchStart')).toEqual([[[40, 41]]])

    // Acompanhar o progresso e fechar a janela de importação; o indicador segue no cabeçalho.
    batch = { ...(batch as FakeBatch), done: 1 }
    notify()
    fireEvent.keyDown(dialog, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(await screen.findByText('Buscando metadados 1/2')).toBeInTheDocument()

    // Cancelar a busca no meio.
    fireEvent.click(within(topBar()).getByRole('button', { name: 'Cancelar' }))
    await waitFor(() => expect(calls('MetadataBatchCancel')).toHaveLength(1))
    expect(calls('MetadataBatchCancel')).toEqual([[7]])
    notify()

    // Confirmar: revisar, desmarcar um jogo e aplicar os marcados.
    fireEvent.click(await within(topBar()).findByRole('button', { name: 'Revisar metadados (2)' }))
    const review = (await screen.findByRole('dialog', { name: 'Confirmar metadados' })) as HTMLElement
    fireEvent.click(await within(review).findByRole('checkbox', { name: 'Selecionar Jogo 1' }))
    fireEvent.click(within(review).getByRole('button', { name: 'Aplicar selecionados' }))
    await waitFor(() => expect(calls('MetadataBatchApply')).toHaveLength(1))
    expect(calls('MetadataBatchApply')).toEqual([[7, [{ gameId: 40, candidate: 0 }]]])
    await flush()

    // Resumo do lote e descarte (MET-066).
    batch = { batchId: 7, state: 'done', total: 2, done: 2, failed: 0, warnings: [], result: { applied: 1, imagesQueued: 0, imagesSkippedByLimit: 0, failedTitles: [] } }
    notify()
    fireEvent.click(await within(topBar()).findByRole('button', { name: 'Fechar' }))
    await waitFor(() => expect(calls('MetadataBatchDiscard')).toHaveLength(1))
    expect(calls('MetadataBatchDiscard')).toEqual([[7]])
    await flush()

    // Nenhum lançamento, sessão ou pausa; só as ações da fase 12, importação e leituras.
    for (const n of ['LaunchGame', 'LaunchGameUntracked', 'ToggleSession', 'PauseSession', 'ResumeSession']) {
      expect(calls(n), n).toHaveLength(0)
    }
    const allowed = new Set([
      'MetadataSearch',
      'MetadataApply',
      'MetadataBatchStart',
      'MetadataBatchStatus',
      'MetadataBatchResults',
      'MetadataBatchApply',
      'MetadataBatchCancel',
      'MetadataBatchDiscard',
      'ImportScan',
      'ImportApply',
      'PickFile',
      'PickFolder',
      'ProviderKeyStatus',
    ])
    const called = Array.from(spies.entries())
      .filter(([, spy]) => spy.mock.calls.length > 0)
      .map(([name]) => name)
    // alterado em 2026-11-22 (fase 15): leituras do tempo para zerar (TimeToBeatGet, TimeToBeatSettings).
    const outside = called.filter((n) => !allowed.has(n) && !/^(List|Get|Catalog|Agent|Current|Game|Media|Emulator|Stats|TakeBigBoxRequest$|TimeToBeat(Get|Settings)$)/.test(n))
    expect(outside, `bindings fora da lista da fase 12`).toEqual([])
    const writesOutside = writeCalls().filter((n) => !allowed.has(n))
    expect(writesOutside, 'escritas fora das ações da fase 12').toEqual([])
  })

  // Fase 13 (CON-024, CON-003): the Big Box and the video actions never touch the session or close a game.
  function captureEvents() {
    const handlers = new Map<string, Set<(...d: unknown[]) => void>>()
    ;(window as unknown as { runtime: unknown }).runtime = {
      EventsOn: vi.fn((name: string, cb: (...d: unknown[]) => void) => {
        const set = handlers.get(name) ?? new Set()
        handlers.set(name, set)
        set.add(cb)
        return () => set.delete(cb)
      }),
    }
    return (name: string, ...data: unknown[]) =>
      act(() => {
        for (const cb of handlers.get(name) ?? []) cb(...data)
      })
  }

  const closeLike = /(Close|Kill|Stop|Terminate|EndSession|Discard)/
  const forbiddenSession = ['ToggleSession', 'PauseSession', 'ResumeSession']

  function calledNames(): string[] {
    return Array.from(spies.entries())
      .filter(([, spy]) => spy.mock.calls.length > 0)
      .map(([name]) => name)
  }

  it('CON-024/Big Box, temas, detalhes, galeria, som, "Buscar vídeo", "Buscar vídeos dos jogos sem vídeo", cancelar buscas, janela dos provedores e o evento game:closedByHotkey: os bindings chamados são só leituras, LaunchGame, LaunchGameUntracked, BringToFront, Video* e ProviderSetKey/ProviderDeleteKey; nenhum ToggleSession, Pause, Resume, nem ação de fechar jogo ou encerrar sessão', async () => {
    const emit = captureEvents()
    overrides.ProviderKeyStatus = (provider) => ({ provider, configured: true })
    overrides.VideoQueue = () => ({ queued: 1, skipped: [] })
    overrides.VideoQueueMissing = () => ({ queued: 2, skipped: 0 })
    overrides.VideoCancelPending = () => 1
    overrides.VideoStatus = () => ({
      pending: 0,
      running: 0,
      failed: 0,
      lane: 'active',
      pausedUntil: '',
      requestsToday: 0,
      maxRequestsPerDay: 0,
    })
    renderApp()
    await focusGame('Hades')

    // Desktop video actions: "Buscar vídeo" in the details and the batch actions in the providers window.
    const searchVideo = await within(aside()).findByRole('button', { name: 'Buscar vídeo' })
    fireEvent.click(searchVideo)
    await flush()
    fireEvent.click(within(topBar()).getByRole('button', { name: 'Provedores' }))
    const providers = await screen.findByRole('dialog')
    fireEvent.click(await within(providers).findByRole('button', { name: 'Buscar vídeos dos jogos sem vídeo' }))
    await flush()
    fireEvent.click(within(providers).getByRole('button', { name: 'Cancelar buscas pendentes' }))
    await flush()
    fireEvent.keyDown(providers, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    // The theme select, then the Big Box: details, gallery, sound, back and the hotkey close notice.
    const themeSel = screen.getByLabelText('Tema do Big Box') as HTMLSelectElement
    fireEvent.change(themeSel, { target: { value: themeSel.value } })
    fireEvent.click(screen.getByRole('button', { name: 'Big Box' }))
    await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThan(0))
    for (const key of ['ArrowRight', 'ArrowLeft', 'PageDown', 'PageUp', 'd', 'ArrowRight', 'ArrowDown', 'ArrowUp', 'm', 'Escape']) {
      fireEvent.keyDown(window, { key })
    }
    await flush()
    emit('game:closedByHotkey', { gameId: 20 })
    await flush()
    // Whatever screen is showing, leave the Big Box if still in it.
    if (document.querySelector('[data-bigbox-theme]') !== null) {
      fireEvent.keyDown(window, { key: 'Escape' })
      await waitFor(() => expect(document.querySelector('[data-bigbox-theme]')).toBeNull())
    }

    const called = calledNames()
    for (const n of forbiddenSession) expect(calls(n), n).toHaveLength(0)
    expect(called.filter((n) => closeLike.test(n)), 'ação de fechar jogo ou encerrar sessão').toEqual([])
    const allowedWrites = new Set(['LaunchGame', 'LaunchGameUntracked', 'BringToFront', 'ProviderSetKey', 'ProviderDeleteKey'])
    const writesOutside = writeCalls().filter((n) => !allowedWrites.has(n))
    expect(writesOutside, 'escritas fora da lista').toEqual([])
    const outside = called.filter(
      // alterado em 2026-11-22 (fase 15): leituras do tempo para zerar (TimeToBeatGet, TimeToBeatSettings).
      (n) => !allowedWrites.has(n) && !/^(Video|EnterBigBox|ExitBigBox)/.test(n) && !/^(List|Get|Catalog|Agent|Current|Game|Media|Emulator|Stats|ProviderKeyStatus|Metadata|TakeBigBoxRequest$|TimeToBeat(Get|Settings)$)/.test(n),
    )
    expect(outside, 'bindings fora da lista da fase 13').toEqual([])
    expect(calls('LaunchGame')).toHaveLength(0)
    expect(calls('LaunchGameUntracked')).toHaveLength(0)
  })

  it('CON-024/LaunchGame e LaunchGameUntracked uma vez por ação, sem repetição por tecla mantida', async () => {
    renderApp()
    await findCard('Hades')
    fireEvent.keyDown(window, { key: 'F11' })
    await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThan(0))

    // Held Enter on the strip: one launch.
    fireEvent.keyDown(window, { key: 'Enter' })
    fireEvent.keyDown(window, { key: 'Enter', repeat: true })
    fireEvent.keyDown(window, { key: 'Enter', repeat: true })
    await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
    await flush()
    expect(calls('LaunchGame')).toHaveLength(1)
    expect(calls('LaunchGameUntracked')).toHaveLength(0)
  })

  it('CON-024/LaunchGameUntracked uma vez com Enter mantido em "Jogar sem contar tempo" nos detalhes', async () => {
    renderApp()
    await findCard('Hades')
    fireEvent.keyDown(window, { key: 'F11' })
    await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThan(0))
    fireEvent.keyDown(window, { key: 'd' })
    await flush()
    fireEvent.keyDown(window, { key: 'ArrowRight' }) // BBX-033
    fireEvent.keyDown(window, { key: 'Enter' })
    fireEvent.keyDown(window, { key: 'Enter', repeat: true })
    fireEvent.keyDown(window, { key: 'Enter', repeat: true })
    await waitFor(() => expect(calls('LaunchGameUntracked')).toHaveLength(1))
    await flush()
    expect(calls('LaunchGameUntracked')).toHaveLength(1)
    expect(calls('LaunchGame')).toHaveLength(0)
  })

  // alterado em 2026-11-22 (fase 15): interações do tempo para zerar (TTB-030).
  it('TTB-030/abrir o painel, os provedores e a edição com o tempo para zerar (inclusive "Confirmar…", "Atualizar" e as caixas) não chama LaunchGame, LaunchGameUntracked, pausa, retomada nem fechar jogo', async () => {
    const emit = captureEvents()
    overrides.ProviderKeyStatus = (provider) => ({ provider, configured: true })
    overrides.TimeToBeatSettings = () => ({ enabled: true, afterMetadata: true })
    overrides.TimeToBeatSetSettings = (enabled, afterMetadata) => ({ enabled, afterMetadata })
    overrides.TimeToBeatGet = (id) => ({
      gameId: id,
      state: 'doubt',
      source: 'hltb',
      externalId: '',
      externalTitle: '',
      mainSeconds: null,
      mainExtraSeconds: null,
      completionistSeconds: null,
      fetchedAt: '',
      confirmed: false,
      stale: false,
      pending: false,
      unavailable: false,
      candidates: [],
    })
    overrides.TimeToBeatRefresh = (id) => overrides.TimeToBeatGet(id)
    renderApp()
    await focusGame('Hades')

    // Panel: the block reads the cache and "Confirmar…" opens "Editar jogo" at the section.
    fireEvent.click(await within(aside()).findByRole('button', { name: 'Confirmar…' }))
    const edit = await screen.findByRole('dialog')
    expect(await within(edit).findByText('Tempo para zerar (HowLongToBeat)')).toBeInTheDocument()
    fireEvent.click(await within(edit).findByRole('button', { name: 'Atualizar' }))
    await flush()
    emit('timeToBeat:changed', { gameId: 20 })
    await flush()
    fireEvent.keyDown(edit, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    // Providers window with the two checkboxes.
    fireEvent.click(within(topBar()).getByRole('button', { name: 'Provedores' }))
    const providers = await screen.findByRole('dialog')
    fireEvent.click(await within(providers).findByRole('checkbox', { name: 'Consultar o HowLongToBeat' }))
    await flush()
    fireEvent.keyDown(providers, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    for (const n of ['LaunchGame', 'LaunchGameUntracked', ...forbiddenSession]) expect(calls(n), n).toHaveLength(0)
    expect(calledNames().filter((n) => closeLike.test(n)), 'ação de fechar jogo ou encerrar sessão').toEqual([])
    const allowedWrites = new Set(['TimeToBeatRefresh', 'TimeToBeatSearch', 'TimeToBeatChoose', 'TimeToBeatSetSettings'])
    expect(writeCalls().filter((n) => !allowedWrites.has(n)), 'escritas fora do tempo para zerar').toEqual([])
    expect(calls('TimeToBeatRefresh')).toEqual([[20]])
    expect(calls('TimeToBeatSetSettings')).toEqual([[false, true]])
  })

  // alterado em 2026-11-22 (fase 15): o pedido externo de Big Box (ATB-015).
  it('ATB-015/o pedido externo entra no Big Box sem chamar LaunchGame, LaunchGameUntracked, pausa, retomada nem fechar jogo', async () => {
    const emit = captureEvents()
    overrides.CurrentSession = () => session('running')
    let pendingRequest = false
    overrides.TakeBigBoxRequest = () => {
      const r = pendingRequest
      pendingRequest = false
      return r
    }
    renderApp()
    await findCard('Hades')
    pendingRequest = true
    await emit('ui:bigBoxRequested')
    await waitFor(() => expect(document.querySelector('[data-bigbox-theme]')).not.toBeNull())
    // A second request while already in the Big Box changes nothing.
    pendingRequest = true
    await emit('ui:bigBoxRequested')
    await flush()

    for (const n of ['LaunchGame', 'LaunchGameUntracked', ...forbiddenSession]) expect(calls(n), n).toHaveLength(0)
    expect(calledNames().filter((n) => closeLike.test(n)), 'ação de fechar jogo ou encerrar sessão').toEqual([])
    expect(writeCalls().filter((n) => n !== 'BringToFront')).toEqual([])
    expect(screen.queryByRole('button', { name: /fechar jogo|encerrar/i })).toBeNull()
  })

  it('CON-003/o Big Box não tem botão nem atalho de teclado para fechar o jogo (nenhuma tecla além das do mapa de entrada o aciona)', async () => {
    overrides.CurrentSession = () => session('running')
    renderApp()
    await findCard('Hades')
    fireEvent.keyDown(window, { key: 'F11' })
    await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThan(0))

    expect(screen.queryByRole('button', { name: /fechar jogo|encerrar|fechar o jogo|sair do jogo/i })).toBeNull()

    const mapped = new Set(['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Enter', 'd', 'D', 'PageUp', 'PageDown', 'Escape', 'm', 'M', 'F11'])
    const others = [
      ...'abcefghijklnopqrstuvwxyz0123456789 ',
      'Delete',
      'Backspace',
      'Tab',
      'End',
      'Home',
      'F1',
      'F2',
      'F4',
      'F10',
      'F12',
    ].filter((k) => !mapped.has(k))
    for (const k of others) {
      fireEvent.keyDown(window, { key: k })
      fireEvent.keyDown(window, { key: k, altKey: true })
      fireEvent.keyDown(window, { key: k, ctrlKey: true })
    }
    fireEvent.keyDown(window, { key: 'F4', altKey: true })
    await flush()

    expect(calledNames().filter((n) => closeLike.test(n))).toEqual([])
    for (const n of [...forbiddenSession, 'LaunchGame', 'LaunchGameUntracked']) expect(calls(n), n).toHaveLength(0)
    expect(writeCalls()).toEqual([])
  })
})
