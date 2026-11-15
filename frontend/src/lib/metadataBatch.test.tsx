import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { MetadataCandidateView } from './metadata'
import { libraryGamesKey, libraryKeys } from './library'
import {
  buildReviewRows,
  chooseCandidate,
  loadBatchRows,
  reviewCounts,
  toChoices,
  useMetadataBatch,
} from './metadataBatch'
import type { BatchRowView, BatchStatusView, ReviewRow } from './metadataBatch'

// Fakes of window.go.main.App (the batch bindings, contract delta §7) and window.runtime.EventsOn (event metadata:batch).
// Shapes assumed from contracts/ipc-v0.14.0-delta.md §7: MetadataBatchStatus -> { batch }, MetadataBatchStart -> { batchId, total },
// MetadataBatchResults -> { rows, total, next }, MetadataBatchApply -> accepted, MetadataBatchCancel -> state, MetadataBatchDiscard -> void.

type Fn = ReturnType<typeof vi.fn>

let app: Record<string, Fn>
let handlers: Map<string, Set<(...data: unknown[]) => void>>

function install() {
  handlers = new Map()
  app = {
    MetadataBatchStatus: vi.fn(() => Promise.resolve({ batch: null })),
    MetadataBatchStart: vi.fn(() => Promise.resolve({ batchId: 7, total: 3 })),
    MetadataBatchResults: vi.fn(() => Promise.resolve({ rows: [], total: 0, next: null })),
    MetadataBatchApply: vi.fn(() => Promise.resolve(2)),
    MetadataBatchCancel: vi.fn(() => Promise.resolve('cancelled')),
    MetadataBatchDiscard: vi.fn(() => Promise.resolve()),
    ListGames: vi.fn(() => Promise.resolve({ games: [], total: 0, offset: 0 })),
    GetGame: vi.fn(() => Promise.resolve({})),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    LaunchGame: vi.fn(() => Promise.resolve({})),
    PauseSession: vi.fn(() => Promise.resolve({})),
    ResumeSession: vi.fn(() => Promise.resolve({})),
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
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

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
}

function wrapperFor(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

async function settle() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 20))
  })
}

function otherBindingCalls(except: string[]) {
  return Object.entries(app)
    .filter(([name]) => !except.includes(name))
    .reduce((n, [, fn]) => n + fn.mock.calls.length, 0)
}

const cand = (title: string, score = 0.9): MetadataCandidateView => ({
  refs: [{ provider: 'igdb', providerId: '1' }],
  title,
  year: 2000,
  platform: 'PC',
  score,
})

const status = (over: Partial<BatchStatusView> = {}): BatchStatusView => ({
  batchId: 7,
  state: 'searching',
  total: 10,
  done: 0,
  failed: 0,
  warnings: [],
  ...over,
})

const row = (gameId: number, confidence: BatchRowView['confidence'], selected: number, candidates: MetadataCandidateView[]): BatchRowView => ({
  gameId,
  title: `Jogo ${gameId}`,
  platform: 'PC',
  confidence,
  selected,
  candidates,
})

// 15 rows: 9 match, 4 doubt (two alternatives each), 2 none (no candidate).
function fifteenRows(): BatchRowView[] {
  const rows: BatchRowView[] = []
  for (let i = 1; i <= 9; i++) rows.push(row(i, 'match', 0, [cand(`M${i}`)]))
  for (let i = 10; i <= 13; i++) rows.push(row(i, 'doubt', 0, [cand(`D${i}a`, 0.7), cand(`D${i}b`, 0.6)]))
  for (let i = 14; i <= 15; i++) rows.push(row(i, 'none', -1, []))
  return rows
}

beforeEach(() => {
  install()
})

afterEach(() => {
  uninstall()
})

