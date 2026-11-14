import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MediaCarousel } from '../components/MediaCarousel'
import type { MediaItemView } from './metadata'
import { bigBoxItems, cardGrid, enterBigBox, exitBigBox, gridNext, nextFocus, pickNext, useBigBoxNav, type Box } from './bigbox'

// A 2 x 2 grid of boxes: 0 top-left, 1 top-right, 2 bottom-left, 3 bottom-right.
const boxes: Box[] = [
  { x: 0, y: 0, w: 100, h: 100 },
  { x: 120, y: 0, w: 100, h: 100 },
  { x: 0, y: 120, w: 100, h: 100 },
  { x: 120, y: 120, w: 100, h: 100 },
]

describe('pickNext (itens fora da grade)', () => {
  it('UIX-044/move o foco na direção pedida e fica quando não há item', () => {
    expect(pickNext(boxes, 0, 'right')).toBe(1)
    expect(pickNext(boxes, 0, 'down')).toBe(2)
    expect(pickNext(boxes, 3, 'left')).toBe(2)
    expect(pickNext(boxes, 0, 'left')).toBe(0)
    expect(pickNext(boxes, 0, 'up')).toBe(0)
  })
})

// jsdom has no layout: every element with data-x / data-y reports that position (100 x 30).
function placeByData() {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const left = Number(this.dataset.x ?? 0)
    const top = Number(this.dataset.y ?? 0)
    return { left, top, right: left + 100, bottom: top + 30, width: 100, height: 30, x: left, y: top, toJSON: () => ({}) } as DOMRect
  })
}

function el(x: number, y: number, attrs: Record<string, string> = {}): HTMLElement {
  const e = document.createElement('div')
  e.dataset.x = String(x)
  e.dataset.y = String(y)
  e.tabIndex = 0
  e.setAttribute('data-bigbox-item', '')
  for (const [k, v] of Object.entries(attrs)) {
    e.setAttribute(k, v)
  }
  document.body.appendChild(e)
  return e
}

function card(x: number, y: number, id = 1): HTMLElement {
  return el(x, y, { 'data-bigbox-card': '', 'data-game-id': String(id) })
}

describe('cardGrid e gridNext (um item por cartão)', () => {
  beforeEach(() => {
    placeByData()
  })
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('UIX-044/cardGrid agrupa por [data-bigbox-card]: linhas pelo topo, colunas pela esquerda, um item por cartão', () => {
    const left = el(0, 0)
    const c3 = card(0, 100, 3)
    const c2 = card(200, 0, 2)
    const c1 = card(100, 0, 1)
    const grid = cardGrid([left, c3, c2, c1])
    expect(grid).not.toBeNull()
    expect(grid?.map((row) => row.map((c) => c.items))).toEqual([[[c1], [c2]], [[c3]]])
  })

  it('UIX-044/cardGrid devolve null quando a página não tem cartões', () => {
    expect(cardGrid([el(0, 0), el(0, 50)])).toBeNull()
    expect(cardGrid([])).toBeNull()
  })

  it('UIX-044/gridNext move cartão a cartão e cruza a linha pela mesma coluna', () => {
    const a = card(100, 0, 1)
    const b = card(200, 0, 2)
    const c = card(100, 100, 3)
    const grid = [[{ items: [a] }, { items: [b] }], [{ items: [c] }]]
    expect(gridNext(grid, a, 'right')).toBe(b)
    expect(gridNext(grid, b, 'left')).toBe(a)
    expect(gridNext(grid, a, 'down')).toBe(c)
    expect(gridNext(grid, c, 'up')).toBe(a)
    expect(gridNext(grid, b, 'down')).toBe(c) // the column clamps to the shorter row
  })

  it('UIX-044/gridNext devolve null nas quatro bordas da grade', () => {
    const a = card(100, 0, 1)
    const b = card(200, 0, 2)
    const c = card(100, 100, 3)
    const grid = [[{ items: [a] }, { items: [b] }], [{ items: [c] }]]
    expect(gridNext(grid, a, 'left')).toBeNull()
    expect(gridNext(grid, a, 'up')).toBeNull()
    expect(gridNext(grid, b, 'right')).toBeNull()
    expect(gridNext(grid, b, 'up')).toBeNull()
    expect(gridNext(grid, c, 'down')).toBeNull()
    expect(gridNext(grid, c, 'left')).toBeNull()
    expect(gridNext(grid, c, 'right')).toBeNull()
  })

  it('UIX-044/nextFocus: direita na borda vai ao painel, esquerda na borda vai à coluna esquerda', () => {
    const search = el(0, 10)
    const platform = el(0, 60)
    const c1 = card(300, 0, 1)
    const c2 = card(420, 0, 2)
    const play = el(900, 10, { 'data-bigbox-play': '' })
    const edit = el(900, 60)
    const items = [search, platform, c1, c2, play, edit]
    expect(nextFocus(items, c1, 'right')).toBe(c2)
    expect(nextFocus(items, c2, 'right')).toBe(play)
    expect(nextFocus(items, c1, 'left')).toBe(search)
  })

  it('UIX-044/nextFocus fora de um cartão usa a geometria de todos os itens', () => {
    const search = el(0, 10)
    const platform = el(0, 60)
    const c1 = card(300, 0, 1)
    const play = el(900, 10)
    const edit = el(900, 60)
    const items = [search, platform, c1, play, edit]
    expect(nextFocus(items, play, 'down')).toBe(edit)
    expect(nextFocus(items, play, 'left')).toBe(c1)
    expect(nextFocus(items, search, 'down')).toBe(platform)
  })
})

