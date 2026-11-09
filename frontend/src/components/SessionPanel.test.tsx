import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { SessionPanel } from './SessionPanel'
import type { SessionView } from '../lib/games'

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

function panel(): HTMLElement {
  return screen.getByRole('status', { name: 'Sessão em andamento' })
}

function advance(ms: number) {
  act(() => {
    vi.advanceTimersByTime(ms)
  })
}

describe('SessionPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date', 'performance'],
    })
    vi.setSystemTime(new Date('2026-11-09T10:04:12.400Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('UI-012 role status com nome Sessão em andamento, título e cronômetro', () => {
    render(<SessionPanel session={session({ title: 'Celeste', activeMs: 45005000 })} now={() => 0} />)
    const p = panel()
    expect(p).toHaveTextContent('Celeste')
    expect(p).toHaveTextContent('12:30:05')
    expect(p).not.toHaveTextContent('Encerrando…')
  })

  it('UI-014 avança a cada segundo', () => {
    // Default clock (performance.now, faked).
    render(<SessionPanel session={session({ activeMs: 252400 })} />)
    expect(panel()).toHaveTextContent('0:04:12')
    advance(599)
    expect(panel()).toHaveTextContent('0:04:12')
    advance(1)
    expect(panel()).toHaveTextContent('0:04:13')
    advance(1000)
    expect(panel()).toHaveTextContent('0:04:14')
  })

  it('UI-014 avança a cada segundo com now injetado e recomeça de uma sessão nova', () => {
    let t = 1000
    const now = () => t
    const step = (ms: number) => {
      t += ms
      advance(ms)
    }
    const { rerender } = render(<SessionPanel session={session({ activeMs: 252400 })} now={now} />)
    expect(panel()).toHaveTextContent('0:04:12')
    step(599)
    expect(panel()).toHaveTextContent('0:04:12')
    step(1)
    expect(panel()).toHaveTextContent('0:04:13')
    step(1000)
    expect(panel()).toHaveTextContent('0:04:14')

    // A new session value restarts from its activeMs and reception instant.
    rerender(<SessionPanel session={session({ sessionId: 2, activeMs: 3000 })} now={now} />)
    expect(panel()).toHaveTextContent('0:00:03')
    step(999)
    expect(panel()).toHaveTextContent('0:00:03')
    step(1)
    expect(panel()).toHaveTextContent('0:00:04')
  })

  it('UI-015 grace mostra Encerrando… e congela', () => {
    const { rerender } = render(<SessionPanel session={session({ state: 'grace', activeMs: 252400 })} />)
    expect(panel()).toHaveTextContent('Encerrando…')
    expect(panel()).toHaveTextContent('0:04:12')
    advance(15000)
    expect(panel()).toHaveTextContent('0:04:12')
    advance(60000)
    expect(panel()).toHaveTextContent('0:04:12')

    // Back to running: "Encerrando…" goes away and the clock advances from the new activeMs.
    rerender(<SessionPanel session={session({ state: 'running', activeMs: 253100 })} />)
    expect(panel()).not.toHaveTextContent('Encerrando…')
    expect(panel()).toHaveTextContent('0:04:13')
    advance(899)
    expect(panel()).toHaveTextContent('0:04:13')
    advance(1)
    expect(panel()).toHaveTextContent('0:04:14')
  })
})
