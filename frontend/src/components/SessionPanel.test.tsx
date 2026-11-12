import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
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

const noop = () => {}
const props = { onToggle: noop, pending: false }

describe('SessionPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date', 'performance'],
    })
    vi.setSystemTime(new Date('2026-11-09T10:04:12.400Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
    delete (window as unknown as { go?: unknown }).go
  })

  it('UI-012 role status com nome Sessão em andamento, título e cronômetro', () => {
    render(<SessionPanel session={session({ title: 'Celeste', activeMs: 45005000 })} {...props} now={() => 0} />)
    const p = panel()
    expect(p).toHaveTextContent('Celeste')
    expect(p).toHaveTextContent('12:30:05')
    expect(p).not.toHaveTextContent('Encerrando…')
  })

  it('UI-014 avança a cada segundo', () => {
    // Default clock (performance.now, faked).
    render(<SessionPanel session={session({ activeMs: 252400 })} {...props} />)
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
    const { rerender } = render(<SessionPanel session={session({ activeMs: 252400 })} {...props} now={now} />)
    expect(panel()).toHaveTextContent('0:04:12')
    step(599)
    expect(panel()).toHaveTextContent('0:04:12')
    step(1)
    expect(panel()).toHaveTextContent('0:04:13')
    step(1000)
    expect(panel()).toHaveTextContent('0:04:14')

    // A new session value restarts from its activeMs and reception instant.
    rerender(<SessionPanel session={session({ sessionId: 2, activeMs: 3000 })} {...props} now={now} />)
    expect(panel()).toHaveTextContent('0:00:03')
    step(999)
    expect(panel()).toHaveTextContent('0:00:03')
    step(1)
    expect(panel()).toHaveTextContent('0:00:04')
  })

  it('UI-015 grace mostra Encerrando… e congela', () => {
    const { rerender } = render(
      <SessionPanel session={session({ state: 'grace', activeMs: 252400 })} {...props} />,
    )
    expect(panel()).toHaveTextContent('Encerrando…')
    expect(panel()).toHaveTextContent('0:04:12')
    advance(15000)
    expect(panel()).toHaveTextContent('0:04:12')
    advance(60000)
    expect(panel()).toHaveTextContent('0:04:12')

    // Back to running: "Encerrando…" goes away and the clock advances from the new activeMs.
    rerender(<SessionPanel session={session({ state: 'running', activeMs: 253100 })} {...props} />)
    expect(panel()).not.toHaveTextContent('Encerrando…')
    expect(panel()).toHaveTextContent('0:04:13')
    advance(899)
    expect(panel()).toHaveTextContent('0:04:13')
    advance(1)
    expect(panel()).toHaveTextContent('0:04:14')
  })

  it('UI-022/paused mostra título, 0:01:40 parado após 10 min falsos, Pausado e não mostra Encerrando…', () => {
    let t = 5000
    const now = () => t
    render(
      <SessionPanel
        session={session({ title: 'Celeste', state: 'paused', activeMs: 100000 })}
        {...props}
        now={now}
      />,
    )
    const p = panel()
    expect(p).toHaveTextContent('Celeste')
    expect(p).toHaveTextContent('0:01:40')
    expect(within(p).getByText('Pausado')).toBeInTheDocument()
    expect(p).not.toHaveTextContent('Encerrando…')

    // 10 min pass on both the injected clock and the faked timers.
    t += 600_000
    advance(600_000)
    expect(p).toHaveTextContent('0:01:40')
    expect(p).not.toHaveTextContent('0:11:40')
  })

  it('UI-023/de paused para running Pausado some e o cronômetro volta a andar; para grace mostra Encerrando… congelado', () => {
    let t = 0
    const now = () => t
    const step = (ms: number) => {
      t += ms
      advance(ms)
    }
    const { rerender } = render(
      <SessionPanel session={session({ state: 'paused', activeMs: 100000 })} {...props} now={now} />,
    )
    expect(within(panel()).getByText('Pausado')).toBeInTheDocument()

    rerender(<SessionPanel session={session({ state: 'running', activeMs: 100000 })} {...props} now={now} />)
    expect(screen.queryByText('Pausado')).toBeNull()
    expect(panel()).toHaveTextContent('0:01:40')
    step(999)
    expect(panel()).toHaveTextContent('0:01:40')
    step(1)
    expect(panel()).toHaveTextContent('0:01:41')

    rerender(<SessionPanel session={session({ state: 'grace', activeMs: 101000 })} {...props} now={now} />)
    expect(panel()).toHaveTextContent('Encerrando…')
    expect(screen.queryByText('Pausado')).toBeNull()
    step(60_000)
    expect(panel()).toHaveTextContent('0:01:41')
  })

  it('UI-025/em paused nenhum binding é chamado e nenhum temporizador fica armado', () => {
    const app = {
      PauseSession: vi.fn(),
      ResumeSession: vi.fn(),
      LaunchGameUntracked: vi.fn(),
      LaunchGame: vi.fn(),
      ListGames: vi.fn(),
      CurrentSession: vi.fn(),
    }
    ;(window as unknown as { go?: unknown }).go = { main: { App: app } }

    // Sanity check: a running panel does arm a timer, so the zero below means something.
    const { rerender } = render(<SessionPanel session={session({ state: 'running', activeMs: 100000 })} {...props} />)
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    rerender(<SessionPanel session={session({ state: 'paused', activeMs: 100000 })} {...props} />)
    expect(vi.getTimerCount()).toBe(0)
    advance(600_000)
    expect(vi.getTimerCount()).toBe(0)
    expect(panel()).toHaveTextContent('0:01:40')
    for (const fn of Object.values(app)) {
      expect(fn).not.toHaveBeenCalled()
    }
  })

  it('UI-027/rótulo Pausar em running, Retomar em paused, Pausar desabilitado em grace', () => {
    const onToggle = vi.fn()
    const cases: Array<[SessionView['state'], string, boolean]> = [
      ['running', 'Pausar', true],
      ['paused', 'Retomar', true],
      ['grace', 'Pausar', false],
    ]
    for (const [state, label, enabled] of cases) {
      const { unmount } = render(
        <SessionPanel session={session({ state })} onToggle={onToggle} pending={false} />,
      )
      const btn = within(panel()).getByRole('button', { name: label })
      if (enabled) {
        expect(btn, `${state}: ${label}`).toBeEnabled()
      } else {
        expect(btn, `${state}: ${label}`).toBeDisabled()
        fireEvent.click(btn)
        expect(onToggle, `${state}: click on disabled`).not.toHaveBeenCalled()
      }
      unmount()
    }
  })

  it('UI-027/botão desabilitado enquanto pending e rótulo muda quando o estado novo chega', () => {
    const onToggle = vi.fn()
    const { rerender } = render(
      <SessionPanel session={session({ state: 'running' })} onToggle={onToggle} pending />,
    )
    const pausing = within(panel()).getByRole('button', { name: 'Pausar' })
    expect(pausing).toBeDisabled()
    fireEvent.click(pausing)
    expect(onToggle).not.toHaveBeenCalled()

    rerender(<SessionPanel session={session({ state: 'running' })} onToggle={onToggle} pending={false} />)
    expect(within(panel()).getByRole('button', { name: 'Pausar' })).toBeEnabled()

    // The new state arrives while the call is still pending: label changes, button stays disabled.
    rerender(<SessionPanel session={session({ state: 'paused' })} onToggle={onToggle} pending />)
    expect(within(panel()).getByRole('button', { name: 'Retomar' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Pausar' })).toBeNull()

    // The call ends: the button comes back enabled with the label of the new state.
    rerender(<SessionPanel session={session({ state: 'paused' })} onToggle={onToggle} pending={false} />)
    expect(within(panel()).getByRole('button', { name: 'Retomar' })).toBeEnabled()
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('UI-028/clicar chama onToggle uma vez e não altera estado, cronômetro nem rótulo por conta própria', () => {
    const onToggle = vi.fn()
    let t = 0
    const now = () => t
    render(
      <SessionPanel session={session({ state: 'running', activeMs: 100000 })} onToggle={onToggle} pending={false} now={now} />,
    )
    const btn = within(panel()).getByRole('button', { name: 'Pausar' })
    const timers = vi.getTimerCount()

    fireEvent.click(btn)
    expect(onToggle).toHaveBeenCalledTimes(1)

    // Nothing changes on the panel until a new session value arrives.
    expect(panel()).toHaveTextContent('0:01:40')
    expect(within(panel()).getByRole('button', { name: 'Pausar' })).toBeEnabled()
    expect(screen.queryByText('Pausado')).toBeNull()
    // The button does not arm or change any timer.
    expect(vi.getTimerCount()).toBe(timers)
  })
})
