import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SessionsList } from './SessionsList'
import type { SessionView } from '../lib/games'
import type { SessionItemView, SessionListView } from '../lib/library'

// ---------------------------------------------------------------------------
// Fake for window.go.main.App.GameSessions (phase 5, IPC-047)
// ---------------------------------------------------------------------------

const GAME_ID = 10
const PAGE = 50

type GameSessionsFn = ReturnType<typeof vi.fn>

function item(over: Partial<SessionItemView> = {}): SessionItemView {
  return {
    sessionId: 1,
    gameId: GAME_ID,
    state: 'ended',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '2026-11-09T11:00:00.000Z',
    activeSeconds: 3600,
    endReason: 'exited',
    ...over,
  }
}

// total sessions of the game, served in pages of whatever offset/limit the UI asks for.
function install(all: SessionItemView[]): GameSessionsFn {
  const fn = vi.fn((_id: number, offset: number, limit: number): Promise<SessionListView> => {
    const page = all.slice(offset, offset + limit)
    return Promise.resolve({ sessions: page, total: all.length, offset })
  })
  ;(window as unknown as { go?: unknown }).go = { main: { App: { GameSessions: fn } } }
  return fn
}

function newClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function wrapperFor(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

function renderList(current: SessionView | null = null) {
  const Wrapper = wrapperFor(newClient())
  return render(<SessionsList gameId={GAME_ID} current={current} />, { wrapper: Wrapper })
}

function localDayIn(text: string, iso: string) {
  const d = new Date(iso)
  expect(text).toContain(String(d.getFullYear()))
  expect(text).toMatch(new RegExp(`(^|\\D)0?${d.getDate()}(\\D|$)`))
}

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('SessionsList', () => {
  it('UI-038/lista com 50 por página e \'Carregar mais\' pede a próxima página', async () => {
    const all = Array.from({ length: 120 }, (_, i) => item({ sessionId: i + 1 }))
    const fn = install(all)
    renderList()

    // The first page asks for 50 sessions from offset 0 and says nothing more.
    await screen.findByRole('button', { name: 'Carregar mais' })
    expect(fn).toHaveBeenNthCalledWith(1, GAME_ID, 0, PAGE)
    expect(fn).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: 'Carregar mais' }))
    await vi.waitFor(() => expect(fn).toHaveBeenCalledTimes(2))
    expect(fn).toHaveBeenNthCalledWith(2, GAME_ID, PAGE, PAGE)
  })

  it('UI-038/sessão discarded mostra \'Descartada\' e o tempo ativo, sem somar no total', async () => {
    install([
      item({ sessionId: 1, state: 'ended', activeSeconds: 3600 }),
      item({ sessionId: 2, state: 'discarded', activeSeconds: 600, endReason: null }),
    ])
    renderList()

    expect(await screen.findByText('Descartada')).toBeInTheDocument()
    // The discarded session shows its own active time (UI-005: 10 min)...
    expect(screen.getByText('10 min')).toBeInTheDocument()
    expect(screen.getByText('1 h 00 min')).toBeInTheDocument()
    // ...and nothing on the page adds it up (3600 + 600 = 1 h 10 min).
    expect(screen.queryByText('1 h 10 min')).toBeNull()
  })

  it('UI-038/sessão aberta mostra \'Em andamento\' e o cronômetro a partir de activeMs', async () => {
    // Only the clock and Date are faked: the list loads with the real timers.
    vi.useFakeTimers({ toFake: ['Date', 'performance'] })
    vi.setSystemTime(new Date('2026-11-09T10:04:12.400Z'))
    try {
      install([
        item({
          sessionId: 5,
          state: 'running',
          startedAt: '2026-11-09T10:00:00.000Z',
          endedAt: null,
          activeSeconds: 252,
          endReason: null,
        }),
      ])
      const current: SessionView = {
        sessionId: 5,
        gameId: GAME_ID,
        title: 'Hollow Knight',
        state: 'running',
        startedAt: '2026-11-09T10:00:00.000Z',
        endedAt: '',
        activeSeconds: 252,
        activeMs: 252400,
        asOf: '2026-11-09T10:04:12.400Z',
      }
      renderList(current)

      expect(await screen.findByText('Em andamento')).toBeInTheDocument()
      const panel = screen.getByRole('status', { name: 'Sessão em andamento' })
      expect(within(panel).getByText('0:04:12')).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('CON-018/endReason closed-by-hotkey mostra a duração e "fechado pelo controle"', async () => {
    install([item({ sessionId: 1, activeSeconds: 5400, endReason: 'closed-by-hotkey' })])
    renderList()
    expect(await screen.findByText('1 h 30 min')).toBeInTheDocument()
    // Premissa: texto "Fechado pelo controle" (CON-017), sem diferenciar maiúscula.
    expect(screen.getByText(/fechado pelo controle/i)).toBeInTheDocument()
  })

  it('CON-018/exited, agent-shutdown, recovered e client-no-launch não ganham texto novo', async () => {
    for (const endReason of ['exited', 'agent-shutdown', 'recovered', 'client-no-launch'] as const) {
      install([item({ endReason })])
      const { unmount } = renderList()
      await screen.findByText('1 h 00 min')
      expect(screen.queryByText(/fechado pelo controle/i), endReason).toBeNull()
      unmount()
    }
  })

  it('CON-018/sem endReason (sessão aberta) não mostra o texto', async () => {
    install([item({ state: 'running', endedAt: null, endReason: null })])
    renderList()
    await screen.findByText('Em andamento')
    expect(screen.queryByText(/fechado pelo controle/i)).toBeNull()
  })

  it('UI-038/cada linha mostra início, fim, tempo ativo em UI-005 e estado', async () => {
    const startedAt = '2026-11-08T20:15:00.000Z'
    const endedAt = '2026-11-08T21:45:00.000Z'
    install([item({ sessionId: 3, startedAt, endedAt, activeSeconds: 5400, state: 'ended' })])
    renderList()

    await screen.findByText('1 h 30 min')
    const text = document.body.textContent ?? ''
    localDayIn(text, startedAt)
    localDayIn(text, endedAt)
    // The state of an ended session is shown; its label is not fixed by the spec.
    expect(text).not.toContain('Descartada')
    expect(text).not.toContain('Em andamento')
  })
})
