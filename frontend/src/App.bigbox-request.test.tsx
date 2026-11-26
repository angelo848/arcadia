import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'

// Fase 15 (ATB-008..ATB-013): the Big Box request from the agent / second instance.
// Bindings are spies on a Proxy; the Wails events are captured by the fake runtime.

type Spy = ReturnType<typeof vi.fn>
let spies: Map<string, Spy>
let overrides: Record<string, (...a: unknown[]) => unknown>
let handlers: Map<string, Set<(...d: unknown[]) => void>>
let offSpies: Spy[]
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

function defaults(name: string, args: unknown[]): unknown {
  switch (name) {
    case 'AgentStatus':
      return { connected: true, version: '0.0.0-test', pid: 1, startedAt: '2026-11-09T09:00:00Z', dataDir: 'C:\\data', uptimeSeconds: 0 }
    case 'AgentElevated':
      return true
    case 'Catalog':
      return {
        platforms: [{ id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 3 }],
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
      return { media: [] }
    case 'EmulatorList':
      return { emulators: [] }
    case 'ProviderKeyStatus':
      return { provider: args[0], configured: false }
    case 'StatsSummary':
      return { games: [], platforms: [], totalSeconds: 0, importedSeconds: 0 }
    case 'StatsWeekly':
      return { weeks: [] }
    case 'TakeBigBoxRequest':
      return false
    default:
      return undefined
  }
}

function install() {
  spies = new Map()
  overrides = {}
  handlers = new Map()
  offSpies = []
  const target = new Proxy(
    {},
    {
      get(_t, prop) {
        if (typeof prop !== 'string' || prop === 'then') return undefined
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
  w.runtime = {
    EventsOn: vi.fn((name: string, cb: (...d: unknown[]) => void) => {
      const set = handlers.get(name) ?? new Set()
      handlers.set(name, set)
      set.add(cb)
      const off = vi.fn(() => {
        set.delete(cb)
      })
      offSpies.push(off)
      return off
    }),
  }
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
  rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () => ({ x: 0, y: 0, top: 0, left: 0, bottom: 600, right: 800, width: 800, height: 600, toJSON: () => ({}) }) as DOMRect,
  )
}

function uninstall() {
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
  rectSpy?.mockRestore()
  vi.unstubAllGlobals()
}

const calls = (name: string): unknown[][] => spies.get(name)?.mock.calls ?? []
const inBigBox = () => document.querySelector('[data-bigbox-theme]') !== null

// emitRequest sends the ui:bigBoxRequested event (no data).
const emitRequest = () =>
  act(async () => {
    for (const cb of handlers.get('ui:bigBoxRequested') ?? []) cb()
    await new Promise((r) => setTimeout(r, 20))
  })

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 20))
  })
}

// pendingRequest makes TakeBigBoxRequest answer true once, then false (ATB-012).
function pendingRequest() {
  let pending = true
  overrides.TakeBigBoxRequest = () => {
    const r = pending
    pending = false
    return r
  }
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
const waitCards = () => waitFor(() => expect(cards().length).toBe(3))
const topBar = () => screen.getByRole('heading', { level: 1, name: 'Arcadia' }).closest('header') as HTMLElement
const aside = () => screen.getByRole('complementary', { name: 'Detalhes do jogo' })

async function openBigBoxByF11() {
  await waitCards()
  fireEvent.keyDown(window, { key: 'F11' })
  await waitFor(() => expect(inBigBox()).toBe(true))
  await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThan(0))
}

beforeEach(() => {
  localStorage.clear()
  install()
})
afterEach(() => {
  cleanup()
  uninstall()
})