// Board: a left item, three cards (two in the first row, one below) and two panel items.
function Board({ onBack, onPlay, pageKey = 'library' }: { onBack: () => void; onPlay: (id: number) => void; pageKey?: 'library' | 'stats' }) {
  useBigBoxNav({ enabled: true, pageKey, onBack, onPlay })
  const click = (name: string) => () => {
    ;(window as unknown as Record<string, string[]>).clicks.push(name)
  }
  return (
    <div>
      <button type="button" data-bigbox-item data-x="0" data-y="0" onClick={click('left')}>
        Esquerda
      </button>
      {[1, 2, 3].map((n) => (
        <div
          key={n}
          role="option"
          aria-selected={false}
          tabIndex={0}
          data-bigbox-item
          data-bigbox-card
          data-game-id={n}
          data-x={300 + ((n - 1) % 2) * 120}
          data-y={Math.floor((n - 1) / 2) * 120}
          onClick={click(`card${n}`)}
        >
          Jogo {n}
        </div>
      ))}
      <button type="button" data-bigbox-item data-bigbox-play data-x="900" data-y="0" onClick={click('play')}>
        Jogar
      </button>
      <button type="button" data-bigbox-item data-x="900" data-y="60" onClick={click('edit')}>
        Editar
      </button>
    </div>
  )
}

const w = window as unknown as { clicks: string[] }

