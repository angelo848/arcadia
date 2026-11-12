import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { LibraryList } from './LibraryList'
import { LibraryGrid } from './LibraryGrid'
import type { PendingLaunch } from '../lib/pendingLaunch'
import { formatPlayTime, type SessionView } from '../lib/games'
import type { GameView } from '../lib/library'

type ListProps = Parameters<typeof LibraryList>[0]

// Every element reports this box, so the virtualizer computes its window from a
// fixed size [UI-030]. Nothing here depends on real layout.
const BOX_WIDTH = 800
const BOX_HEIGHT = 600

class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
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

function props(over: Partial<ListProps> = {}): ListProps {
  return {
    games: makeGames(3),
    total: 3,
    current: null,
    pending: null,
    onNeedMore: vi.fn(),
    onOpen: vi.fn(),
    onPlay: vi.fn(),
    onPlayUntracked: vi.fn(),
    ...over,
  }
}

// titlesInDom lists the synthetic titles currently rendered.
function titlesInDom(): string[] {
  return screen.queryAllByText(/^Jogo \d+$/).map((e) => e.textContent ?? '')
}

// rowOf returns the smallest ancestor of the title that holds a button (any of the
// launch buttons, including "Iniciando…"): the row (or card) of that game, without
// assuming the markup.
function rowOf(title: string): HTMLElement {
  let el: HTMLElement | null = screen.getByText(title)
  while (el && within(el).queryAllByRole('button').length === 0) {
    el = el.parentElement
  }
  if (!el) {
    throw new Error(`no row for ${title}`)
  }
  return el
}

describe('LibraryList', () => {
  it('UI-030/lista mostra a mesma página, os mesmos dados e os mesmos botões da grade', () => {
    const games = [
      game(0, { totalPlaySeconds: 0 }),
      game(1, { totalPlaySeconds: 3900, platform: 'Nintendo Switch' }),
      game(2, { totalPlaySeconds: 30 }),
    ]
    const onPlay = vi.fn()
    const onPlayUntracked = vi.fn()
    const pending: PendingLaunch = { id: 2, mode: 'tracked' }
    const p = props({ games, total: games.length, onPlay, onPlayUntracked, pending })

    // Same page, same data and same buttons in the grid [UI-029, UI-031].
    const grid = render(<LibraryGrid {...p} />)
    const gridTitles = titlesInDom()
    for (const g of games) {
      const card = rowOf(g.title)
      expect(within(card).getByText(g.platform)).toBeInTheDocument()
      expect(within(card).getByText(formatPlayTime(g.totalPlaySeconds))).toBeInTheDocument()
      expect(within(card).getAllByRole('button').map((b) => b.textContent)).toEqual(
        g.id === 2 ? ['Iniciando…', 'Sem contar tempo'] : ['Jogar', 'Sem contar tempo'],
      )
    }
    grid.unmount()

    // The list shows the same games, in the same order, with the same data and buttons.
    render(<LibraryList {...p} />)
    expect(titlesInDom()).toEqual(gridTitles)
    expect(titlesInDom()).toEqual(['Jogo 0', 'Jogo 1', 'Jogo 2'])
    for (const g of games) {
      const row = rowOf(g.title)
      expect(within(row).getByText(g.platform)).toBeInTheDocument()
      expect(within(row).getByText(formatPlayTime(g.totalPlaySeconds))).toBeInTheDocument()
      expect(within(row).getAllByRole('button').map((b) => b.textContent)).toEqual(
        g.id === 2 ? ['Iniciando…', 'Sem contar tempo'] : ['Jogar', 'Sem contar tempo'],
      )
    }

    // The buttons of the row act on the row's game, as in the grid [UI-031].
    fireEvent.click(within(rowOf('Jogo 1')).getByRole('button', { name: 'Jogar' }))
    expect(onPlay).toHaveBeenCalledWith(1)
    fireEvent.click(within(rowOf('Jogo 0')).getByRole('button', { name: 'Sem contar tempo' }))
    expect(onPlayUntracked).toHaveBeenCalledWith(0)
    expect(onPlay).toHaveBeenCalledTimes(1)
    expect(onPlayUntracked).toHaveBeenCalledTimes(1)
  })

  it('UI-030/lista virtualizada: no máximo 100 linhas no DOM com 10 000 jogos', () => {
    // Extra case for the requirement "em linhas virtualizadas" of UI-030; not in the
    // tasks.md list of T29.
    render(<LibraryList {...props({ games: makeGames(10000), total: 10000 })} />)

    const rows = screen.getAllByRole('button', { name: 'Jogar' })
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThanOrEqual(100)
    expect(screen.getByText('Jogo 0')).toBeInTheDocument()
    expect(screen.queryByText('Jogo 9999')).toBeNull()
  })

  it('UI-024/linha de jogo pausado mostra \'Pausado\'; jogo em execução mostra \'Em execução\'', () => {
    const p = props({ games: makeGames(2), total: 2, current: session({ gameId: 0, state: 'paused' }) })
    const { rerender } = render(<LibraryList {...p} />)

    expect(within(rowOf('Jogo 0')).getByText('Pausado')).toBeInTheDocument()
    expect(within(rowOf('Jogo 0')).queryByText('Em execução')).toBeNull()
    expect(within(rowOf('Jogo 1')).queryByText('Pausado')).toBeNull()
    expect(within(rowOf('Jogo 1')).queryByText('Em execução')).toBeNull()

    for (const state of ['running', 'grace'] as const) {
      rerender(<LibraryList {...p} current={session({ gameId: 0, state })} />)
      expect(within(rowOf('Jogo 0')).getByText('Em execução')).toBeInTheDocument()
      expect(within(rowOf('Jogo 0')).queryByText('Pausado')).toBeNull()
      expect(within(rowOf('Jogo 1')).queryByText('Em execução')).toBeNull()
    }
  })
})
