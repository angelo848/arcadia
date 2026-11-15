import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'
import type { MetadataCandidateView } from './lib/metadata'
import type { BatchRowView, BatchStatusView } from './lib/metadataBatch'
import type { ImportCandidate } from './lib/imports'

// App-level tests of the batch of unified metadata (phase 12, T38; MET-053..067): the header indicator,
// the automatic opening of the confirmation window, and the invalidations at the end of the apply.
// Fakes of window.go.main.App and window.runtime.EventsOn, as in App.test.tsx. The batch lives in `batch`:
// the fake MetadataBatchStatus answers it, and emit('metadata:batch') is the notification of a change.

type Fn = ReturnType<typeof vi.fn>

let app: Record<string, Fn>
let handlers: Map<string, Set<(...data: unknown[]) => void>>
let batch: BatchStatusView | null

const BOX_WIDTH = 800
const BOX_HEIGHT = 600
let rectSpy: ReturnType<typeof vi.spyOn> | undefined

class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

function status(connected: boolean) {
  return {
    connected,
    version: connected ? '0.14.0-test' : '',
    pid: connected ? 4242 : 0,
    startedAt: connected ? '2026-11-15T09:00:00Z' : '',
    dataDir: connected ? 'C:\\Users\\test\\AppData\\Local\\Arcadia' : '',
    uptimeSeconds: 0,
  }
}