describe('useBigBoxNav (UIX-044)', () => {
  beforeEach(() => {
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [] })
    placeByData()
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0)
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    w.clicks = []
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  const cardEl = (n: number) => bigBoxItems().find((e) => e.dataset.gameId === String(n)) as HTMLElement
  const key = (k: string, init: KeyboardEventInit = {}) => fireEvent.keyDown(window, { key: k, ...init })

  it('UIX-044/o foco começa no primeiro item da página', () => {
    render(<Board onBack={vi.fn()} onPlay={vi.fn()} />)
    expect(document.activeElement).toBe(bigBoxItems()[0])
  })

  it('UIX-044/cada cartão é um item: direita e baixo andam cartão a cartão', () => {
    render(<Board onBack={vi.fn()} onPlay={vi.fn()} />)
    cardEl(1).focus()
    key('ArrowRight')
    expect(document.activeElement).toBe(cardEl(2))
    key('ArrowLeft')
    expect(document.activeElement).toBe(cardEl(1))
    key('ArrowDown')
    expect(document.activeElement).toBe(cardEl(3))
    key('ArrowUp')
    expect(document.activeElement).toBe(cardEl(1))
  })

  it('UIX-044/na borda direita da grade o foco vai ao painel e na esquerda à coluna esquerda', () => {
    render(<Board onBack={vi.fn()} onPlay={vi.fn()} />)
    cardEl(2).focus()
    key('ArrowRight')
    expect(document.activeElement?.textContent).toBe('Jogar')
    cardEl(1).focus()
    key('ArrowLeft')
    expect(document.activeElement?.textContent).toBe('Esquerda')
  })

  it('UIX-044/Enter num cartão faz click() nele', () => {
    render(<Board onBack={vi.fn()} onPlay={vi.fn()} />)
    cardEl(2).focus()
    key('Enter')
    expect(w.clicks).toEqual(['card2'])
  })

  it('UIX-044/Enter em item que não é cartão fica com a ativação nativa (o hook não clica)', () => {
    render(<Board onBack={vi.fn()} onPlay={vi.fn()} />)
    ;(bigBoxItems().find((e) => e.textContent === 'Editar') as HTMLElement).focus()
    key('Enter')
    expect(w.clicks).toEqual([])
  })

  it('UIX-044/Shift+Enter num cartão chama onPlay com o id do cartão, uma vez', () => {
    const onPlay = vi.fn()
    render(<Board onBack={vi.fn()} onPlay={onPlay} />)
    cardEl(3).focus()
    key('Enter', { shiftKey: true })
    expect(onPlay).toHaveBeenCalledTimes(1)
    expect(onPlay).toHaveBeenCalledWith(3)
    expect(w.clicks).toEqual([])
  })

  it('UIX-044/Shift+Enter fora de um cartão não chama onPlay', () => {
    const onPlay = vi.fn()
    render(<Board onBack={vi.fn()} onPlay={onPlay} />)
    ;(bigBoxItems().find((e) => e.textContent === 'Editar') as HTMLElement).focus()
    key('Enter', { shiftKey: true })
    expect(onPlay).not.toHaveBeenCalled()
  })

  it('UIX-044/Esc chama o voltar da página', () => {
    const onBack = vi.fn()
    render(<Board onBack={onBack} onPlay={vi.fn()} />)
    key('Escape')
    expect(onBack).toHaveBeenCalledTimes(1)
  })

  it('UIX-044/controle: direcional move, A faz click no cartão, Y chama onPlay, B volta', () => {
    const onBack = vi.fn()
    const onPlay = vi.fn()
    const pressed = (index: number) => {
      const buttons = Array.from({ length: 16 }, (_, i) => ({ pressed: i === index }))
      return { buttons, axes: [0, 0] } as unknown as Gamepad
    }
    let pad: Gamepad = pressed(-1)
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [pad] })
    let frame: FrameRequestCallback = () => undefined
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      frame = cb
      return 1
    })
    render(<Board onBack={onBack} onPlay={onPlay} />)
    cardEl(1).focus()

    const step = (index: number) => {
      pad = pressed(index)
      act(() => frame(0))
      pad = pressed(-1)
      act(() => frame(0))
    }
    step(15) // right: card 2
    expect(document.activeElement).toBe(cardEl(2))
    step(0) // A
    expect(w.clicks).toEqual(['card2'])
    step(3) // Y
    expect(onPlay).toHaveBeenCalledTimes(1)
    expect(onPlay).toHaveBeenCalledWith(2)
    step(15) // right edge: the panel
    expect(document.activeElement?.textContent).toBe('Jogar')
    step(1) // B
    expect(onBack).toHaveBeenCalledTimes(1)
  })
})

