import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { LibraryGrid } from './LibraryGrid'
import type { SessionView } from '../lib/games'
import type { GameView } from '../lib/library'
import { GRID_INSET, columnsFor, usableWidth } from '../lib/gridNav'

type GridProps = Parameters<typeof LibraryGrid>[0]

// Every element reports this box, so the virtualizer computes its window from a
// fixed size [UIX-016]. Nothing here depends on real layout. 800 px gives 4 columns.
const BOX_WIDTH = 800
const BOX_HEIGHT = 600

// Observer calls are recorded so the unmount test can see the cleanup [UIX-016].
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
  delete (HTMLElement.prototype as unknown as { scrollTo?: unknown }).scrollTo
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
    selectedId: null,
    onNeedMore: vi.fn(),
    onSelect: vi.fn(),
    onLaunch: vi.fn(),
    keyboard: true,
    ...over,
  }
}

// scrollGridTo moves the window of the grid to top px. The scroll element is the
// ancestor that the virtualizer listens to; every ancestor inside the container
// gets the scrollTop and a scroll event, so the test does not depend on the markup.
function scrollGridTo(container: HTMLElement, top: number): void {
  const anchor = within(container).getAllByRole('option')[0]
  for (let el: HTMLElement | null = anchor; el !== null && el !== container.parentElement; el = el.parentElement) {
    Object.defineProperty(el, 'scrollTop', { configurable: true, value: top })
    fireEvent.scroll(el)
  }
}

// titlesInDom lists the synthetic titles currently rendered.
function titlesInDom(): string[] {
  return screen.queryAllByText(/^Jogo \d+$/).map((e) => e.textContent ?? '')
}

// cardOf returns the option (card) of the game with that title.
function cardOf(title: string): HTMLElement {
  return screen.getByRole('option', { name: new RegExp(`^${title}( |$)`) })
}