function install() {
  handlers = new Map()
  batch = null
  app = {
    AgentStatus: vi.fn(() => Promise.resolve(status(true))),
    Catalog: vi.fn(() =>
      Promise.resolve({
        platforms: [{ id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 0 }],
        genres: [],
        companies: [],
        playModes: [],
      }),
    ),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    GameSessions: vi.fn(() => Promise.resolve({ sessions: [], total: 0, offset: 0 })),
    ListGames: vi.fn(() => Promise.resolve({ games: [], total: 0, offset: 0 })),
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
    ProviderKeyStatus: vi.fn((provider: string) => Promise.resolve({ provider, configured: true })),
    MetadataBatchStatus: vi.fn(() => Promise.resolve({ batch })),
    MetadataBatchStart: vi.fn(() => Promise.resolve({ batchId: 7, total: 3 })),
    MetadataBatchResults: vi.fn((_id: number, _offset: number, _limit: number) =>
      Promise.resolve({ rows: reviewRows, total: reviewRows.length, next: null }),
    ),
    MetadataBatchApply: vi.fn((_id: number, _choices: unknown[]) => {
      if (batch) {
        batch = { ...batch, state: 'applying', done: 0, total: 1, failed: 0 }
      }
      return Promise.resolve(1)
    }),
    MetadataBatchCancel: vi.fn(() => {
      if (batch) {
        batch = { ...batch, state: batch.state === 'searching' ? 'cancelled' : batch.state }
      }
      return Promise.resolve(batch?.state ?? 'cancelled')
    }),
    MetadataBatchDiscard: vi.fn(() => {
      batch = null
      return Promise.resolve()
    }),
    ImportScan: vi.fn(() => Promise.resolve({ candidates: [], warnings: [] })),
    ImportApply: vi.fn(() => Promise.resolve({ applied: [], skipped: [] })),
    PickFolder: vi.fn(() => Promise.resolve('')),
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

// announce sends the notification of a batch change, as the agent does (MET-054).
function announce(b: BatchStatusView) {
  emit('metadata:batch', { batchId: b.batchId, state: b.state, done: b.done, total: b.total, failed: b.failed })
}

const cand = (title: string, score = 0.95): MetadataCandidateView => ({
  refs: [{ provider: 'igdb', providerId: '1' }],
  title,
  year: 2017,
  platform: 'PC',
  score,
})

const row = (gameId: number, title: string): BatchRowView => ({
  gameId,
  title,
  platform: 'PC',
  confidence: 'match',
  selected: 0,
  // The found title differs from the row title, so each text is unique in the table.
  candidates: [cand(`${title} (encontrado)`)],
})

let reviewRows: BatchRowView[] = []

const batchOf = (over: Partial<BatchStatusView> = {}): BatchStatusView => ({
  batchId: 7,
  state: 'searching',
  total: 3,
  done: 0,
  failed: 0,
  warnings: [],
  ...over,
})

function renderApp() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  render(<App />, { wrapper })
  return qc
}

function indicatorState(): string | null {
  return document.querySelector('[data-state]')?.getAttribute('data-state') ?? null
}

async function waitConnected() {
  await waitFor(() => expect(indicatorState()).toBe('connected'))
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

function reviewDialog(): HTMLElement | null {
  return screen.queryByRole('dialog', { name: 'Confirmar metadados' })
}

function importDialog(): HTMLElement | null {
  return screen.queryByRole('dialog', { name: 'Importar jogos' })
}

// header buttons open the other modals [UIX-*]; the names are the labels of the header.
function openHeader(label: string) {
  fireEvent.click(screen.getByRole('button', { name: label }))
}

// findChip waits for the text of the batch indicator in the header [MET-067].
async function findChip(text: RegExp | string): Promise<HTMLElement> {
  return screen.findByText(text)
}

beforeEach(() => {
  install()
})

afterEach(() => {
  uninstall()
})

describe('MET-067 abertura automática e indicador', () => {
  it('MET-067/transição para ready sem modal abre a confirmação sozinha', async () => {
    batch = batchOf({ state: 'searching', done: 1 })
    reviewRows = [row(10, 'Hollow Knight')]
    renderApp()
    await waitConnected()
    await findChip('Buscando metadados 1/3')
    expect(reviewDialog()).toBeNull()

    batch = batchOf({ state: 'ready', done: 3 })
    announce(batch)

    expect(await screen.findByRole('dialog', { name: 'Confirmar metadados' })).toBeInTheDocument()
    expect(app.MetadataBatchResults).toHaveBeenCalledWith(7, 0, expect.any(Number))
  })

  it('MET-067/ready já visto ao reabrir o app (primeira leitura) não reabre sozinho', async () => {
    batch = batchOf({ state: 'ready', done: 3 })
    reviewRows = [row(10, 'Hollow Knight')]
    renderApp()
    await waitConnected()
    await findChip(/Revisar metadados \(3\)/)

    expect(reviewDialog()).toBeNull()
    expect(app.MetadataBatchResults).not.toHaveBeenCalled()
  })

  it('MET-067/só com a janela de importação aberta e sem operação: ela fecha e a confirmação abre', async () => {
    batch = batchOf({ state: 'searching', done: 1 })
    reviewRows = [row(10, 'Hollow Knight')]
    renderApp()
    await waitConnected()
    openHeader('Importar')
    expect(importDialog()).not.toBeNull()

    batch = batchOf({ state: 'ready', done: 3 })
    announce(batch)

    expect(await screen.findByRole('dialog', { name: 'Confirmar metadados' })).toBeInTheDocument()
    expect(importDialog()).toBeNull()
  })

  it('MET-067/janela de importação em operação: não abre', async () => {
    batch = batchOf({ state: 'searching', done: 1 })
    reviewRows = [row(10, 'Hollow Knight')]
    // The scan stays in flight until the test ends, so the import dialog is busy.
    let finish!: (v: unknown) => void
    app.ImportScan.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    renderApp()
    await waitConnected()
    openHeader('Importar')
    fireEvent.click(within(importDialog() as HTMLElement).getByRole('button', { name: 'Buscar' }))
    await waitFor(() => expect(app.ImportScan).toHaveBeenCalled())
    await waitFor(() => expect(screen.getByRole('button', { name: 'Buscar' })).toBeDisabled())

    batch = batchOf({ state: 'ready', done: 3 })
    announce(batch)

    await findChip(/Revisar metadados \(3\)/)
    expect(reviewDialog()).toBeNull()
    expect(importDialog()).not.toBeNull()
    finish({ candidates: [], warnings: [] })
  })

  it('MET-067/outro modal aberto: não abre, só o indicador muda', async () => {
    batch = batchOf({ state: 'searching', done: 1 })
    reviewRows = [row(10, 'Hollow Knight')]
    renderApp()
    await waitConnected()
    openHeader('Provedores')
    expect(screen.getByRole('dialog', { name: 'Chaves dos provedores' })).toBeInTheDocument()

    batch = batchOf({ state: 'ready', done: 3 })
    announce(batch)

    await findChip(/Revisar metadados \(3\)/)
    expect(reviewDialog()).toBeNull()
    expect(screen.getByRole('dialog', { name: 'Chaves dos provedores' })).toBeInTheDocument()
  })

  it('MET-067/indicador mostra "Buscando metadados k/n" e Cancelar, e Cancelar chama MetadataBatchCancel', async () => {
    batch = batchOf({ state: 'searching', done: 2, total: 3 })
    renderApp()
    await waitConnected()

    await findChip('Buscando metadados 2/3')
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))

    await waitFor(() => expect(app.MetadataBatchCancel).toHaveBeenCalledWith(7))
  })

  it('MET-067/agente reiniciado mostra a interrupção uma vez', async () => {
    batch = batchOf({ state: 'searching', done: 1 })
    renderApp()
    await waitConnected()
    await findChip('Buscando metadados 1/3')

    batch = null
    await disconnectAgent()
    await reconnectAgent()

    const notice = await screen.findByText('A busca de metadados foi interrompida. Use Buscar metadados no jogo.')
    expect(notice).toBeInTheDocument()
    expect(screen.queryByText(/Buscando metadados/)).toBeNull()
    expect(screen.getAllByText('A busca de metadados foi interrompida. Use Buscar metadados no jogo.')).toHaveLength(1)
  })

  it('MET-067/com outro modal aberto, o indicador pisca uma vez ao chegar em ready (data-attention) e a janela não abre', async () => {
    batch = batchOf({ state: 'searching', done: 1 })
    reviewRows = [row(10, 'Hollow Knight')]
    renderApp()
    await waitConnected()
    openHeader('Provedores')
    expect(screen.getByRole('dialog', { name: 'Chaves dos provedores' })).toBeInTheDocument()

    batch = batchOf({ state: 'ready', done: 3 })
    announce(batch)

    await findChip(/Revisar metadados \(3\)/)
    const mark = document.querySelector('[data-attention]')
    expect(mark).toHaveAttribute('data-attention', 'true')
    expect(reviewDialog()).toBeNull()

    // The highlight ends once (animationend); the same state read again does not blink again.
    fireEvent.animationEnd(mark as Element)
    await waitFor(() => expect(document.querySelector('[data-attention]')).toBeNull())
    announce(batch)
    expect(document.querySelector('[data-attention]')).toBeNull()
  })

  it('MET-067/reconexão relê o estado (MET-055)', async () => {
    batch = batchOf({ state: 'searching', done: 1 })
    renderApp()
    await waitConnected()
    await findChip('Buscando metadados 1/3')
    const reads = app.MetadataBatchStatus.mock.calls.length

    batch = batchOf({ state: 'applying', done: 2 })
    await disconnectAgent()
    await reconnectAgent()

    await findChip('Aplicando metadados 2/3')
    expect(app.MetadataBatchStatus.mock.calls.length).toBeGreaterThan(reads)
  })
})