describe('useBigBoxNav com modal, eventos tratados e tecla mantida', () => {
  beforeEach(() => {
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [] })
    placeByData()
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0)
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    w.clicks = []
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  const cardEl = (n: number) => bigBoxItems().find((e) => e.dataset.gameId === String(n)) as HTMLElement
  const key = (k: string, init: KeyboardEventInit = {}) => fireEvent.keyDown(window, { key: k, ...init })

  function WithModal({ open, onBack, onPlay }: { open: boolean; onBack: () => void; onPlay: (id: number) => void }) {
    return (
      <div>
        <Board onBack={onBack} onPlay={onPlay} />
        {open && (
          <div role="dialog" aria-modal="true" aria-label="Editar jogo">
            <button type="button">Salvar</button>
          </div>
        )}
      </div>
    )
  }

  it('UIX-037/com um modal aberto, setas, Enter, Shift+Enter, Y e B do teclado não agem no fundo e o foco fica no modal', () => {
    const onBack = vi.fn()
    const onPlay = vi.fn()
    const { rerender } = render(<WithModal open={false} onBack={onBack} onPlay={onPlay} />)
    cardEl(1).focus()
    rerender(<WithModal open onBack={onBack} onPlay={onPlay} />)
    const save = document.querySelector('[role="dialog"] button') as HTMLElement
    save.focus()
    for (const k of ['ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp', 'Enter', 'Escape']) {
      key(k)
    }
    key('Enter', { shiftKey: true })
    key('y')
    key('Y')
    expect(onBack).not.toHaveBeenCalled()
    expect(onPlay).not.toHaveBeenCalled()
    expect(w.clicks).toEqual([])
    expect(document.activeElement).toBe(save)
  })

  it('UIX-037/com um modal aberto, direções, A, Y e B do controle (gamepad simulado) não agem no fundo', () => {
    const onBack = vi.fn()
    const onPlay = vi.fn()
    const pressed = (index: number) => ({ buttons: Array.from({ length: 16 }, (_, i) => ({ pressed: i === index })), axes: [0, 0] }) as unknown as Gamepad
    let pad: Gamepad = pressed(-1)
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [pad] })
    let frame: FrameRequestCallback = () => undefined
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      frame = cb
      return 1
    })
    const { rerender } = render(<WithModal open={false} onBack={onBack} onPlay={onPlay} />)
    cardEl(1).focus()
    rerender(<WithModal open onBack={onBack} onPlay={onPlay} />)
    const save = document.querySelector('[role="dialog"] button') as HTMLElement
    save.focus()
    for (const b of [15, 13, 14, 12, 0, 3, 1]) {
      pad = pressed(b)
      act(() => frame(0))
      pad = pressed(-1)
      act(() => frame(0))
    }
    expect(onBack).not.toHaveBeenCalled()
    expect(onPlay).not.toHaveBeenCalled()
    expect(w.clicks).toEqual([])
    expect(document.activeElement).toBe(save)
  })

  it('UIX-037/ao fechar o modal, a navegação do Big Box volta a valer', () => {
    const onBack = vi.fn()
    const { rerender } = render(<WithModal open onBack={onBack} onPlay={vi.fn()} />)
    key('Escape')
    expect(onBack).not.toHaveBeenCalled()
    rerender(<WithModal open={false} onBack={onBack} onPlay={vi.fn()} />)
    cardEl(1).focus()
    key('ArrowRight')
    expect(document.activeElement).toBe(cardEl(2))
    key('Escape')
    expect(onBack).toHaveBeenCalledTimes(1)
  })

  it('UIX-044/evento de teclado com defaultPrevented é ignorado pelo Big Box', () => {
    const onBack = vi.fn()
    const onPlay = vi.fn()
    render(<Board onBack={onBack} onPlay={onPlay} />)
    cardEl(1).focus()
    const handled = (k: string, init: KeyboardEventInit = {}) => {
      const ev = new KeyboardEvent('keydown', { key: k, cancelable: true, bubbles: true, ...init })
      ev.preventDefault()
      window.dispatchEvent(ev)
    }
    handled('ArrowRight')
    handled('Enter')
    handled('Enter', { shiftKey: true })
    handled('Escape')
    expect(document.activeElement).toBe(cardEl(1))
    expect(w.clicks).toEqual([])
    expect(onPlay).not.toHaveBeenCalled()
    expect(onBack).not.toHaveBeenCalled()
  })

  it('UIX-046/Shift+Enter mantido (repeat) num cartão chama onPlay uma vez', () => {
    const onPlay = vi.fn()
    render(<Board onBack={vi.fn()} onPlay={onPlay} />)
    cardEl(2).focus()
    key('Enter', { shiftKey: true })
    key('Enter', { shiftKey: true, repeat: true })
    key('Enter', { shiftKey: true, repeat: true })
    expect(onPlay).toHaveBeenCalledTimes(1)
    expect(onPlay).toHaveBeenCalledWith(2)
  })
})

describe('enterBigBox / exitBigBox', () => {
  it('UIX-044/fora do aplicativo desktop a tela cheia não faz nada e não falha', async () => {
    await expect(enterBigBox()).resolves.toBeUndefined()
    await expect(exitBigBox()).resolves.toBeUndefined()
  })
})