describe('useMetadataBatch', () => {
  it('MET-055/lê o estado no início e na reconexão', async () => {
    app.MetadataBatchStatus.mockResolvedValueOnce({ batch: status({ state: 'searching', done: 2 }) })
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })

    await waitFor(() => expect(result.current.status?.done).toBe(2))
    expect(app.MetadataBatchStatus).toHaveBeenCalledTimes(1)
    // TODO(lacuna): o gatilho da reconexão (UI-009) não está definido na spec nem no hook; reativar quando houver mecanismo.
  })

  it('MET-055/evento com o mesmo state só atualiza contadores (sem nova leitura)', async () => {
    app.MetadataBatchStatus.mockResolvedValue({ batch: status({ state: 'searching', done: 0, total: 10 }) })
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.status?.state).toBe('searching'))

    emit('metadata:batch', { batchId: 7, state: 'searching', done: 5, total: 10, failed: 1 })

    await waitFor(() => expect(result.current.status?.done).toBe(5))
    expect(result.current.status?.failed).toBe(1)
    expect(result.current.status?.total).toBe(10)
    await settle()
    expect(app.MetadataBatchStatus).toHaveBeenCalledTimes(1)
  })

  it('MET-055/evento com state novo relê o estado', async () => {
    app.MetadataBatchStatus.mockResolvedValueOnce({ batch: status({ state: 'searching' }) })
    app.MetadataBatchStatus.mockResolvedValue({ batch: status({ state: 'ready', done: 10 }) })
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.status?.state).toBe('searching'))

    emit('metadata:batch', { batchId: 7, state: 'ready', done: 10, total: 10, failed: 0 })

    await waitFor(() => expect(result.current.status?.state).toBe('ready'))
    expect(app.MetadataBatchStatus).toHaveBeenCalledTimes(2)
  })

  it('MET-055/uma só assinatura de evento e cancelada ao desmontar', () => {
    const qc = newClient()
    const { rerender, unmount } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    expect(handlers.get('metadata:batch')?.size ?? 0).toBe(1)

    rerender()
    expect(handlers.get('metadata:batch')?.size ?? 0).toBe(1)

    unmount()
    expect(handlers.get('metadata:batch')?.size ?? 0).toBe(0)
  })

  it('MET-055/estado depois do descarte volta a null', async () => {
    app.MetadataBatchStatus.mockResolvedValueOnce({ batch: status({ state: 'ready' }) })
    app.MetadataBatchStatus.mockResolvedValue({ batch: null })
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.status?.state).toBe('ready'))

    await act(() => result.current.discard())

    await waitFor(() => expect(result.current.status).toBeNull())
    expect(app.MetadataBatchDiscard).toHaveBeenCalledWith(7)
  })

  it('MET-067/status null depois de ter tido estado marca interrupted', async () => {
    app.MetadataBatchStatus.mockResolvedValueOnce({ batch: status({ state: 'searching' }) })
    app.MetadataBatchStatus.mockResolvedValue({ batch: null })
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.status?.state).toBe('searching'))
    expect(result.current.interrupted).toBe(false)

    // O agente reiniciou: a leitura volta null.
    emit('metadata:batch', { batchId: 7, state: 'ready', done: 0, total: 10, failed: 0 })

    await waitFor(() => expect(result.current.interrupted).toBe(true))
    expect(result.current.status).toBeNull()

    // A leitura seguinte zera o aviso: ele já foi exibido (MET-067).
    emit('metadata:batch', { batchId: 7, state: 'applying', done: 0, total: 10, failed: 0 })
    await waitFor(() => expect(result.current.interrupted).toBe(false))
  })

  it('MET-067/o aviso de interrupção aparece uma vez e some na leitura seguinte (zera interrupted)', async () => {
    app.MetadataBatchStatus.mockResolvedValueOnce({ batch: status({ state: 'searching' }) })
    app.MetadataBatchStatus.mockResolvedValue({ batch: null })
    const qc = newClient()
    // Every render records the flag, so the test can count how many times the notice turned on.
    const seen: boolean[] = []
    const { result } = renderHook(
      () => {
        const h = useMetadataBatch()
        seen.push(h.interrupted)
        return h
      },
      { wrapper: wrapperFor(qc) },
    )
    await waitFor(() => expect(result.current.status?.state).toBe('searching'))

    // O agente reiniciou: a leitura volta null e o aviso liga uma vez.
    emit('metadata:batch', { batchId: 7, state: 'ready', done: 0, total: 10, failed: 0 })
    await waitFor(() => expect(result.current.interrupted).toBe(true))

    // A leitura seguinte (outro evento, mesmo sem batch) zera o aviso: ele já foi exibido.
    emit('metadata:batch', { batchId: 7, state: 'applying', done: 0, total: 10, failed: 0 })
    await waitFor(() => expect(result.current.interrupted).toBe(false))

    const turnedOn = seen.filter((v, i) => v && (i === 0 || !seen[i - 1])).length
    expect(turnedOn).toBe(1)
    expect(result.current.status).toBeNull()
  })

  it('MET-067/start com falha não muda o estado', async () => {
    app.MetadataBatchStart.mockRejectedValueOnce(new Error('BatchBusy'))
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(app.MetadataBatchStatus).toHaveBeenCalledTimes(1))

    await act(async () => {
      await result.current.start([1, 2, 3]).catch(() => undefined)
    })

    expect(app.MetadataBatchStart).toHaveBeenCalledWith([1, 2, 3])
    expect(result.current.status).toBeNull()
    expect(result.current.interrupted).toBe(false)
    // LACUNA: a spec pede "mostra o erro", mas a interface de useMetadataBatch não tem campo de erro; o caso não confere a exibição.
  })

  it('MET-066/ao ir a done invalida jogos e catálogo e nada de sessão', async () => {
    app.MetadataBatchStatus.mockResolvedValueOnce({ batch: status({ state: 'applying' }) })
    app.MetadataBatchStatus.mockResolvedValue({
      batch: status({ state: 'done', done: 3, total: 3, result: { applied: 2, imagesQueued: 4, imagesSkippedByLimit: 0, failedTitles: [] } }),
    })
    const qc = newClient()
    qc.setQueryData([...libraryGamesKey, 'page'], { pages: [], pageParams: [] })
    qc.setQueryData(libraryKeys.catalog, {})
    qc.setQueryData(['library', 'game', 10], {})
    qc.setQueryData(['library', 'sessions', 10], {})
    qc.setQueryData(['session', 'current'], null)
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.status?.state).toBe('applying'))

    emit('metadata:batch', { batchId: 7, state: 'done', done: 3, total: 3, failed: 0 })

    await waitFor(() => expect(result.current.status?.state).toBe('done'))
    expect(qc.getQueryState([...libraryGamesKey, 'page'])?.isInvalidated).toBe(true)
    expect(qc.getQueryState(libraryKeys.catalog)?.isInvalidated).toBe(true)
    expect(qc.getQueryState(['library', 'game', 10])?.isInvalidated).toBe(true)
    expect(qc.getQueryState(['library', 'sessions', 10])?.isInvalidated).toBe(false)
    expect(qc.getQueryState(['session', 'current'])?.isInvalidated).toBe(false)
    expect(otherBindingCalls(['MetadataBatchStatus', 'ListGames', 'GetGame'])).toBe(0)
    expect(app.CurrentSession).not.toHaveBeenCalled()
  })

  it('MET-066/cancel chama o binding com o batchId atual', async () => {
    app.MetadataBatchStatus.mockResolvedValue({ batch: status({ state: 'applying' }) })
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.status?.state).toBe('applying'))

    await act(() => result.current.cancel())

    expect(app.MetadataBatchCancel).toHaveBeenCalledWith(7)
  })

  it('MET-064/apply envia as escolhas do lote atual', async () => {
    app.MetadataBatchStatus.mockResolvedValue({ batch: status({ state: 'ready' }) })
    const qc = newClient()
    const { result } = renderHook(() => useMetadataBatch(), { wrapper: wrapperFor(qc) })
    await waitFor(() => expect(result.current.status?.state).toBe('ready'))

    const choices = [
      { gameId: 1, candidate: 0 },
      { gameId: 4, candidate: 2 },
    ]
    await act(() => result.current.apply(choices))

    expect(app.MetadataBatchApply).toHaveBeenCalledWith(7, choices)
  })
})