// folderCandidate is a candidate of the "Pasta" source, as ImportScan returns it (IMV-016).
function folderCandidate(file: string, title: string): ImportCandidate {
  return {
    source: 'folder',
    title,
    platform: '',
    launchType: 'emulator',
    launchTarget: `E:\\Games\\ps1\\${file}`,
    installDir: 'E:\\Games\\ps1',
    developers: [],
    publishers: [],
    genres: [],
    year: null,
    description: '',
    playModes: [],
    playedSeconds: null,
    lastPlayed: null,
    exists: false,
    file,
    extension: 'cue',
    discs: 1,
  }
}

describe('MET-050 e MET-053 busca iniciada pela janela de importação', () => {
  it('MET-053/depois de batchStart pelo ImportDialog, o indicador mostra "Buscando metadados 0/n" sem esperar notificação', async () => {
    batch = null
    app.ImportScan.mockResolvedValueOnce({
      candidates: [folderCandidate('Spyro.cue', 'Spyro'), folderCandidate('Crash.cue', 'Crash')],
      warnings: [],
    })
    app.PickFolder.mockResolvedValueOnce('E:\\Games\\ps1')
    app.ImportApply.mockResolvedValueOnce({
      applied: [
        { gameId: 10, title: 'Spyro' },
        { gameId: 11, title: 'Crash' },
      ],
      skipped: [],
    })
    // The agent creates the batch and answers the status read; no metadata:batch notification is emitted.
    app.MetadataBatchStart.mockImplementationOnce((ids: number[]) => {
      batch = batchOf({ state: 'searching', done: 0, total: ids.length })
      return Promise.resolve({ batchId: 7, total: ids.length })
    })
    renderApp()
    await waitConnected()

    openHeader('Importar')
    const dialog = importDialog() as HTMLElement
    fireEvent.change(within(dialog).getByLabelText('Fonte'), { target: { value: 'folder' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Escolher pasta' }))
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Pasta' })).toHaveValue('E:\\Games\\ps1'))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Buscar' }))
    await within(dialog).findByRole('table')
    // The box needs the provider keys, read when the dialog opens; wait for it before importing.
    await waitFor(() => expect(within(dialog).getByRole('checkbox', { name: 'Buscar metadados e imagens' })).toBeEnabled())

    fireEvent.click(within(dialog).getByRole('button', { name: 'Importar selecionados' }))

    await waitFor(() => expect(app.MetadataBatchStart).toHaveBeenCalledWith([10, 11]))
    expect(await screen.findByText('Buscando metadados 0/2', undefined, { timeout: 3000 })).toBeInTheDocument()
  })
})

