import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  TTB_FETCH_DELAY_MS,
  formatTimeToBeat,
  timeToBeatKey,
  timeToBeatRows,
  timeToBeatSource,
  useTimeToBeat,
  useTimeToBeatEvents,
} from './timeToBeat'
import type { TimeToBeatView } from './timeToBeat'

// Fakes of window.go.main.App (TimeToBeatGet(gameId, fetch), contract v0.16.0 delta §6, camelCase views)
// and window.runtime.EventsOn (event timeToBeat:changed, payload { gameId }).

type Fn = ReturnType<typeof vi.fn>
let app: Record<string, Fn>
let handlers: Map<string, Set<(...data: unknown[]) => void>>

function view(over: Partial<TimeToBeatView> = {}): TimeToBeatView {
  return {
    gameId: 1,
    state: 'found',
    source: 'hltb',
    externalId: 'x',
    externalTitle: 'X',
    mainSeconds: 3600,
    mainExtraSeconds: 5400,
    completionistSeconds: 97200,
    fetchedAt: '2026-11-22T10:00:00Z',
    confirmed: false,
    stale: false,
    pending: false,
    unavailable: false,
    candidates: [],
    ...over,
  } as TimeToBeatView
}

function install(withApp = true) {
  handlers = new Map()
  app = {
    TimeToBeatGet: vi.fn((id: number) => Promise.resolve(view({ gameId: id }))),
  }
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = withApp ? { main: { App: app } } : { main: { App: {} } }
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

function emit(name: string, ...data: unknown[]) {
  act(() => {
    for (const cb of handlers.get(name) ?? []) {
      cb(...data)
    }
  })
}

function wrapperFor(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
}

async function flush(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

function callsWith(fetch: boolean, id?: number) {
  return app.TimeToBeatGet.mock.calls.filter((c) => c[1] === fetch && (id === undefined || c[0] === id))
}

beforeEach(() => {
  vi.useFakeTimers()
  install()
})

afterEach(() => {
  vi.useRealTimers()
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
})

describe('formatTimeToBeat', () => {
  const cases: [number | null, string][] = [
    [null, '—'],
    [0, '—'],
    [-5, '—'],
    [100, '5 min'],
    [1800, '30 min'],
    [3300, '55 min'],
    [3599, '1 h'],
    [5400, '1½ h'],
    [97200, '27 h'],
    [99000, '27½ h'],
    [99900, '28 h'],
  ]
  for (const [s, want] of cases) {
    it(`TTB-014/formatTimeToBeat(${String(s)}) = ${want}`, () => {
      expect(formatTimeToBeat(s)).toBe(want)
    })
  }
})

describe('timeToBeatRows', () => {
  it('TTB-014/sempre 3 linhas com os rótulos; valores formatados', () => {
    const rows = timeToBeatRows(view())
    expect(rows.map((r) => r.label)).toEqual(['História principal', 'História + extras', '100%'])
    expect(rows.map((r) => r.value)).toEqual(['1 h', '1½ h', '27 h'])
  })

  it('TTB-014/TTB-013/sem dado (undefined ou nulos) -> 3 linhas com "—"', () => {
    for (const v of [undefined, view({ state: 'none', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })]) {
      const rows = timeToBeatRows(v)
      expect(rows.map((r) => r.label)).toEqual(['História principal', 'História + extras', '100%'])
      expect(rows.map((r) => r.value)).toEqual(['—', '—', '—'])
    }
  })
})

describe('timeToBeatSource', () => {
  it('TTB-039/hltb, igdb e sem valor', () => {
    expect(timeToBeatSource(view({ source: 'hltb' }))).toBe('HowLongToBeat')
    expect(timeToBeatSource(view({ source: 'igdb' }))).toBe('IGDB')
    expect(timeToBeatSource(view({ source: '' }))).toBeNull()
    expect(timeToBeatSource(undefined)).toBeNull()
  })
})

describe('useTimeToBeat', () => {
  it('TTB-015/constante e chave', () => {
    expect(TTB_FETCH_DELAY_MS).toBe(1000)
    expect(timeToBeatKey(7)).toEqual(['timeToBeat', 7])
  })

  it('TTB-013/erro do binding vira data undefined, sem lançar', async () => {
    app.TimeToBeatGet.mockImplementation(() => Promise.reject(new Error('boom')))
    const { result } = renderHook(() => useTimeToBeat(1, { fetchAfterMs: null }), { wrapper: wrapperFor(newClient()) })
    await flush()
    expect(result.current.data).toBeUndefined()
  })

  it('TTB-013/binding ausente vira data undefined', async () => {
    install(false)
    const { result } = renderHook(() => useTimeToBeat(1, { fetchAfterMs: null }), { wrapper: wrapperFor(newClient()) })
    await flush()
    expect(result.current.data).toBeUndefined()
  })

  it('TTB-013/agente desconectado (rejeição) não lança', async () => {
    app.TimeToBeatGet.mockImplementation(() => Promise.reject('agent disconnected'))
    const { result } = renderHook(() => useTimeToBeat(1, { fetchAfterMs: 0 }), { wrapper: wrapperFor(newClient()) })
    await flush(10)
    expect(result.current.data).toBeUndefined()
  })

  it('TTB-015/lê o cache na hora e pede fetch true uma vez após 1.000 ms', async () => {
    const { result } = renderHook(() => useTimeToBeat(1, { fetchAfterMs: TTB_FETCH_DELAY_MS }), {
      wrapper: wrapperFor(newClient()),
    })
    await flush()
    expect(callsWith(false, 1)).toHaveLength(1)
    expect(callsWith(true)).toHaveLength(0)
    expect(result.current.data?.gameId).toBe(1)
    await flush(999)
    expect(callsWith(true)).toHaveLength(0)
    await flush(1)
    expect(callsWith(true, 1)).toHaveLength(1)
    await flush(5000)
    expect(callsWith(true)).toHaveLength(1)
  })

  it('TTB-015/trocar de jogo antes de 1.000 ms cancela o pedido do anterior', async () => {
    const { rerender } = renderHook(({ id }) => useTimeToBeat(id, { fetchAfterMs: TTB_FETCH_DELAY_MS }), {
      initialProps: { id: 1 },
      wrapper: wrapperFor(newClient()),
    })
    await flush(500)
    rerender({ id: 2 })
    await flush(600)
    expect(callsWith(true, 1)).toHaveLength(0)
    expect(callsWith(true, 2)).toHaveLength(0)
    await flush(400)
    expect(callsWith(true, 2)).toHaveLength(1)
    expect(callsWith(true, 1)).toHaveLength(0)
  })

  it('TTB-015/desmontar limpa o temporizador', async () => {
    const { unmount } = renderHook(() => useTimeToBeat(1, { fetchAfterMs: TTB_FETCH_DELAY_MS }), {
      wrapper: wrapperFor(newClient()),
    })
    await flush(500)
    unmount()
    await flush(2000)
    expect(callsWith(true)).toHaveLength(0)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('TTB-016/fetchAfterMs 0 pede fetch true na hora, uma vez', async () => {
    renderHook(() => useTimeToBeat(1, { fetchAfterMs: 0 }), { wrapper: wrapperFor(newClient()) })
    await flush(0)
    expect(callsWith(true, 1)).toHaveLength(1)
    await flush(5000)
    expect(callsWith(true)).toHaveLength(1)
  })

  it('TTB-016/fetchAfterMs null só lê o cache', async () => {
    renderHook(() => useTimeToBeat(1, { fetchAfterMs: null }), { wrapper: wrapperFor(newClient()) })
    await flush(5000)
    expect(callsWith(false, 1).length).toBeGreaterThanOrEqual(1)
    expect(callsWith(true)).toHaveLength(0)
  })

  it('TTB-015/gameId null não chama o binding', async () => {
    renderHook(() => useTimeToBeat(null, { fetchAfterMs: 0 }), { wrapper: wrapperFor(newClient()) })
    await flush(2000)
    expect(app.TimeToBeatGet).not.toHaveBeenCalled()
  })
})

describe('useTimeToBeatEvents', () => {
  it('TTB-017/timeToBeat:changed invalida só a consulta do jogo do aviso', async () => {
    const qc = newClient()
    const w = wrapperFor(qc)
    renderHook(
      () => {
        useTimeToBeatEvents()
        return [useTimeToBeat(1, { fetchAfterMs: null }), useTimeToBeat(2, { fetchAfterMs: null })]
      },
      { wrapper: w },
    )
    await flush()
    expect(callsWith(false, 1)).toHaveLength(1)
    expect(callsWith(false, 2)).toHaveLength(1)

    emit('timeToBeat:changed', { gameId: 2 })
    await flush()
    expect(callsWith(false, 2)).toHaveLength(2)
    expect(callsWith(false, 1)).toHaveLength(1)
    expect(callsWith(true)).toHaveLength(0)

    emit('timeToBeat:changed', { gameId: 99 })
    await flush()
    expect(callsWith(false, 1)).toHaveLength(1)
    expect(callsWith(false, 2)).toHaveLength(2)
  })

  it('TTB-017/desmontar cancela a assinatura do evento', async () => {
    const { unmount } = renderHook(() => useTimeToBeatEvents(), { wrapper: wrapperFor(newClient()) })
    expect(handlers.get('timeToBeat:changed')?.size).toBe(1)
    unmount()
    expect(handlers.get('timeToBeat:changed')?.size ?? 0).toBe(0)
  })
})
