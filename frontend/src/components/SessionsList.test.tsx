import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SessionsList } from './SessionsList'
import type { SessionView } from '../lib/games'
import { libraryKeys } from '../lib/library'
import type { GameDetailView, SessionItemView, SessionListView } from '../lib/library'
import { formatLocalDateTime } from '../lib/formatDate'

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
    source: 'tracked', // alterado em 2026-11-25 (fase 16)
    editedAt: '', // alterado em 2026-11-25 (fase 16)
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

// alterado em 2026-11-25 (fase 16): a prop game é nova e obrigatória.
function gameDetail(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: GAME_ID,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'PC',
    path: 'C:\\Games\\HK\\hk.exe',
    status: 'playing',
    favorite: false,
    year: 2017,
    totalPlaySeconds: 3600,
    playCount: 1,
    firstPlayedAt: '',
    lastPlayedAt: '',
    description: '',
    completedAt: null,
    developers: [],
    publishers: [],
    genres: [],
    playModes: [],
    launchType: 'exe',
    launchArgs: '',
    emulatorId: null,
    sessionPlaySeconds: 3600,
    ...over,
  }
}

function renderList(current: SessionView | null = null, qc: QueryClient = newClient(), game: GameDetailView = gameDetail()) {
  const Wrapper = wrapperFor(qc)
  return render(<SessionsList gameId={GAME_ID} current={current} game={game} />, { wrapper: Wrapper })
}

