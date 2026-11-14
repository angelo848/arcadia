import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { LibraryList } from './LibraryList'
import { formatLocalDateTime } from '../lib/formatDate'
import { formatPlayTime, type SessionView } from '../lib/games'
import type { GameView } from '../lib/library'

type ListProps = Parameters<typeof LibraryList>[0]

// Every element reports this box, so the virtualizer computes its window from a
// fixed size [UIX-020]. Nothing here depends on real layout.
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
    selectedId: null,
    onNeedMore: vi.fn(),
    onSelect: vi.fn(),
    onLaunch: vi.fn(),
    ...over,
  }
}

// rowOf returns the option (row) of the game with that title.
function rowOf(title: string): HTMLElement {
  return screen.getByRole('option', { name: new RegExp(`^${title}( |$)`) })
}

describe('LibraryList', () => {
  it('UIX-020/lista: linha com título, plataforma, tempo, última vez e selo, sem botões; clique põe em foco e duplo clique lança', () => {
    const lastPlayedAt = '2026-11-09T10:00:00.000Z'
    const games = [
      game(0, { totalPlaySeconds: 0 }),
      game(1, { totalPlaySeconds: 3900, platform: 'Nintendo Switch', lastPlayedAt }),
      game(2, { totalPlaySeconds: 30 }),
    ]
    const p = props({ games, total: games.length, current: session({ gameId: 1, state: 'running' }) })
    render(<LibraryList {...p} />)

    expect(screen.getAllByRole('option').map((o) => within(o).getByText(/^Jogo \d$/).textContent)).toEqual([
      'Jogo 0',
      'Jogo 1',
      'Jogo 2',
    ])
    for (const g of games) {
      const row = rowOf(g.title)
      expect(within(row).getByText(g.platform)).toBeInTheDocument()
      expect(within(row).getByText(formatPlayTime(g.totalPlaySeconds))).toBeInTheDocument()
      expect(within(row).getByText(formatLocalDateTime(g.lastPlayedAt))).toBeInTheDocument()
    }
    expect(within(rowOf('Jogo 1')).getByText('Em execução')).toBeInTheDocument()
    expect(within(rowOf('Jogo 0')).queryByText(/Em execução|Pausado/)).toBeNull()
    expect(screen.queryAllByRole('button')).toHaveLength(0)

    fireEvent.click(rowOf('Jogo 2'))
    expect(p.onSelect).toHaveBeenCalledTimes(1)
    expect(p.onSelect).toHaveBeenCalledWith(2)
    expect(p.onLaunch).not.toHaveBeenCalled()

    fireEvent.doubleClick(rowOf('Jogo 1'))
    expect(p.onLaunch).toHaveBeenCalledTimes(1)
    expect(p.onLaunch).toHaveBeenCalledWith(1)
  })

  it('UIX-020/lista virtualizada: no máximo 100 linhas no DOM com 10 000 jogos', () => {
    render(<LibraryList {...props({ games: makeGames(10000), total: 10000 })} />)

    const rows = screen.getAllByRole('option')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThanOrEqual(100)
    expect(screen.getByText('Jogo 0')).toBeInTheDocument()
    expect(screen.queryByText('Jogo 9999')).toBeNull()
  })

  it('UIX-014/linha de jogo pausado mostra Pausado; jogo em execução ou grace mostra Em execução', () => {
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

  it('UIX-017/selectedId marca só a linha do jogo em foco (aria-selected)', () => {
    render(<LibraryList {...props({ selectedId: 1 })} />)

    expect(screen.getAllByRole('option', { selected: true })).toHaveLength(1)
    expect(rowOf('Jogo 1')).toHaveAttribute('aria-selected', 'true')
    expect(rowOf('Jogo 0')).toHaveAttribute('aria-selected', 'false')
  })

  it('UIX-042/lista com role listbox "Jogos" e linhas role option que começam pelo título', () => {
    render(<LibraryList {...props()} />)

    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    expect(within(listbox).getAllByRole('option')).toHaveLength(3)
    expect(within(listbox).getByRole('option', { name: /^Jogo 0\b/ })).toBeInTheDocument()
  })
})
