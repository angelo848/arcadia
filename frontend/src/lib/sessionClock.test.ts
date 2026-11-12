import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { anchorFrom, formatElapsed, useElapsedSeconds } from './sessionClock'
import type { SessionAnchor } from './sessionClock'
import type { SessionView } from './games'

function session(over: Partial<SessionView> = {}): SessionView {
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

type FakeApp = Record<string, ReturnType<typeof vi.fn>>

function installFakeBindings(): FakeApp {
  const app: FakeApp = {
    AddGame: vi.fn(() => Promise.resolve({})),
    AgentStatus: vi.fn(() => Promise.resolve({})),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    LaunchGame: vi.fn(() => Promise.resolve({})),
    ListGames: vi.fn(() => Promise.resolve([])),
    PickGameExe: vi.fn(() => Promise.resolve({ path: '', title: '' })),
  }
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: app } }
  w.runtime = { EventsOn: vi.fn(() => () => {}) }
  return app
}

function uninstallFakeBindings() {
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
}

function shown(seconds: number): string {
  return formatElapsed(seconds)
}

function advance(ms: number) {
  act(() => {
    vi.advanceTimersByTime(ms)
  })
}

describe('formatElapsed', () => {
  it('UI-013 0, 59, 252, 3600, 45005 e 450000', () => {
    const cases: Array<[number, string]> = [
      [0, '0:00:00'],
      [59, '0:00:59'],
      [252, '0:04:12'],
      [3600, '1:00:00'],
      [45005, '12:30:05'],
      [450000, '125:00:00'],
    ]
    for (const [input, want] of cases) {
      expect(formatElapsed(input), `formatElapsed(${input})`).toBe(want)
    }
  })
})

describe('anchorFrom', () => {
  it('UI-014 âncora usa activeMs, state e o instante de recebimento do relógio injetado', () => {
    const a = anchorFrom(session({ activeMs: 252400, state: 'running' }), () => 1234.5)
    expect(a).toEqual({ activeMs: 252400, state: 'running', receivedAt: 1234.5 })

    const g = anchorFrom(session({ activeMs: 9000, state: 'grace' }), () => 42)
    expect(g).toEqual({ activeMs: 9000, state: 'grace', receivedAt: 42 })
  })
})

describe('anchorFrom com paused', () => {
  it('UI-022 anchorFrom mapeia paused para state paused, com activeMs e receivedAt do relógio injetado', () => {
    const p = anchorFrom(session({ state: 'paused', activeMs: 100000 }), () => 777)
    expect(p).toEqual({ activeMs: 100000, state: 'paused', receivedAt: 777 })
  })
})