describe('MET-064 e MET-066 confirmação e resumo', () => {
  it('MET-064/clique no indicador abre a confirmação e Aplicar selecionados envia só as linhas marcadas e fecha', async () => {
    batch = batchOf({ state: 'ready', done: 3, total: 3 })
    reviewRows = [row(10, 'Hollow Knight'), row(11, 'Celeste')]
    renderApp()
    await waitConnected()

    fireEvent.click(await screen.findByRole('button', { name: /Revisar metadados \(3\)/ }))
    const dialog = (await screen.findByRole('dialog', { name: 'Confirmar metadados' })) as HTMLElement
    // Both rows are "match", so both are marked; unmark the second one.
    await within(dialog).findByText('Hollow Knight')
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Selecionar Celeste' }))

    fireEvent.click(within(dialog).getByRole('button', { name: 'Aplicar selecionados' }))

    await waitFor(() => expect(app.MetadataBatchApply).toHaveBeenCalledTimes(1))
    expect(app.MetadataBatchApply).toHaveBeenCalledWith(7, [{ gameId: 10, candidate: 0 }])
    await waitFor(() => expect(reviewDialog()).toBeNull())
  })

  it('MET-064/Descartar tudo chama MetadataBatchDiscard e fecha a janela', async () => {
    batch = batchOf({ state: 'ready', done: 3, total: 3 })
    reviewRows = [row(10, 'Hollow Knight')]
    renderApp()
    await waitConnected()

    fireEvent.click(await screen.findByRole('button', { name: /Revisar metadados \(3\)/ }))
    const dialog = (await screen.findByRole('dialog', { name: 'Confirmar metadados' })) as HTMLElement
    await within(dialog).findByText('Hollow Knight')

    fireEvent.click(within(dialog).getByRole('button', { name: 'Descartar tudo' }))

    await waitFor(() => expect(app.MetadataBatchDiscard).toHaveBeenCalledWith(7))
    await waitFor(() => expect(reviewDialog()).toBeNull())
    await waitFor(() => expect(screen.queryByText(/Revisar metadados/)).toBeNull())
  })

  it('MET-066/ao entrar em done a lista de jogos e o catálogo são invalidados', async () => {
    batch = batchOf({ state: 'applying', done: 1, total: 3 })
    renderApp()
    await waitConnected()
    await findChip('Aplicando metadados 1/3')
    await waitFor(() => expect(app.ListGames).toHaveBeenCalled())
    await waitFor(() => expect(app.Catalog).toHaveBeenCalled())
    const lists = app.ListGames.mock.calls.length
    const catalogs = app.Catalog.mock.calls.length
    const sessions = app.CurrentSession.mock.calls.length + app.GameSessions.mock.calls.length

    batch = batchOf({
      state: 'done',
      done: 3,
      total: 3,
      failed: 1,
      result: { applied: 2, imagesQueued: 4, imagesSkippedByLimit: 0, failedTitles: ['Alfa'] },
    })
    announce(batch)

    await findChip('Metadados aplicados em 2 jogos; 4 imagens na fila; 1 falharam')
    await waitFor(() => expect(app.ListGames.mock.calls.length).toBeGreaterThan(lists))
    await waitFor(() => expect(app.Catalog.mock.calls.length).toBeGreaterThan(catalogs))
    // Sessions are not touched by the batch [MET-066, MET-080].
    expect(app.CurrentSession.mock.calls.length + app.GameSessions.mock.calls.length).toBe(sessions)
  })

  it('MET-066/done: Fechar chama MetadataBatchDiscard e o resumo some', async () => {
    batch = batchOf({
      state: 'done',
      done: 3,
      total: 3,
      result: { applied: 3, imagesQueued: 0, imagesSkippedByLimit: 0, failedTitles: [] },
    })
    renderApp()
    await waitConnected()
    await findChip('Metadados aplicados em 3 jogos; 0 imagens na fila; 0 falharam')

    fireEvent.click(screen.getByRole('button', { name: 'Fechar' }))

    await waitFor(() => expect(app.MetadataBatchDiscard).toHaveBeenCalledWith(7))
    await waitFor(() => expect(screen.queryByText(/Metadados aplicados em/)).toBeNull())
  })

  it('MET-066/Cancelar em applying chama MetadataBatchCancel', async () => {
    batch = batchOf({ state: 'applying', done: 1, total: 3 })
    renderApp()
    await waitConnected()
    await findChip('Aplicando metadados 1/3')

    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))

    await waitFor(() => expect(app.MetadataBatchCancel).toHaveBeenCalledWith(7))
  })
})