describe('LibraryGrid', () => {
  it('UIX-016/10 000 jogos: no máximo 100 cartões no DOM com altura e largura fixas', () => {
    render(<LibraryGrid {...props({ games: makeGames(10000), total: 10000 })} />)

    const cards = screen.getAllByRole('option')
    expect(cards.length).toBeGreaterThan(0)
    expect(cards.length).toBeLessThanOrEqual(100)
    expect(screen.getByText('Jogo 0')).toBeInTheDocument()
    expect(screen.queryByText('Jogo 9999')).toBeNull()
  })

  it('UIX-016/rolar muda a janela sem chamar a rede (nenhum onNeedMore fora da pré-busca)', () => {
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

  it('UIX-016/pré-busca chama onNeedMore uma única vez ao passar de loaded - 40', () => {
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

  it('UIX-042/grade com role listbox "Jogos" e cartões role option com o título como nome', () => {
    render(<LibraryGrid {...props({ games: makeGames(3), total: 3 })} />)

    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    const options = within(listbox).getAllByRole('option')
    expect(options).toHaveLength(3)
    expect(within(listbox).getByRole('option', { name: 'Jogo 0' })).toBeInTheDocument()
    expect(within(listbox).getByRole('option', { name: 'Jogo 1' })).toBeInTheDocument()
    expect(within(listbox).getByRole('option', { name: 'Jogo 2' })).toBeInTheDocument()
  })

  // The card receives live from the grid, which maps running and grace to "Em execução"
  // and drops ended and discarded [UIX-014, UI-043].
  it('UIX-014/grade: sessão grace marca só o cartão do jogo dela como Em execução', () => {
    render(<LibraryGrid {...props({ current: session({ gameId: 0, state: 'grace' }) })} />)

    expect(screen.getAllByText('Em execução')).toHaveLength(1)
    expect(cardOf('Jogo 0')).toHaveTextContent('Em execução')
    expect(cardOf('Jogo 1')).not.toHaveTextContent('Em execução')
  })

  it('UIX-014/grade: sessão paused marca Pausado; ended ou discarded não marcam nenhum cartão', () => {
    const p = props({ current: session({ gameId: 0, state: 'paused' }) })
    const { rerender } = render(<LibraryGrid {...p} />)
    expect(cardOf('Jogo 0')).toHaveTextContent('Pausado')

    for (const state of ['ended', 'discarded'] as const) {
      rerender(<LibraryGrid {...p} current={session({ gameId: 0, state, endedAt: '2026-11-09T10:10:00.000Z' })} />)
      expect(screen.queryByText(/Em execução|Pausado/), state).toBeNull()
    }
  })

  it('UIX-014/grade não tem botões nos cartões', () => {
    render(<LibraryGrid {...props()} />)
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('UIX-017/selectedId marca só o cartão do jogo em foco (aria-selected)', () => {
    const { rerender } = render(<LibraryGrid {...props({ selectedId: 2 })} />)
    expect(screen.getAllByRole('option', { selected: true })).toHaveLength(1)
    expect(cardOf('Jogo 2')).toHaveAttribute('aria-selected', 'true')
    expect(cardOf('Jogo 1')).toHaveAttribute('aria-selected', 'false')

    rerender(<LibraryGrid {...props({ selectedId: 5 })} />)
    expect(screen.getAllByRole('option', { selected: true })).toHaveLength(1)
    expect(cardOf('Jogo 5')).toHaveAttribute('aria-selected', 'true')
    expect(cardOf('Jogo 2')).toHaveAttribute('aria-selected', 'false')
  })

  it('UIX-017/um clique chama onSelect(id) do cartão; duplo clique chama onLaunch(id)', () => {
    const p = props()
    render(<LibraryGrid {...p} />)

    fireEvent.click(cardOf('Jogo 3'))
    expect(p.onSelect).toHaveBeenCalledTimes(1)
    expect(p.onSelect).toHaveBeenCalledWith(3)
    expect(p.onLaunch).not.toHaveBeenCalled()

    fireEvent.doubleClick(cardOf('Jogo 4'))
    expect(p.onLaunch).toHaveBeenCalledTimes(1)
    expect(p.onLaunch).toHaveBeenCalledWith(4)
  })

  it('UIX-021/jogo em foco fora do carregado: nenhum cartão marcado e a grade começa do topo', () => {
    render(<LibraryGrid {...props({ selectedId: 999 })} />)

    expect(screen.queryAllByRole('option', { selected: true })).toHaveLength(0)
    expect(screen.getByText('Jogo 0')).toBeInTheDocument()
  })

  it('UIX-021/jogo em foco carregado: ao montar a grade rola até ele e o cartão fica marcado', async () => {
    // jsdom has no Element.scrollTo: this one moves scrollTop and dispatches scroll
    // afterwards (as a browser does, never inside the caller), so the window follows.
    const scrollTo = vi.fn(function (this: HTMLElement, opts: ScrollToOptions) {
      Object.defineProperty(this, 'scrollTop', { configurable: true, value: opts.top ?? 0 })
      queueMicrotask(() => fireEvent.scroll(this))
    })
    ;(HTMLElement.prototype as unknown as { scrollTo: unknown }).scrollTo = scrollTo

    render(<LibraryGrid {...props({ games: makeGames(1000), total: 1000, selectedId: 600 })} />)

    await waitFor(() => expect(screen.getByText('Jogo 600')).toBeInTheDocument())
    expect(scrollTo).toHaveBeenCalled()
    expect(cardOf('Jogo 600')).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByText('Jogo 0')).toBeNull()
  })

  it('UIX-016/desmontar a grade desfaz a observação da janela', () => {
    const { unmount } = render(<LibraryGrid {...props()} />)
    const before = observerUnobserve.mock.calls.length + observerDisconnect.mock.calls.length

    unmount()
    const after = observerUnobserve.mock.calls.length + observerDisconnect.mock.calls.length
    expect(after).toBeGreaterThan(before)
  })
})

// boxOf mocks the box of every element with the given width [AJU-003].
function mockBox(width: number): void {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () =>
      ({
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        bottom: BOX_HEIGHT,
        right: width,
        width,
        height: BOX_HEIGHT,
        toJSON: () => ({}),
      }) as DOMRect,
  )
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(width)
}

// installScrollTo mimics a browser: scrollTop moves and the scroll event follows later.
function installScrollToFn() {
  const scrollTo = vi.fn(function (this: HTMLElement, opts: ScrollToOptions) {
    Object.defineProperty(this, 'scrollTop', { configurable: true, value: opts.top ?? 0 })
    queueMicrotask(() => fireEvent.scroll(this))
  })
  ;(HTMLElement.prototype as unknown as { scrollTo: unknown }).scrollTo = scrollTo
  return scrollTo
}

// rowElOf returns the absolute row element that holds the cards of a first row.
function firstRowEl(): HTMLElement {
  const el = screen.getAllByRole('option')[0].parentElement
  if (el === null) {
    throw new Error('card without row')
  }
  return el
}

// Margin inside the scroll box [AJU-001..AJU-004]. Box 800 px: usable width = 800 - 12 - 8 = 780.
describe('LibraryGrid: imagens do cartão', () => {
  it('AJU-015/images por id chega ao cartão: capa com contain, substituta com cover, sem entrada placeholder', () => {
    render(
      <LibraryGrid
        {...props({
          games: makeGames(3),
          total: 3,
          images: { 0: { url: '/m/0.jpg', fit: 'contain' }, 1: { url: '/m/1.jpg', fit: 'cover' } },
        })}
      />,
    )
    const imgOf = (title: string) => cardOf(title).querySelector('img') as HTMLImageElement
    expect(imgOf('Jogo 0')).toHaveAttribute('src', '/m/0.jpg')
    expect(imgOf('Jogo 0')).toHaveAttribute('data-fit', 'contain')
    expect(imgOf('Jogo 1')).toHaveAttribute('src', '/m/1.jpg')
    expect(imgOf('Jogo 1')).toHaveAttribute('data-fit', 'cover')
    expect(imgOf('Jogo 2')).not.toHaveAttribute('data-fit')
  })
})

describe('LibraryGrid: margem interna', () => {
  const { rowHeight } = columnsFor(usableWidth(BOX_WIDTH))

  it('AJU-001/primeira linha em translateY(GRID_INSET), cada linha com left GRID_INSET e largura descontando 2 x GRID_INSET', () => {
    render(<LibraryGrid {...props({ games: makeGames(10), total: 10 })} />)

    const rows = new Set(screen.getAllByRole('option').map((c) => c.parentElement as HTMLElement))
    expect(rows.size).toBeGreaterThan(1)
    const sorted = [...rows]
    expect(sorted[0].style.transform).toBe(`translateY(${GRID_INSET}px)`)
    for (const row of sorted) {
      expect(row.style.left).toBe(`${GRID_INSET}px`)
      expect(row.getAttribute('style') ?? '').toMatch(new RegExp(`calc\\(100% - ${2 * GRID_INSET}px\\)`))
    }
  })

  it('AJU-001/altura total inclui GRID_INSET em cima e embaixo', () => {
    render(<LibraryGrid {...props({ games: makeGames(10), total: 10 })} />)

    const inner = screen.getByRole('listbox', { name: 'Jogos' }).firstElementChild as HTMLElement
    // 10 games in 4 columns = 3 rows.
    expect(parseFloat(inner.style.height)).toBeCloseTo(3 * rowHeight + 2 * GRID_INSET, 1)
  })

  it('AJU-001/seta para baixo até linha fora da janela rola com GRID_INSET de folga', async () => {
    mockBox(540) // usable 520 = 3 columns
    const scrollTo = installScrollToFn()
    const { rowHeight: rh } = columnsFor(usableWidth(540))
    render(<LibraryGrid {...props({ games: makeGames(1000), total: 1000, selectedId: 0 })} />)
    scrollTo.mockClear()

    cardOf('Jogo 0').focus()
    fireEvent.keyDown(cardOf('Jogo 0'), { key: 'End' })

    expect(scrollTo).toHaveBeenCalled()
    const top = (scrollTo.mock.calls.at(-1)?.[0] as ScrollToOptions).top ?? NaN
    const rowTop = GRID_INSET + Math.floor(999 / 3) * rh
    expect(top).toBeCloseTo(rowTop + rh + GRID_INSET - BOX_HEIGHT, 1)
    await waitFor(() => expect(cardOf('Jogo 999')).toHaveFocus())
  })

  it('AJU-001/ao montar com o jogo em foco, a centralização soma GRID_INSET', async () => {
    const scrollTo = installScrollToFn()
    render(<LibraryGrid {...props({ games: makeGames(1000), total: 1000, selectedId: 600 })} />)

    await waitFor(() => expect(scrollTo).toHaveBeenCalled())
    const top = (scrollTo.mock.calls[0][0] as ScrollToOptions).top ?? NaN
    const rowTop = GRID_INSET + Math.floor(600 / 4) * rowHeight
    expect(top).toBeCloseTo(rowTop - (BOX_HEIGHT - rowHeight) / 2, 1)
  })

  it.each([
    [484, 2, 2],
    [1132, 5, 6],
  ])('AJU-003/caixa de %i px dá entre %i e %i colunas (484 dá 2; 1132 dá pelo menos 5)', (width, min, max) => {
    mockBox(width)
    render(<LibraryGrid {...props({ games: makeGames(20), total: 20 })} />)

    const cols = firstRowEl().children.length
    expect(cols).toBeGreaterThanOrEqual(min)
    expect(cols).toBeLessThanOrEqual(max)
  })

  it('AJU-004/grade tem a classe [scrollbar-gutter:stable]', () => {
    render(<LibraryGrid {...props()} />)
    expect(screen.getByRole('listbox', { name: 'Jogos' })).toHaveClass('[scrollbar-gutter:stable]')
  })
})

// Keyboard [UIX-019]: a fixed grid of 3 columns (box 540 px, usable 520 = 3 cards of 160 px and 2 gaps of 16 px).
describe('LibraryGrid: teclado (3 colunas)', () => {
  beforeEach(() => {
    mockBox(540)
  })

  // installScrollTo mimics a browser: scrollTop moves and the scroll event follows later.
  function installScrollTo() {
    const scrollTo = vi.fn(function (this: HTMLElement, opts: ScrollToOptions) {
      Object.defineProperty(this, 'scrollTop', { configurable: true, value: opts.top ?? 0 })
      queueMicrotask(() => fireEvent.scroll(this))
    })
    ;(HTMLElement.prototype as unknown as { scrollTo: unknown }).scrollTo = scrollTo
    return scrollTo
  }

  function tabStops(listbox: HTMLElement): HTMLElement[] {
    return [listbox, ...within(listbox).queryAllByRole('option')].filter((e) => e.tabIndex >= 0)
  }

  it('UIX-019/a grade é uma parada de Tab: o foco vai ao cartão do jogo em foco ou ao primeiro', () => {
    const { rerender } = render(<LibraryGrid {...props({ selectedId: null })} />)
    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    expect(tabStops(listbox)).toEqual([cardOf('Jogo 0')])

    rerender(<LibraryGrid {...props({ selectedId: 4 })} />)
    expect(tabStops(listbox)).toEqual([cardOf('Jogo 4')])

    // Game in focus not loaded: the first card.
    rerender(<LibraryGrid {...props({ selectedId: 999 })} />)
    expect(tabStops(listbox)).toEqual([cardOf('Jogo 0')])
  })

  it('UIX-019/jogo em foco carregado mas fora da janela: só o contêiner é parada de Tab', () => {
    render(<LibraryGrid {...props({ games: makeGames(1000), total: 1000, selectedId: 600 })} />)
    const listbox = screen.getByRole('listbox', { name: 'Jogos' })
    expect(screen.queryByText('Jogo 600')).toBeNull()
    expect(tabStops(listbox)).toEqual([listbox])
  })

  it.each([
    ['ArrowRight', 5],
    ['ArrowLeft', 3],
    ['ArrowDown', 7],
    ['ArrowUp', 1],
    ['Home', 0],
    ['End', 9],
  ])('UIX-019/setas, Home e End movem o foco e o jogo em foco (onSelect): %s', (key, target) => {
    const p = props({ selectedId: 4 })
    render(<LibraryGrid {...p} />)

    cardOf('Jogo 4').focus()
    fireEvent.keyDown(cardOf('Jogo 4'), { key })

    expect(p.onSelect).toHaveBeenCalledTimes(1)
    expect(p.onSelect).toHaveBeenCalledWith(target)
    expect(cardOf(`Jogo ${target}`)).toHaveFocus()
    expect(p.onLaunch).not.toHaveBeenCalled()
  })

  it.each([
    ['ArrowLeft', 0],
    ['ArrowUp', 1],
    ['ArrowRight', 2],
    ['ArrowDown', 9],
  ])('UIX-019/seta na borda não chama onSelect: %s no jogo %i', (key, from) => {
    const p = props({ selectedId: from })
    render(<LibraryGrid {...p} />)

    cardOf(`Jogo ${from}`).focus()
    fireEvent.keyDown(cardOf(`Jogo ${from}`), { key })

    expect(p.onSelect).not.toHaveBeenCalled()
    expect(cardOf(`Jogo ${from}`)).toHaveFocus()
  })

  it('UIX-019/o cartão focado rola para a área visível', async () => {
    const scrollTo = installScrollTo()
    const p = props({ games: makeGames(1000), total: 1000, selectedId: 0 })
    render(<LibraryGrid {...p} />)
    scrollTo.mockClear()

    cardOf('Jogo 0').focus()
    fireEvent.keyDown(cardOf('Jogo 0'), { key: 'End' })

    expect(p.onSelect).toHaveBeenCalledWith(999)
    await waitFor(() => expect(screen.getByText('Jogo 999')).toBeInTheDocument())
    expect(scrollTo).toHaveBeenCalled()
    await waitFor(() => expect(cardOf('Jogo 999')).toHaveFocus())
  })

  it('UIX-019/perto do fim do carregado, onNeedMore uma vez', async () => {
    installScrollTo()
    const onNeedMore = vi.fn()
    render(<LibraryGrid {...props({ games: makeGames(50), total: 1000, selectedId: 0, onNeedMore })} />)
    expect(onNeedMore).not.toHaveBeenCalled()

    cardOf('Jogo 0').focus()
    fireEvent.keyDown(cardOf('Jogo 0'), { key: 'End' })

    await waitFor(() => expect(onNeedMore).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(cardOf('Jogo 49')).toHaveFocus())
    fireEvent.keyDown(cardOf('Jogo 49'), { key: 'ArrowUp' })
    expect(onNeedMore).toHaveBeenCalledTimes(1)
  })

  it('UIX-019/Enter chama onLaunch do cartão focado', () => {
    const p = props({ selectedId: 4 })
    render(<LibraryGrid {...p} />)

    cardOf('Jogo 4').focus()
    fireEvent.keyDown(cardOf('Jogo 4'), { key: 'Enter' })

    expect(p.onLaunch).toHaveBeenCalledTimes(1)
    expect(p.onLaunch).toHaveBeenCalledWith(4)
  })

  it('UIX-046/Enter mantido (repeat) chama onLaunch uma vez', () => {
    const p = props({ selectedId: 4 })
    render(<LibraryGrid {...p} />)

    cardOf('Jogo 4').focus()
    fireEvent.keyDown(cardOf('Jogo 4'), { key: 'Enter' })
    fireEvent.keyDown(cardOf('Jogo 4'), { key: 'Enter', repeat: true })
    fireEvent.keyDown(cardOf('Jogo 4'), { key: 'Enter', repeat: true })

    expect(p.onLaunch).toHaveBeenCalledTimes(1)
    expect(p.onLaunch).toHaveBeenCalledWith(4)
  })

  it('UIX-019/com keyboard=false as setas não fazem nada na grade', () => {
    const p = props({ selectedId: 4, keyboard: false })
    render(<LibraryGrid {...p} />)

    cardOf('Jogo 4').focus()
    for (const key of ['ArrowRight', 'ArrowLeft', 'ArrowDown', 'ArrowUp', 'Home', 'End']) {
      fireEvent.keyDown(cardOf('Jogo 4'), { key })
    }

    expect(p.onSelect).not.toHaveBeenCalled()
    expect(cardOf('Jogo 4')).toHaveFocus()
  })
})