describe('useElapsedSeconds', () => {
  beforeEach(() => {
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date', 'performance'],
    })
    vi.setSystemTime(new Date('2026-11-09T10:04:12.400Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
    uninstallFakeBindings()
  })

  it('UI-014 A = 252400 mostra 0:04:12; +599 ms continua 0:04:12; +1 ms vira 0:04:13; +1000 ms vira 0:04:14', () => {
    const anchor = anchorFrom(session({ activeMs: 252400 }))
    const { result } = renderHook(() => useElapsedSeconds(anchor))

    expect(shown(result.current)).toBe('0:04:12')
    advance(599)
    expect(shown(result.current)).toBe('0:04:12')
    advance(1)
    expect(shown(result.current)).toBe('0:04:13')
    advance(1000)
    expect(shown(result.current)).toBe('0:04:14')
  })

  it('UI-014 mesmo exemplo com o relógio now injetado', () => {
    let t = 5000
    const now = () => t
    const anchor = anchorFrom(session({ activeMs: 252400 }), now)
    const { result } = renderHook(() => useElapsedSeconds(anchor, now))

    expect(result.current).toBe(252)
    t += 599
    advance(599)
    expect(result.current).toBe(252)
    t += 1
    advance(1)
    expect(result.current).toBe(253)
    t += 1000
    advance(1000)
    expect(result.current).toBe(254)
  })

  it('UI-014 âncora nova recomeça sem somar a anterior', () => {
    const first = anchorFrom(session({ sessionId: 1, activeMs: 10000 }))
    const { result, rerender } = renderHook(
      ({ anchor }: { anchor: SessionAnchor | null }) => useElapsedSeconds(anchor),
      { initialProps: { anchor: first } },
    )
    expect(result.current).toBe(10)
    advance(5000)
    expect(result.current).toBe(15)

    const second = anchorFrom(session({ sessionId: 2, activeMs: 3000 }))
    rerender({ anchor: second })
    expect(result.current).toBe(3)
    advance(999)
    expect(result.current).toBe(3)
    advance(1)
    expect(result.current).toBe(4)
  })

  it('UI-014 não usa startedAt nem asOf', () => {
    // startedAt far in the past and asOf in the future: neither may affect the value.
    const anchor = anchorFrom(
      session({
        activeMs: 252400,
        startedAt: '2000-01-01T00:00:00.000Z',
        asOf: '2099-01-01T00:00:00.000Z',
      }),
    )
    const { result, rerender } = renderHook(() => useElapsedSeconds(anchor))
    expect(shown(result.current)).toBe('0:04:12')

    // The wall clock (Date) jumps one hour ahead; the monotonic clock does not move.
    act(() => {
      vi.setSystemTime(new Date('2026-11-09T11:04:12.400Z'))
    })
    rerender()
    expect(shown(result.current)).toBe('0:04:12')

    // Wall clock jumps back; only monotonic time counts.
    act(() => {
      vi.setSystemTime(new Date('2026-11-09T09:00:00.000Z'))
    })
    advance(1000)
    expect(shown(result.current)).toBe('0:04:13')
  })

  it('UI-015 grace fica parado ao avançar o tempo', () => {
    const anchor = anchorFrom(session({ state: 'grace', activeMs: 252400 }))
    const { result } = renderHook(() => useElapsedSeconds(anchor))

    expect(shown(result.current)).toBe('0:04:12')
    // No timer is armed in grace (design: only in running).
    expect(vi.getTimerCount()).toBe(0)
    advance(15000)
    expect(shown(result.current)).toBe('0:04:12')
    advance(60000)
    expect(shown(result.current)).toBe('0:04:12')
  })

  it('UI-015 grace para running volta a avançar do activeMs novo', () => {
    const grace = anchorFrom(session({ state: 'grace', activeMs: 252400 }))
    const { result, rerender } = renderHook(
      ({ anchor }: { anchor: SessionAnchor | null }) => useElapsedSeconds(anchor),
      { initialProps: { anchor: grace } },
    )
    advance(8000)
    expect(result.current).toBe(252)

    const running = anchorFrom(session({ state: 'running', activeMs: 253100 }))
    rerender({ anchor: running })
    expect(result.current).toBe(253)
    advance(899)
    expect(result.current).toBe(253)
    advance(1)
    expect(result.current).toBe(254)
    advance(1000)
    expect(result.current).toBe(255)
  })

  it('UI-017 10 min avançando não chama nenhum binding', () => {
    const app = installFakeBindings()
    const anchor = anchorFrom(session({ activeMs: 252400 }))
    const { result } = renderHook(() => useElapsedSeconds(anchor))

    advance(10 * 60 * 1000)

    expect(result.current).toBe(Math.floor((252400 + 600000) / 1000))
    for (const [name, fn] of Object.entries(app)) {
      expect(fn, `binding ${name}`).not.toHaveBeenCalled()
    }
  })

  it('UI-017 desmontar limpa o temporizador', () => {
    const anchor = anchorFrom(session({ activeMs: 252400 }))
    const { unmount } = renderHook(() => useElapsedSeconds(anchor))
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    unmount()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('UI-017 sem âncora não arma temporizador', () => {
    renderHook(() => useElapsedSeconds(null))
    expect(vi.getTimerCount()).toBe(0)
  })

  it('UI-022/anchorFrom de paused congela: A = 100000 mostra 0:01:40 e depois de 10 min falsos continua 0:01:40', () => {
    const anchor = anchorFrom(session({ state: 'paused', activeMs: 100000 }))
    const { result } = renderHook(() => useElapsedSeconds(anchor))

    expect(shown(result.current)).toBe('0:01:40')
    advance(10 * 60 * 1000)
    expect(shown(result.current)).toBe('0:01:40')
  })

  it('UI-023/de paused para running o cronômetro volta a avançar de A: 0:01:40 → 0:01:41 após 1000 ms', () => {
    const paused = anchorFrom(session({ state: 'paused', activeMs: 100000 }))
    const { result, rerender } = renderHook(
      ({ anchor }: { anchor: SessionAnchor | null }) => useElapsedSeconds(anchor),
      { initialProps: { anchor: paused } },
    )
    advance(5 * 60 * 1000)
    expect(shown(result.current)).toBe('0:01:40')

    const running = anchorFrom(session({ state: 'running', activeMs: 100000 }))
    rerender({ anchor: running })
    expect(shown(result.current)).toBe('0:01:40')
    advance(999)
    expect(shown(result.current)).toBe('0:01:40')
    advance(1)
    expect(shown(result.current)).toBe('0:01:41')
  })

  it('UI-023/retomada depois de pausa no meio usa o activeMs da pausa e não soma o intervalo pausado', () => {
    const first = anchorFrom(session({ state: 'running', activeMs: 100000 }))
    const { result, rerender } = renderHook(
      ({ anchor }: { anchor: SessionAnchor | null }) => useElapsedSeconds(anchor),
      { initialProps: { anchor: first } },
    )
    advance(30000)
    expect(shown(result.current)).toBe('0:02:10')

    // Paused at active 130 s: the paused interval (here 10 min on the fake clock) must not count.
    rerender({ anchor: anchorFrom(session({ state: 'paused', activeMs: 130000 })) })
    advance(10 * 60 * 1000)
    expect(shown(result.current)).toBe('0:02:10')

    rerender({ anchor: anchorFrom(session({ state: 'running', activeMs: 130000 })) })
    expect(shown(result.current)).toBe('0:02:10')
    advance(1000)
    expect(shown(result.current)).toBe('0:02:11')
  })

  it('UI-023/de paused para grace congela no valor de A', () => {
    const paused = anchorFrom(session({ state: 'paused', activeMs: 100000 }))
    const { result, rerender } = renderHook(
      ({ anchor }: { anchor: SessionAnchor | null }) => useElapsedSeconds(anchor),
      { initialProps: { anchor: paused } },
    )
    rerender({ anchor: anchorFrom(session({ state: 'grace', activeMs: 100000 })) })
    expect(shown(result.current)).toBe('0:01:40')
    expect(vi.getTimerCount()).toBe(0)
    advance(60000)
    expect(shown(result.current)).toBe('0:01:40')
  })

  it('UI-025/em paused nenhum setTimeout fica armado (vi.getTimerCount() é 0)', () => {
    const anchor = anchorFrom(session({ state: 'paused', activeMs: 100000 }))
    const { result } = renderHook(() => useElapsedSeconds(anchor))
    expect(vi.getTimerCount()).toBe(0)

    advance(10 * 60 * 1000)
    expect(vi.getTimerCount()).toBe(0)
    expect(shown(result.current)).toBe('0:01:40')
  })

  it('UI-025/running para paused desarma o temporizador que estava armado', () => {
    const running = anchorFrom(session({ state: 'running', activeMs: 252400 }))
    const { rerender } = renderHook(
      ({ anchor }: { anchor: SessionAnchor | null }) => useElapsedSeconds(anchor),
      { initialProps: { anchor: running } },
    )
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    rerender({ anchor: anchorFrom(session({ state: 'paused', activeMs: 252400 })) })
    expect(vi.getTimerCount()).toBe(0)
  })
})
