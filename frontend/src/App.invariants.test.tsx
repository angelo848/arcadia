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
    fireEvent.keyDown(window, { key: 'F11' })
    fireEvent.click(await screen.findByRole('button', { name: /Sair do Big Box/ }))
    await waitFor(() => expect(screen.queryByRole('button', { name: /Sair do Big Box/ })).toBeNull())
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

    it('UIX-046/Y (Shift+Enter) no Big Box', async () => {
      renderApp()
      await findCard('Hades')
      fireEvent.keyDown(window, { key: 'F11' })
      await screen.findByRole('button', { name: /Sair do Big Box/ })
      const card = await findCard('Celeste')
      card.focus()
      fireEvent.keyDown(window, { key: 'Enter', shiftKey: true })
      await waitFor(() => expect(calls('LaunchGame')).toHaveLength(1))
      await flush()
      expect(calls('LaunchGame')).toEqual([[30]])
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

  it('UIX-046/Enter mantido na grade e Shift+Enter mantido no Big Box chamam LaunchGame exatamente uma vez cada', async () => {
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
    await screen.findByRole('button', { name: /Sair do Big Box/ })
    const bb = await findCard('Celeste')
    bb.focus()
    fireEvent.keyDown(window, { key: 'Enter', shiftKey: true })
    fireEvent.keyDown(window, { key: 'Enter', shiftKey: true, repeat: true })
    fireEvent.keyDown(window, { key: 'Enter', shiftKey: true, repeat: true })
    await waitFor(() => expect(calls('LaunchGame')).toHaveLength(2))
    await flush()
    expect(calls('LaunchGame')).toEqual([[20], [30]])
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
    fireEvent.keyDown(window, { key: 'F11' })
    fireEvent.click(await screen.findByRole('button', { name: /Sair do Big Box/ }))
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
    const unknown = called.filter((n) => !phase9.has(n))
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
    const unknown = called.filter((n) => !phase9.has(n))
    expect(unknown, `bindings fora da fase 9: ${unknown.join(', ')}`).toEqual([])
  })
})
