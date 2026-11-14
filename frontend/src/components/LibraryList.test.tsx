import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { GRID_INSET } from '../lib/gridNav'
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
    keyboard: true,
    ...over,
  }
}

// rowOf returns the option (row) of the game with that title.
function rowOf(title: string): HTMLElement {
  return screen.getByRole('option', { name: new RegExp(`^${title}( |$)`) })
}

describe('LibraryList: margem interna', () => {
  const ROW = 64

  function installScrollTo() {
    const scrollTo = vi.fn(function (this: HTMLElement, opts: ScrollToOptions) {
      Object.defineProperty(this, 'scrollTop', { configurable: true, value: opts.top ?? 0 })
      queueMicrotask(() => fireEvent.scroll(this))
    })
    ;(HTMLElement.prototype as unknown as { scrollTo: unknown }).scrollTo = scrollTo
    return scrollTo
  }

  afterEach(() => {
    delete (HTMLElement.prototype as unknown as { scrollTo?: unknown }).scrollTo
  })

  it('AJU-002/lista: primeira linha em translateY(GRID_INSET), linhas com left GRID_INSET e largura descontando a margem', () => {
    render(<LibraryList {...props()} />)

    const rows = screen.getAllByRole('option')
    expect(rows[0].style.transform).toBe(`translateY(${GRID_INSET}px)`)
    expect(rows[1].style.transform).toBe(`translateY(${GRID_INSET + ROW}px)`)
    for (const row of rows) {
      expect(row.style.left).toBe(`${GRID_INSET}px`)
      expect(row.getAttribute('style') ?? '').toMatch(new RegExp(`calc\\(100% - ${2 * GRID_INSET}px\\)`))
    }
  })

  it('AJU-002/lista: altura total inclui GRID_INSET em cima e embaixo', () => {
    render(<LibraryList {...props()} />)
    const inner = screen.getByRole('listbox', { name: 'Jogos' }).firstElementChild as HTMLElement
    expect(parseFloat(inner.style.height)).toBe(3 * ROW + 2 * GRID_INSET)
  })

  it('AJU-002/lista: End e Home rolam com a margem', async () => {
    const scrollTo = installScrollTo()
    render(<LibraryList {...props({ games: makeGames(1000), total: 1000, selectedId: 0 })} />)
    scrollTo.mockClear()

    rowOf('Jogo 0').focus()
    fireEvent.keyDown(rowOf('Jogo 0'), { key: 'End' })
    expect(scrollTo).toHaveBeenCalled()
    const down = (scrollTo.mock.calls.at(-1)?.[0] as ScrollToOptions).top ?? NaN
    expect(down).toBeCloseTo(GRID_INSET + 999 * ROW + ROW + GRID_INSET - BOX_HEIGHT, 1)
    await waitFor(() => expect(rowOf('Jogo 999')).toHaveFocus())

    scrollTo.mockClear()
    fireEvent.keyDown(rowOf('Jogo 999'), { key: 'Home' })
    expect(scrollTo).toHaveBeenCalled()
    const up = (scrollTo.mock.calls.at(-1)?.[0] as ScrollToOptions).top ?? NaN
    expect(up).toBeLessThanOrEqual(GRID_INSET)
  })

  it('AJU-002/lista: ao montar com o jogo em foco, a centralização soma GRID_INSET', async () => {
    const scrollTo = installScrollTo()
    render(<LibraryList {...props({ games: makeGames(1000), total: 1000, selectedId: 600 })} />)

    await waitFor(() => expect(scrollTo).toHaveBeenCalled())
    const top = (scrollTo.mock.calls[0][0] as ScrollToOptions).top ?? NaN
    expect(top).toBeCloseTo(GRID_INSET + 600 * ROW - (BOX_HEIGHT - ROW) / 2, 1)
  })

  it('AJU-004/lista tem a classe [scrollbar-gutter:stable]', () => {
    render(<LibraryList {...props()} />)
    expect(screen.getByRole('listbox', { name: 'Jogos' })).toHaveClass('[scrollbar-gutter:stable]')
  })
})

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

  it('UIX-020/lista: uma parada de Tab (a linha do jogo em foco ou a primeira)', () => {
    const { rerender } = render(<LibraryList {...props({ selectedId: null })} />)
    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    const stops = () => [listbox, ...within(listbox).getAllByRole('option')].filter((e) => e.tabIndex >= 0)
    expect(stops()).toEqual([rowOf('Jogo 0')])

    rerender(<LibraryList {...props({ selectedId: 2 })} />)
    expect(stops()).toEqual([rowOf('Jogo 2')])
  })

  it.each([
    ['ArrowDown', 1, 2],
    ['ArrowUp', 1, 0],
  ])('UIX-020/lista: %s move o foco e o jogo em foco', (key, from, target) => {
    const p = props({ selectedId: from })
    render(<LibraryList {...p} />)

    rowOf(`Jogo ${from}`).focus()
    fireEvent.keyDown(rowOf(`Jogo ${from}`), { key })

    expect(p.onSelect).toHaveBeenCalledTimes(1)
    expect(p.onSelect).toHaveBeenCalledWith(target)
    expect(rowOf(`Jogo ${target}`)).toHaveFocus()
    expect(p.onLaunch).not.toHaveBeenCalled()
  })

  it.each([
    ['ArrowUp', 0],
    ['ArrowDown', 2],
  ])('UIX-020/lista: %s na borda não chama onSelect', (key, from) => {
    const p = props({ selectedId: from })
    render(<LibraryList {...p} />)

    rowOf(`Jogo ${from}`).focus()
    fireEvent.keyDown(rowOf(`Jogo ${from}`), { key })

    expect(p.onSelect).not.toHaveBeenCalled()
    expect(rowOf(`Jogo ${from}`)).toHaveFocus()
  })

  it('UIX-020/lista: Enter lança o jogo da linha focada', () => {
    const p = props({ selectedId: 1 })
    render(<LibraryList {...p} />)

    rowOf('Jogo 1').focus()
    fireEvent.keyDown(rowOf('Jogo 1'), { key: 'Enter' })

    expect(p.onLaunch).toHaveBeenCalledTimes(1)
    expect(p.onLaunch).toHaveBeenCalledWith(1)
  })

  it('UIX-046/Enter mantido (repeat) chama onLaunch uma vez', () => {
    const p = props({ selectedId: 1 })
    render(<LibraryList {...p} />)

    rowOf('Jogo 1').focus()
    fireEvent.keyDown(rowOf('Jogo 1'), { key: 'Enter' })
    fireEvent.keyDown(rowOf('Jogo 1'), { key: 'Enter', repeat: true })
    fireEvent.keyDown(rowOf('Jogo 1'), { key: 'Enter', repeat: true })

    expect(p.onLaunch).toHaveBeenCalledTimes(1)
    expect(p.onLaunch).toHaveBeenCalledWith(1)
  })

  it('UIX-020/lista: com keyboard=false as setas não fazem nada', () => {
    const p = props({ selectedId: 1, keyboard: false })
    render(<LibraryList {...p} />)

    rowOf('Jogo 1').focus()
    fireEvent.keyDown(rowOf('Jogo 1'), { key: 'ArrowDown' })
    fireEvent.keyDown(rowOf('Jogo 1'), { key: 'ArrowUp' })

    expect(p.onSelect).not.toHaveBeenCalled()
    expect(rowOf('Jogo 1')).toHaveFocus()
  })
})
