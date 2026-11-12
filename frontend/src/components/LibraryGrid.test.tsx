import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { LibraryGrid } from './LibraryGrid'
import type { PendingLaunch } from '../lib/pendingLaunch'
import type { SessionView } from '../lib/games'
import type { GameView } from '../lib/library'

type GridProps = Parameters<typeof LibraryGrid>[0]

// Every element reports this box, so the virtualizer computes its window from a
// fixed size [UI-029]. Nothing here depends on real layout.
const BOX_WIDTH = 800
const BOX_HEIGHT = 600

// Observer calls are recorded so the unmount test can see the cleanup [UI-029].
const observerUnobserve = vi.fn()
const observerDisconnect = vi.fn()

class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {
    observerUnobserve()
  }
  disconnect(): void {
    observerDisconnect()
  }
}

beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
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
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(BOX_HEIGHT)
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(BOX_WIDTH)
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
  observerUnobserve.mockClear()
  observerDisconnect.mockClear()
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function game(id: number, over: Partial<GameView> = {}): GameView {
  return {
    id,
    platformId: 1,
    title: `Jogo ${id}`,
    platform: 'Windows',
    path: `C:\\Games\\jogo${id}.exe`,
    status: 'not_played',
    favorite: false,
    year: null,
    totalPlaySeconds: 0,
    playCount: 0,
    firstPlayedAt: '',
    lastPlayedAt: '',
    ...over,
  }
}

function makeGames(n: number): GameView[] {
  return Array.from({ length: n }, (_, i) => game(i))
}

function session(over: Partial<SessionView> = {}): SessionView {
  return {
    sessionId: 1,
    gameId: 0,
    title: 'Jogo 0',
    state: 'running',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '',
    activeSeconds: 0,
    activeMs: 0,
    asOf: '2026-11-09T10:00:00.000Z',
    ...over,
  }
}

function props(over: Partial<GridProps> = {}): GridProps {
  return {
    games: makeGames(10),
    total: 10,
    current: null,
    pending: null,
    onNeedMore: vi.fn(),
    onOpen: vi.fn(),
    onPlay: vi.fn(),
    onPlayUntracked: vi.fn(),
    ...over,
  }
}

// scrollGridTo moves the window of the grid to top px. The scroll element is the
// ancestor that the virtualizer listens to; every ancestor inside the container
// gets the scrollTop and a scroll event, so the test does not depend on the markup.
function scrollGridTo(container: HTMLElement, top: number): void {
  const anchor = within(container).getAllByRole('button', { name: 'Jogar' })[0]
  for (let el: HTMLElement | null = anchor; el !== null && el !== container.parentElement; el = el.parentElement) {
    Object.defineProperty(el, 'scrollTop', { configurable: true, value: top })
    fireEvent.scroll(el)
  }
}

// titlesInDom lists the synthetic titles currently rendered.
function titlesInDom(): string[] {
  return screen.queryAllByText(/^Jogo \d+$/).map((e) => e.textContent ?? '')
}

// cardOf returns the smallest ancestor of the title that holds a "Jogar" button.
function cardOf(title: string): HTMLElement {
  let el: HTMLElement | null = screen.getByText(title)
  while (el && within(el).queryAllByRole('button', { name: 'Jogar' }).length === 0) {
    el = el.parentElement
  }
  if (!el) {
    throw new Error(`no card for ${title}`)
  }
  return el
}