describe('loadBatchRows', () => {
  it('MET-057/junta páginas de 100 até next nulo, com progresso', async () => {
    const page = (start: number, n: number, next: number | null) => ({
      rows: Array.from({ length: n }, (_, i) => row(start + i, 'match', 0, [cand('x')])),
      total: 250,
      next,
    })
    const pages = [page(1, 100, 100), page(101, 100, 200), page(201, 50, null)]
    app.MetadataBatchResults.mockImplementation((_id: number, offset: number) => {
      const i = offset / 100
      return Promise.resolve(pages[i])
    })
    const progress = vi.fn()

    const rows = await loadBatchRows(7, progress)

    expect(rows).toHaveLength(250)
    expect(rows[0].gameId).toBe(1)
    expect(rows[249].gameId).toBe(250)
    expect(app.MetadataBatchResults.mock.calls.map((c) => [c[0], c[1], c[2]])).toEqual([
      [7, 0, 100],
      [7, 100, 100],
      [7, 200, 100],
    ])
    expect(progress).toHaveBeenLastCalledWith(250, 250)
    expect(progress.mock.calls.length).toBeGreaterThanOrEqual(3)
  })

  it('MET-057/página única', async () => {
    app.MetadataBatchResults.mockResolvedValueOnce({ rows: [row(1, 'match', 0, [cand('a')])], total: 1, next: null })

    const rows = await loadBatchRows(7)

    expect(rows).toHaveLength(1)
    expect(app.MetadataBatchResults).toHaveBeenCalledTimes(1)
  })
})