describe('pedido de Big Box (fase 15)', () => {
  it('ATB-008/ao montar com TakeBigBoxRequest = true o App entra no Big Box e chama BringToFront uma vez', async () => {
    pendingRequest()
    renderApp()
    await waitFor(() => expect(inBigBox()).toBe(true))
    await flush()
    expect(calls('BringToFront')).toHaveLength(1)
    expect(calls('TakeBigBoxRequest').length).toBeGreaterThanOrEqual(1)
  })

  it('ATB-008/com TakeBigBoxRequest = false abre a janela normal, sem Big Box e sem BringToFront', async () => {
    renderApp()
    await waitCards()
    await flush()
    expect(calls('TakeBigBoxRequest').length).toBeGreaterThanOrEqual(1)
    expect(inBigBox()).toBe(false)
    expect(calls('BringToFront')).toHaveLength(0)
    expect(screen.getByRole('heading', { level: 1, name: 'Arcadia' })).toBeInTheDocument()
  })

  it('ATB-012/o pedido chega antes da montagem: atendido na montagem; evento e montagem atendendo o mesmo pedido entram uma vez só', async () => {
    pendingRequest()
    renderApp()
    // The event arrives while the mount reading is still in flight: the request is consumed once.
    await emitRequest()
    await emitRequest()
    await flush()
    expect(inBigBox()).toBe(true)
    expect(calls('BringToFront')).toHaveLength(1)
  })

  it('ATB-009/o evento ui:bigBoxRequested consome o pedido (TakeBigBoxRequest) e entra no Big Box', async () => {
    renderApp()
    await waitCards()
    await flush()
    expect(inBigBox()).toBe(false)
    const before = calls('TakeBigBoxRequest').length
    pendingRequest()
    await emitRequest()
    await waitFor(() => expect(inBigBox()).toBe(true))
    expect(calls('TakeBigBoxRequest').length).toBe(before + 1)
    expect(calls('BringToFront')).toHaveLength(1)
  })

  it('ATB-009/evento com TakeBigBoxRequest = false (pedido já atendido) não entra no Big Box', async () => {
    renderApp()
    await waitCards()
    await flush()
    await emitRequest()
    expect(inBigBox()).toBe(false)
    expect(calls('BringToFront')).toHaveLength(0)
  })

  it('ATB-009/o listener de ui:bigBoxRequested é removido ao desmontar', async () => {
    const { unmount } = renderApp()
    await waitCards()
    expect(handlers.get('ui:bigBoxRequested')?.size).toBe(1)
    unmount()
    expect(handlers.get('ui:bigBoxRequested')?.size ?? 0).toBe(0)
    const registered = (window as unknown as { runtime: { EventsOn: Spy } }).runtime.EventsOn.mock.calls
      .map((c) => c[0])
      .indexOf('ui:bigBoxRequested')
    expect(registered).toBeGreaterThanOrEqual(0)
    expect(offSpies.some((o) => o.mock.calls.length > 0)).toBe(true)
  })

  it('ATB-010/já no Big Box (Navegar) o pedido só traz a janela à frente: continua no Big Box, sem sair nem reentrar', async () => {
    renderApp()
    await openBigBoxByF11()
    const theme = document.querySelector('[data-bigbox-theme]')?.getAttribute('data-bigbox-theme')
    const coversBefore = screen.queryAllByTestId('bb-cover').length
    pendingRequest()
    await emitRequest()
    await flush()
    expect(inBigBox()).toBe(true)
    expect(document.querySelector('[data-bigbox-theme]')?.getAttribute('data-bigbox-theme')).toBe(theme)
    expect(screen.queryAllByTestId('bb-cover').length).toBe(coversBefore)
    expect(screen.queryByText('Voltar')).toBeNull()
    expect(calls('BringToFront')).toHaveLength(1)
  })

  it('ATB-010/já nos Detalhes a tela de Detalhes continua e a janela vem à frente', async () => {
    renderApp()
    await openBigBoxByF11()
    fireEvent.keyDown(window, { key: 'd' })
    await screen.findByText('Voltar')
    pendingRequest()
    await emitRequest()
    await flush()
    expect(inBigBox()).toBe(true)
    expect(screen.getByText('Voltar')).toBeInTheDocument()
    expect(calls('BringToFront')).toHaveLength(1)
  })

  it('ATB-010/o pedido não lança jogo nem fecha o Big Box (um Escape depois ainda sai)', async () => {
    renderApp()
    await openBigBoxByF11()
    pendingRequest()
    await emitRequest()
    expect(calls('LaunchGame')).toHaveLength(0)
    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => expect(inBigBox()).toBe(false))
  })

  describe('ATB-011/com uma janela modal aberta o Big Box não abre', () => {
    const topModals = ['Provedores', 'Emuladores', 'Plataformas']
    it.each(topModals)('ATB-011/%s: a janela fica, o pedido é consumido e BringToFront é chamado', async (name) => {
      renderApp()
      await waitCards()
      fireEvent.click(within(topBar()).getByRole('button', { name }))
      const dialog = await screen.findByRole('dialog')
      pendingRequest()
      await emitRequest()
      await flush()
      expect(inBigBox()).toBe(false)
      expect(screen.getByRole('dialog')).toBe(dialog)
      expect(calls('TakeBigBoxRequest').length).toBeGreaterThanOrEqual(1)
      expect(calls('BringToFront')).toHaveLength(1)
    })

    it.each(['Editar', 'Buscar metadados', 'Remover'])('ATB-011/%s (janela do jogo em foco): continua aberta', async (name) => {
      renderApp()
      await waitCards()
      fireEvent.click(within(cards()[1]).getByText('Hades'))
      await within(aside()).findByRole('heading', { level: 2, name: 'Hades' })
      fireEvent.click(await within(aside()).findByRole('button', { name }))
      const dialog = await screen.findByRole('dialog')
      pendingRequest()
      await emitRequest()
      await flush()
      expect(inBigBox()).toBe(false)
      expect(screen.getByRole('dialog')).toBe(dialog)
      expect(calls('BringToFront')).toHaveLength(1)
    })

    it('ATB-011/o campo digitado na janela de edição continua com o mesmo valor', async () => {
      renderApp()
      await waitCards()
      fireEvent.click(within(cards()[1]).getByText('Hades'))
      await within(aside()).findByRole('heading', { level: 2, name: 'Hades' })
      fireEvent.click(await within(aside()).findByRole('button', { name: 'Editar' }))
      const dialog = await screen.findByRole('dialog')
      const field = (await within(dialog).findAllByRole('textbox'))[0] as HTMLInputElement
      fireEvent.change(field, { target: { value: 'Hades II rascunho' } })
      pendingRequest()
      await emitRequest()
      await flush()
      expect(inBigBox()).toBe(false)
      expect((within(screen.getByRole('dialog')).getAllByRole('textbox')[0] as HTMLInputElement).value).toBe('Hades II rascunho')
    })

    it('ATB-011/depois de fechar a modal, um pedido novo entra no Big Box', async () => {
      renderApp()
      await waitCards()
      fireEvent.click(within(topBar()).getByRole('button', { name: 'Provedores' }))
      const dialog = await screen.findByRole('dialog')
      pendingRequest()
      await emitRequest()
      expect(inBigBox()).toBe(false)
      fireEvent.keyDown(dialog, { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      pendingRequest()
      await emitRequest()
      await waitFor(() => expect(inBigBox()).toBe(true))
    })

    it('ATB-011/com a página de estatísticas aberta e nenhuma modal, entra no Big Box e, ao sair, ela volta', async () => {
      renderApp()
      await waitCards()
      fireEvent.click(screen.getByRole('button', { name: 'Estatísticas' }))
      await screen.findByRole('heading', { name: 'Estatísticas' })
      pendingRequest()
      await emitRequest()
      await waitFor(() => expect(inBigBox()).toBe(true))
      fireEvent.keyDown(window, { key: 'Escape' })
      await waitFor(() => expect(inBigBox()).toBe(false))
      expect(await screen.findByRole('heading', { name: 'Estatísticas' })).toBeInTheDocument()
    })
  })

  it('ATB-013/o botão "Big Box" tem title "Big Box (F11 ou Select + A no controle)"', async () => {
    renderApp()
    await waitCards()
    expect(within(topBar()).getByRole('button', { name: /^Big Box/ })).toHaveAttribute(
      'title',
      'Big Box (F11 ou Select + A no controle)',
    )
  })

  it('ATB-013/o rodapé do Big Box não mostra dica de Select + A', async () => {
    renderApp()
    await openBigBoxByF11()
    expect(document.body.textContent).not.toMatch(/Select\s*\+\s*A\b/)
    fireEvent.keyDown(window, { key: 'd' })
    await screen.findByText('Voltar')
    expect(document.body.textContent).not.toMatch(/Select\s*\+\s*A\b/)
  })
})
