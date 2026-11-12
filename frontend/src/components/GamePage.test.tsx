import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { GamePage } from './GamePage'
import type { SessionView } from '../lib/games'
import type { GameDetailView, SessionListView } from '../lib/library'

// ---------------------------------------------------------------------------
// Fakes for window.go.main.App (GetGame and GameSessions of cmd/arcadia, phase 5)
// ---------------------------------------------------------------------------

type App = {
  GetGame: ReturnType<typeof vi.fn>
  GameSessions: ReturnType<typeof vi.fn>
}

let app: App

function install(detail: GameDetailView) {
  app = {
    GetGame: vi.fn(() => Promise.resolve(detail)),
    GameSessions: vi.fn(
      (): Promise<SessionListView> => Promise.resolve({ sessions: [], total: 0, offset: 0 }),
    ),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
}

function detail(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: 10,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'PC',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    status: 'playing',
    favorite: true,
    year: 2017,
    totalPlaySeconds: 3900,
    playCount: 4,
    firstPlayedAt: '',
    lastPlayedAt: '',
    description: 'Um metroidvania difícil.',
    completedAt: null,
    developers: ['Team Cherry'],
    publishers: ['Team17'],
    genres: ['Metroidvania', 'Plataforma'],
    playModes: ['single'],
    ...over,
  }
}

function sessionView(over: Partial<SessionView> = {}): SessionView {
  return {
    sessionId: 1,
    gameId: 10,
    title: 'Hollow Knight',
    state: 'running',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '',
    activeSeconds: 252,
    activeMs: 252400,
    asOf: '2026-11-09T10:04:12.400Z',
    ...over,
  }
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

type PageOver = Partial<{
  id: number
  current: SessionView | null
  onBack: () => void
  onEdit: () => void
  onRemove: () => void
}>

function renderPage(over: PageOver = {}) {
  const props = {
    id: 10,
    current: null,
    onBack: vi.fn(),
    onEdit: vi.fn(),
    onRemove: vi.fn(),
    ...over,
  }
  const Wrapper = wrapperFor(newClient())
  const utils = render(<GamePage {...props} />, { wrapper: Wrapper })
  return { ...utils, props }
}

// The page is loaded when the title shows up.
async function loaded(title: string): Promise<HTMLElement> {
  await screen.findByText(title)
  return document.body
}

// The day, month and year of an ISO instant in the local zone must appear in the page text.
function expectLocalDate(text: string, iso: string) {
  const d = new Date(iso)
  expect(text).toContain(String(d.getFullYear()))
  expect(text).toMatch(new RegExp(`(^|\\D)0?${d.getDate()}(\\D|$)`))
}

function precedes(a: Node, b: Node): boolean {
  return (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
}

describe('GamePage', () => {
  beforeEach(() => {
    install(detail())
  })

  afterEach(() => {
    delete (window as unknown as { go?: unknown }).go
  })

  it('UI-037/mostra título, plataforma, gêneros, modos, desenvolvedora, editora, ano, tempo total, status, favorito e descrição', async () => {
    renderPage()
    await loaded('Hollow Knight')

    expect(screen.getByText('PC')).toBeInTheDocument()
    expect(screen.getByText('Metroidvania')).toBeInTheDocument()
    expect(screen.getByText('Plataforma')).toBeInTheDocument()
    // Play mode label is not fixed by the spec; see the report on ambiguities.
    expect(screen.getByText('Team Cherry')).toBeInTheDocument()
    expect(screen.getByText('Team17')).toBeInTheDocument()
    expect(screen.getByText('2017')).toBeInTheDocument()
    // 3900 s = 1 h 05 min, the format of UI-005.
    expect(screen.getByText('1 h 05 min')).toBeInTheDocument()
    expect(screen.getByText('Jogando')).toBeInTheDocument()
    expect(screen.getByText(/favorito/i)).toBeInTheDocument()
    expect(screen.getByText('Um metroidvania difícil.')).toBeInTheDocument()
  })

  it('UI-037/primeira e última vez jogado mostram \'Nunca\' quando vazias e data local quando há', async () => {
    const { unmount } = renderPage()
    await loaded('Hollow Knight')
    // Both empty: two 'Nunca' values, one for first and one for last played.
    expect(screen.getAllByText('Nunca')).toHaveLength(2)
    unmount()

    const first = '2026-11-01T10:00:00.000Z'
    const last = '2026-11-09T10:00:00.000Z'
    install(detail({ firstPlayedAt: first, lastPlayedAt: last }))
    renderPage()
    await loaded('Hollow Knight')
    expect(screen.queryByText('Nunca')).toBeNull()
    const text = document.body.textContent ?? ''
    expectLocalDate(text, first)
    expectLocalDate(text, last)
  })

  it('UI-037/data de conclusão sai em UTC', async () => {
    // completedAt arrives as YYYY-MM-DD (IPC-044, stored as midnight UTC); it must show the same day.
    install(detail({ completedAt: '2026-11-12' }))
    renderPage()
    await loaded('Hollow Knight')
    expect(screen.getByText(/2026-11-12/)).toBeInTheDocument()
  })

  it('UI-037/a página não tem img nem video', async () => {
    const { container } = renderPage()
    await loaded('Hollow Knight')
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('video')).toBeNull()
  })

  it('UI-037/\'Voltar\' chama onBack; a marcação de sessão aparece no topo quando o jogo está em sessão', async () => {
    const { props } = renderPage({ current: sessionView({ state: 'running' }) })
    const title = await screen.findByText('Hollow Knight')
    const marking = screen.getByText('Em execução')
    expect(precedes(marking, title)).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Voltar' }))
    expect(props.onBack).toHaveBeenCalledTimes(1)
  })

  it('UI-006/marcação de sessão aparece no topo da página quando o jogo está em sessão', async () => {
    const cases: Array<[SessionView['state'], string]> = [
      ['running', 'Em execução'],
      ['grace', 'Em execução'],
      ['paused', 'Pausado'],
    ]
    for (const [state, label] of cases) {
      const { unmount } = renderPage({ current: sessionView({ state }) })
      const title = await screen.findByText('Hollow Knight')
      const marking = screen.getByText(label)
      expect(precedes(marking, title), `${state}: marking above the title`).toBe(true)
      unmount()
    }

    // A session of another game does not mark this page.
    renderPage({ current: sessionView({ gameId: 99, state: 'running' }) })
    await screen.findByText('Hollow Knight')
    expect(screen.queryByText('Em execução')).toBeNull()
  })

  it('UI-039/botão Editar chama onEdit uma vez (o formulário é aberto pelo pai)', async () => {
    const { props } = renderPage()
    await loaded('Hollow Knight')
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    expect(props.onEdit).toHaveBeenCalledTimes(1)
  })

  it('UI-041/botão Remover chama onRemove uma vez (só a entrada; a confirmação é a T33)', async () => {
    const { props } = renderPage()
    await loaded('Hollow Knight')
    fireEvent.click(screen.getByRole('button', { name: 'Remover' }))
    expect(props.onRemove).toHaveBeenCalledTimes(1)
    expect(props.onEdit).not.toHaveBeenCalled()
  })
})