describe('buildReviewRows e marcação', () => {
  it('MET-061/buildReviewRows: 15 linhas (9 match, 4 doubt, 2 none) → 9 marcadas; doubt com 1º item e desmarcada; none sem escolha, desmarcada', () => {
    const rs = buildReviewRows(fifteenRows())

    expect(rs).toHaveLength(15)
    expect(rs.filter((r) => r.checked)).toHaveLength(9)
    expect(rs.filter((r) => r.checked && r.row.confidence !== 'match')).toHaveLength(0)
    for (const r of rs.slice(9, 13)) {
      expect(r.choice, `doubt ${r.row.gameId}`).toBe(0)
      expect(r.checked).toBe(false)
    }
    for (const r of rs.slice(13)) {
      expect(r.choice, `none ${r.row.gameId}`).toBe(-1)
      expect(r.checked).toBe(false)
    }
    expect(rs.every((r) => r.byUser === false)).toBe(true)
  })

  it('MET-061/chooseCandidate marca a linha; -1 desmarca e bloqueia', () => {
    const [doubt] = buildReviewRows([row(10, 'doubt', 0, [cand('a'), cand('b')])])

    const marked = chooseCandidate(doubt, 1)
    expect(marked.choice).toBe(1)
    expect(marked.checked).toBe(true)

    const blocked = chooseCandidate(marked, -1)
    expect(blocked.choice).toBe(-1)
    expect(blocked.checked).toBe(false)
  })

  it('MET-062/chooseCandidate guarda byUser', () => {
    const [r] = buildReviewRows([row(10, 'doubt', 0, [cand('a'), cand('b')])])
    expect(r.byUser).toBe(false)

    expect(chooseCandidate(r, 1).byUser).toBe(true)
  })

  it('MET-063/reviewCounts: selecionáveis, marcadas e sem correspondência', () => {
    const rs = buildReviewRows(fifteenRows())

    expect(reviewCounts(rs)).toEqual({ selectable: 13, checked: 9, none: 2 })

    const withUnmarked = rs.map((r) => (r.row.gameId === 1 ? chooseCandidate(r, -1) : r))
    expect(reviewCounts(withUnmarked)).toEqual({ selectable: 12, checked: 8, none: 2 })
  })

  it('MET-063/reviewCounts.none conta a confiança none, inclusive linhas que têm alternativas', () => {
    const rs = buildReviewRows([
      row(1, 'none', -1, []),
      row(2, 'none', -1, [cand('Alternativa', 0.5)]),
      row(3, 'match', 0, [cand('Achado')]),
    ])

    expect(reviewCounts(rs).none).toBe(2)
    expect(reviewCounts(rs)).toEqual({ selectable: 1, checked: 1, none: 2 })
  })

  it('MET-064/toChoices só as marcadas, na ordem', () => {
    const rs = buildReviewRows(fifteenRows())
    const withDoubt = rs.map((r) => (r.row.gameId === 12 ? chooseCandidate(r, 1) : r))

    const choices = toChoices(withDoubt as ReviewRow[])

    expect(choices).toHaveLength(10)
    expect(choices.map((c) => c.gameId)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 12])
    expect(choices.find((c) => c.gameId === 12)).toEqual({ gameId: 12, candidate: 1 })
    expect(choices.find((c) => c.gameId === 10)).toBeUndefined()
    expect(choices.find((c) => c.gameId === 14)).toBeUndefined()
  })
})