describe('LibraryGrid', () => {
  it('UI-029/10 000 jogos: no máximo 100 cards no DOM com altura e largura fixas', () => {
    render(<LibraryGrid {...props({ games: makeGames(10000), total: 10000 })} />)

    const cards = screen.getAllByRole('button', { name: 'Jogar' })
    expect(cards.length).toBeGreaterThan(0)
    expect(cards.length).toBeLessThanOrEqual(100)
    expect(screen.getByText('Jogo 0')).toBeInTheDocument()
    expect(screen.queryByText('Jogo 9999')).toBeNull()
  })

  it('UI-029/rolar muda a janela sem chamar a rede (nenhum onNeedMore fora da pré-busca)', () => {
    const onNeedMore = vi.fn()
    const { container } = render(
      <LibraryGrid {...props({ games: makeGames(1000), total: 10000, onNeedMore })} />,
    )
    const before = titlesInDom()
    expect(before).toContain('Jogo 0')

    scrollGridTo(container, 5000)
    const after = titlesInDom()
    expect(after).not.toEqual(before)
    expect(after).not.toContain('Jogo 0')

    scrollGridTo(container, 0)
    expect(titlesInDom()).toContain('Jogo 0')

    // 1000 loaded: the prefetch threshold is far below these positions [UI-036].
    expect(onNeedMore).not.toHaveBeenCalled()
  })

  it('UI-036/pré-busca chama onNeedMore uma única vez ao passar de loaded - 40', () => {
    const onNeedMore = vi.fn()
    const { container } = render(
      <LibraryGrid {...props({ games: makeGames(50), total: 10000, onNeedMore })} />,
    )

    // 50 loaded: the threshold is index 10. Scroll in small steps until it fires.
    let top = 0
    for (; top <= 20000 && onNeedMore.mock.calls.length === 0; top += 100) {
      scrollGridTo(container, top)
    }
    expect(onNeedMore).toHaveBeenCalledTimes(1)

    // Further scrolling with the same loaded pages does not ask again.
    scrollGridTo(container, top)
    scrollGridTo(container, top + 100)
    expect(onNeedMore).toHaveBeenCalledTimes(1)
  })

  // Extra cases for the marking of the grid itself: the card receives live from
  // the grid, which maps running and grace to "Em execução" and drops ended and discarded [UI-043].
  it('UI-006/grade: sessão grace marca só o card do jogo dela como Em execução', () => {
    render(<LibraryGrid {...props({ current: session({ gameId: 0, state: 'grace' }) })} />)

    expect(screen.getAllByText('Em execução')).toHaveLength(1)
    expect(cardOf('Jogo 0')).toHaveTextContent('Em execução')
    expect(cardOf('Jogo 1')).not.toHaveTextContent('Em execução')
  })

  it('UI-043/grade: sessão ended ou discarded não marca nenhum card', () => {
    const p = props({ current: session({ gameId: 0, state: 'paused' }) })
    const { rerender } = render(<LibraryGrid {...p} />)
    expect(cardOf('Jogo 0')).toHaveTextContent('Pausado')

    for (const state of ['ended', 'discarded'] as const) {
      rerender(<LibraryGrid {...p} current={session({ gameId: 0, state, endedAt: '2026-11-09T10:10:00.000Z' })} />)
      expect(screen.queryByText(/Em execução|Pausado/), state).toBeNull()
    }
  })

  it('UI-021/pendência de um jogo na grade desabilita só os botões dele', () => {
    const pending: PendingLaunch = { id: 0, mode: 'untracked' }
    render(<LibraryGrid {...props({ pending })} />)

    const triggered = within(cardOf('Jogo 0')).getByRole('button', { name: 'Iniciando…' })
    expect(triggered).toBeDisabled()
    expect(within(cardOf('Jogo 0')).getByRole('button', { name: 'Jogar' })).toBeDisabled()
    expect(within(cardOf('Jogo 1')).getByRole('button', { name: 'Jogar' })).toBeEnabled()
    expect(within(cardOf('Jogo 1')).getByRole('button', { name: 'Sem contar tempo' })).toBeEnabled()
    expect(screen.getAllByRole('button', { name: 'Iniciando…' })).toHaveLength(1)
  })

  it('UI-029/desmontar a grade desfaz a observação da janela', () => {
    const { unmount } = render(<LibraryGrid {...props()} />)
    const before = observerUnobserve.mock.calls.length + observerDisconnect.mock.calls.length

    unmount()
    const after = observerUnobserve.mock.calls.length + observerDisconnect.mock.calls.length
    expect(after).toBeGreaterThan(before)
  })
})