// Fake agent with a mutable list: writes change what GameSessions serves next.
function installWritable(initial: SessionItemView[]) {
  const store = [...initial]
  const app = {
    GameSessions: vi.fn((_id: number, offset: number, limit: number): Promise<SessionListView> =>
      Promise.resolve({ sessions: store.slice(offset, offset + limit), total: store.length, offset }),
    ),
    CreateSession: vi.fn(),
    UpdateSession: vi.fn(),
    DeleteSession: vi.fn(),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
  return { store, app }
}

const label = (iso: string) => formatLocalDateTime(iso)
const T1 = '2026-11-12T20:00:00.000Z'
const T2 = '2026-11-11T20:00:00.000Z'

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

  // ------------------------------------------------------------------------
  // Phase 16 (edicao-sessoes): SEL-001..SEL-004, SEL-010..SEL-013, SEL-015
  // ------------------------------------------------------------------------

  const addButton = () => screen.getByRole('button', { name: 'Adicionar sessão' })

  it('SEL-001/o botão "Adicionar sessão" aparece com sessões, com a lista vazia e com o jogo rodando', async () => {
    install([item()])
    const first = renderList()
    await screen.findByText('1 h 00 min')
    expect(addButton()).toBeInTheDocument()
    first.unmount()

    install([])
    const empty = renderList()
    expect(await screen.findByText('Nenhuma sessão registrada.')).toBeInTheDocument()
    expect(addButton()).toBeInTheDocument()
    empty.unmount()

    install([item({ sessionId: 5, state: 'running', endedAt: null, endReason: null })])
    renderList({
      sessionId: 5,
      gameId: GAME_ID,
      title: 'Hollow Knight',
      state: 'running',
      startedAt: '2026-11-09T10:00:00.000Z',
      endedAt: '',
      activeSeconds: 10,
      activeMs: 10000,
      asOf: '2026-11-09T10:00:10.000Z',
    })
    await screen.findByText('Em andamento')
    expect(addButton()).toBeInTheDocument()
  })

  it('SEL-001/o botão abre o formulário de criação', async () => {
    install([])
    renderList()
    await screen.findByText('Nenhuma sessão registrada.')
    fireEvent.click(addButton())
    expect(screen.getByRole('dialog', { name: 'Adicionar sessão' })).toBeInTheDocument()
  })

  it('SEL-002/contada mostra "Editar" e "Excluir"; descartada só "Excluir"; a aberta e o SessionPanel nenhum', async () => {
    install([
      item({ sessionId: 1, startedAt: T1 }),
      item({ sessionId: 2, startedAt: T2, state: 'discarded', endReason: null }),
      item({ sessionId: 3, startedAt: '2026-11-10T20:00:00.000Z', state: 'running', endedAt: null, endReason: null }),
    ])
    renderList({
      sessionId: 3,
      gameId: GAME_ID,
      title: 'Hollow Knight',
      state: 'running',
      startedAt: '2026-11-10T20:00:00.000Z',
      endedAt: '',
      activeSeconds: 10,
      activeMs: 10000,
      asOf: '2026-11-10T20:00:10.000Z',
    })
    await screen.findByText('Descartada')
    expect(screen.getAllByRole('button', { name: /^Editar sessão de / })).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: /^Excluir sessão de / })).toHaveLength(2)
    expect(screen.queryByRole('button', { name: `Editar sessão de ${label(T2)}` })).toBeNull()
    expect(screen.getByRole('button', { name: `Excluir sessão de ${label(T2)}` })).toBeInTheDocument()
    const panel = screen.getByRole('status', { name: 'Sessão em andamento' })
    expect(within(panel).queryByRole('button', { name: /Editar|Excluir/ })).toBeNull()
  })

  it('SEL-002/os botões têm nome acessível com o início da sessão', async () => {
    install([item({ startedAt: T1 })])
    renderList()
    const edit = await screen.findByRole('button', { name: `Editar sessão de ${label(T1)}` })
    expect(edit).toHaveAttribute('aria-label', `Editar sessão de ${label(T1)}`)
    expect(screen.getByRole('button', { name: `Excluir sessão de ${label(T1)}` })).toBeInTheDocument()
  })

  it('SEL-003/marcas de origem: Manual, Importada, Editada, as duas e nenhuma', async () => {
    const day = (n: number) => `2026-11-0${n}T10:00:00.000Z`
    install([
      item({ sessionId: 1, startedAt: day(1), source: 'manual' }),
      item({ sessionId: 2, startedAt: day(2), source: 'import' }),
      item({ sessionId: 3, startedAt: day(3), source: 'tracked', editedAt: '2026-11-20T10:00:00.000Z' }),
      item({ sessionId: 4, startedAt: day(4), source: 'import', editedAt: '2026-11-20T10:00:00.000Z' }),
      item({ sessionId: 5, startedAt: day(5), source: 'tracked' }),
      item({ sessionId: 6, startedAt: day(6), source: 'legacy' as never }),
    ])
    renderList()
    await screen.findAllByRole('listitem')
    const row = (n: number) =>
      screen.getByRole('button', { name: `Excluir sessão de ${label(day(n))}` }).closest('li') as HTMLElement
    const marks = (n: number) => ['Manual', 'Importada', 'Editada'].filter((m) => within(row(n)).queryByText(m) !== null)
    expect(marks(1)).toEqual(['Manual'])
    expect(marks(2)).toEqual(['Importada'])
    expect(marks(3)).toEqual(['Editada'])
    expect(marks(4)).toEqual(['Importada', 'Editada'])
    expect(marks(5)).toEqual([])
    expect(marks(6)).toEqual([])
  })

  it('SEL-004/depois de criar, a lista mostra o que o agente devolveu, sem a UI somar tempo', async () => {
    const { store, app } = installWritable([item({ sessionId: 1, startedAt: T1 })])
    // The agent stores 1 h 30 min even though the form sends 1 h: the list must show the agent's value.
    app.CreateSession.mockImplementation(() => {
      const created = item({ sessionId: 2, startedAt: T2, activeSeconds: 5400, source: 'manual' })
      store.unshift(created)
      return Promise.resolve({ session: created, game: gameDetail({ totalPlaySeconds: 9000 }) })
    })
    renderList()
    await screen.findByText('1 h 00 min')
    fireEvent.click(addButton())
    fireEvent.click(await screen.findByRole('button', { name: 'Salvar' }))
    expect(await screen.findByText('1 h 30 min')).toBeInTheDocument()
    expect(screen.queryByText('2 h 30 min')).toBeNull()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('SEL-013/depois de uma escrita a lista é recarregada uma vez e o detalhe mostra o total do game da resposta', async () => {
    const { store, app } = installWritable([item({ sessionId: 1, startedAt: T1 })])
    app.DeleteSession.mockImplementation(() => {
      store.length = 0
      return Promise.resolve(gameDetail({ totalPlaySeconds: 0, sessionPlaySeconds: 0 }))
    })
    const qc = newClient()
    renderList(null, qc)
    await screen.findByText('1 h 00 min')
    expect(app.GameSessions).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: `Excluir sessão de ${label(T1)}` }))
    fireEvent.click(await screen.findByRole('button', { name: 'Excluir' }))
    await screen.findByText('Nenhuma sessão registrada.')
    expect(app.DeleteSession).toHaveBeenCalledTimes(1)
    expect(app.DeleteSession).toHaveBeenCalledWith(1)
    expect(app.GameSessions).toHaveBeenCalledTimes(2)
    expect((qc.getQueryData(libraryKeys.game(GAME_ID)) as GameDetailView).totalPlaySeconds).toBe(0)
  })

  it('SEL-012/Cancelar não chama o agente; foco inicial em Cancelar', async () => {
    const { app } = installWritable([item({ startedAt: T1 })])
    renderList()
    fireEvent.click(await screen.findByRole('button', { name: `Excluir sessão de ${label(T1)}` }))
    expect(screen.getByRole('button', { name: 'Cancelar' })).toHaveFocus()
    expect(
      screen.getByText(
        `Excluir a sessão de «${label(T1)}» («1 h 00 min»)? O tempo dela sai do tempo total do jogo. Esta ação não pode ser desfeita.`,
      ),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(app.DeleteSession).not.toHaveBeenCalled()
  })

  it('SEL-010/erro do agente na exclusão aparece em role="alert" e o diálogo continua aberto', async () => {
    const { app } = installWritable([item({ startedAt: T1 })])
    app.DeleteSession.mockRejectedValueOnce('Não é possível alterar uma sessão em andamento.')
    renderList()
    fireEvent.click(await screen.findByRole('button', { name: `Excluir sessão de ${label(T1)}` }))
    fireEvent.click(screen.getByRole('button', { name: 'Excluir' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Não é possível alterar uma sessão em andamento.')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('SEL-010/"Esta sessão não existe mais." na exclusão fecha o diálogo e recarrega a lista', async () => {
    const { app } = installWritable([item({ startedAt: T1 })])
    app.DeleteSession.mockRejectedValueOnce('Esta sessão não existe mais.')
    renderList()
    fireEvent.click(await screen.findByRole('button', { name: `Excluir sessão de ${label(T1)}` }))
    fireEvent.click(screen.getByRole('button', { name: 'Excluir' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(app.GameSessions).toHaveBeenCalledTimes(2))
  })

  it('SEL-011/durante a exclusão o botão mostra "Excluindo…" e um segundo clique não chama o agente de novo', async () => {
    const { app } = installWritable([item({ startedAt: T1 })])
    app.DeleteSession.mockImplementation(() => new Promise(() => {}))
    renderList()
    fireEvent.click(await screen.findByRole('button', { name: `Excluir sessão de ${label(T1)}` }))
    fireEvent.click(screen.getByRole('button', { name: 'Excluir' }))
    const busy = await screen.findByRole('button', { name: 'Excluindo…' })
    expect(busy).toBeDisabled()
    fireEvent.click(busy)
    expect(app.DeleteSession).toHaveBeenCalledTimes(1)
  })

  it('SEL-015/depois de excluir a linha, o foco vai para "Adicionar sessão"', async () => {
    const { store, app } = installWritable([item({ sessionId: 1, startedAt: T1 })])
    app.DeleteSession.mockImplementation(() => {
      store.length = 0
      return Promise.resolve(gameDetail({ totalPlaySeconds: 0, sessionPlaySeconds: 0 }))
    })
    renderList()
    const del = await screen.findByRole('button', { name: `Excluir sessão de ${label(T1)}` })
    del.focus()
    fireEvent.click(del)
    fireEvent.click(await screen.findByRole('button', { name: 'Excluir' }))
    await screen.findByText('Nenhuma sessão registrada.')
    await waitFor(() => expect(addButton()).toHaveFocus())
  })

  it('SEL-015/depois de salvar uma edição, o foco volta ao botão "Editar" da linha', async () => {
    const { store, app } = installWritable([item({ sessionId: 1, startedAt: T1, endedAt: '2026-11-12T21:00:00.000Z' })]) // alterado em 2026-11-25 (fase 16)
    app.UpdateSession.mockImplementation(() => {
      store[0] = { ...store[0], activeSeconds: 1800, editedAt: '2026-11-25T20:00:00.000Z' }
      return Promise.resolve({ session: store[0], game: gameDetail() })
    })
    renderList()
    const edit = await screen.findByRole('button', { name: `Editar sessão de ${label(T1)}` })
    edit.focus()
    fireEvent.click(edit)
    expect(screen.getByRole('dialog', { name: 'Editar sessão' })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/horas/i), { target: { value: '0' } })
    fireEvent.change(screen.getByLabelText(/minutos/i), { target: { value: '30' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await screen.findByText('30 min')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(app.UpdateSession).toHaveBeenCalledWith(1, { activeSeconds: 1800 })
    await waitFor(() => expect(screen.getByRole('button', { name: `Editar sessão de ${label(T1)}` })).toHaveFocus())
  })
})