// The carousel's thumbnail strip inside the Big Box navigation [AJU-014].
describe('useBigBoxNav com a faixa de miniaturas do MediaCarousel (AJU-014)', () => {
  const media = (id: number, kind: MediaItemView['kind']): MediaItemView => ({ id, kind, provider: 'p', path: '', thumbPath: '', url: `/media/${id}.jpg`, thumbUrl: `/thumb/${id}.jpg` })
  // Display order: cover(1), screenshot(2), banner(3).
  const items = [media(1, 'cover'), media(2, 'screenshot'), media(3, 'banner')]

  function Page() {
    useBigBoxNav({ enabled: true, pageKey: 'library', onBack: vi.fn(), onPlay: vi.fn() })
    return (
      <div>
        <button type="button" data-bigbox-item data-x="120" data-y="300">Acima</button>
        <MediaCarousel items={items} resetKey={1} />
        <button type="button" data-bigbox-item data-x="120" data-y="700">Abaixo</button>
        <button type="button" data-bigbox-item data-x="900" data-y="500">Direita</button>
      </div>
    )
  }

  beforeEach(() => {
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [] })
    // Thumbnails sit side by side at y = 500; the other items use data-x / data-y.
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const m = /(\d+) de \d+$/.exec(this.getAttribute('aria-label') ?? '')
      const left = m ? (Number(m[1]) - 1) * 120 : Number(this.dataset.x ?? 0)
      const top = m ? 500 : Number(this.dataset.y ?? 0)
      return { left, top, right: left + 100, bottom: top + 30, width: 100, height: 30, x: left, y: top, toJSON: () => ({}) } as DOMRect
    })
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0)
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    Element.prototype.scrollIntoView = vi.fn() as unknown as typeof Element.prototype.scrollIntoView
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  const thumbs = () => within(screen.getByRole('group', { name: 'Miniaturas' })).getAllByRole('button')
  const shown = (c: HTMLElement) => c.querySelector('img')?.getAttribute('src')
  const countText = () => screen.getByText(/^\d+ de 3$/).textContent

  it('AJU-014/cada miniatura é um item data-bigbox-item', () => {
    render(<Page />)
    const all = bigBoxItems()
    expect(thumbs()).toHaveLength(3)
    for (const t of thumbs()) {
      expect(t.hasAttribute('data-bigbox-item')).toBe(true)
      expect(all).toContain(t)
    }
  })

  it('AJU-014/A (gamepad simulado) e Enter numa miniatura a põem em destaque', () => {
    const pressed = (index: number) => ({ buttons: Array.from({ length: 16 }, (_, i) => ({ pressed: i === index })), axes: [0, 0] }) as unknown as Gamepad
    let pad: Gamepad = pressed(-1)
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [pad] })
    let frame: FrameRequestCallback = () => undefined
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      frame = cb
      return 1
    })
    const { container } = render(<Page />)
    thumbs()[1].focus()
    pad = pressed(0)
    act(() => frame(0))
    pad = pressed(-1)
    act(() => frame(0))
    expect(countText()).toBe('2 de 3')
    expect(shown(container)).toBe('/media/2.jpg')
    // Enter on a non-card item is the browser's activation: the hook leaves it alone (jsdom does not click).
    const free = fireEvent.keyDown(thumbs()[2], { key: 'Enter' })
    expect(free).toBe(true)
    act(() => thumbs()[2].click())
    expect(countText()).toBe('3 de 3')
    expect(shown(container)).toBe('/media/3.jpg')
  })

  it('AJU-014/seta direita numa miniatura do meio troca a imagem uma vez e o Big Box não move o foco (defaultPrevented)', () => {
    const { container } = render(<Page />)
    thumbs()[0].focus()
    const notPrevented = fireEvent.keyDown(thumbs()[0], { key: 'ArrowRight' })
    expect(notPrevented).toBe(false)
    expect(countText()).toBe('2 de 3')
    expect(shown(container)).toBe('/media/2.jpg')
    expect(document.activeElement).toBe(thumbs()[1])
    expect(document.activeElement?.textContent).not.toBe('Direita')
  })

  it('AJU-014/seta direita na última miniatura não troca a imagem e o Big Box move o foco ao vizinho', () => {
    const { container } = render(<Page />)
    thumbs()[0].focus()
    fireEvent.keyDown(thumbs()[0], { key: 'ArrowRight' })
    fireEvent.keyDown(thumbs()[1], { key: 'ArrowRight' })
    expect(countText()).toBe('3 de 3')
    const last = thumbs()[2]
    expect(document.activeElement).toBe(last)
    fireEvent.keyDown(last, { key: 'ArrowRight' })
    expect(countText()).toBe('3 de 3')
    expect(shown(container)).toBe('/media/3.jpg')
    expect(document.activeElement?.textContent).toBe('Direita')
  })

  it('AJU-012/seta direita na miniatura 1 focada com destaque 0 vai para a 3 (a partir da focada)', () => {
    const { container } = render(<Page />)
    thumbs()[1].focus()
    const notPrevented = fireEvent.keyDown(thumbs()[1], { key: 'ArrowRight' })
    expect(notPrevented).toBe(false)
    expect(countText()).toBe('3 de 3')
    expect(shown(container)).toBe('/media/3.jpg')
    expect(document.activeElement).toBe(thumbs()[2])
  })

  it('AJU-014/cima e baixo numa miniatura são do Big Box', () => {
    const { container } = render(<Page />)
    thumbs()[1].focus()
    fireEvent.keyDown(thumbs()[1], { key: 'ArrowUp' })
    expect(document.activeElement?.textContent).toBe('Acima')
    thumbs()[1].focus()
    fireEvent.keyDown(thumbs()[1], { key: 'ArrowDown' })
    expect(document.activeElement?.textContent).toBe('Abaixo')
    expect(countText()).toBe('1 de 3')
    expect(shown(container)).toBe('/media/1.jpg')
  })
})
