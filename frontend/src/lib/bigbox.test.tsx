import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render } from '@testing-library/react'
import { bigBoxItems, enterBigBox, exitBigBox, gridNext, pickNext, useBigBoxNav, type Box } from './bigbox'

// A 2 x 2 grid of boxes: 0 top-left, 1 top-right, 2 bottom-left, 3 bottom-right.
const grid: Box[] = [
  { x: 0, y: 0, w: 100, h: 100 },
  { x: 120, y: 0, w: 100, h: 100 },
  { x: 0, y: 120, w: 100, h: 100 },
  { x: 120, y: 120, w: 100, h: 100 },
]

describe('pickNext (pages without cards)', () => {
  it('BIG-002/move o foco na direção pedida e fica quando não há item', () => {
    expect(pickNext(grid, 0, 'right')).toBe(1)
    expect(pickNext(grid, 0, 'down')).toBe(2)
    expect(pickNext(grid, 3, 'left')).toBe(2)
    expect(pickNext(grid, 0, 'left')).toBe(0)
    expect(pickNext(grid, 0, 'up')).toBe(0)
  })
})

// Board renders three game cards like the library: a title, "Jogar" and "Sem contar tempo", in a
// 2-column grid (card 0 and 1 in the first row, card 2 below card 0).
function Board({ onBack }: { onBack: () => void }) {
  useBigBoxNav({ enabled: true, pageKey: 'library', onBack })
  return (
    <div>
      {[1, 2, 3].map((n) => (
        <article key={n}>
          <div data-bigbox-item tabIndex={0} onClick={() => ((window as unknown as { opened?: number }).opened = n)}>
            Jogo {n}
          </div>
          <button type="button" data-bigbox-item data-bigbox-play onClick={() => ((window as unknown as { played?: number }).played = n)}>
            Jogar {n}
          </button>
          <button type="button" data-bigbox-item onClick={() => ((window as unknown as { untracked?: number }).untracked = n)}>
            Sem contar {n}
          </button>
        </article>
      ))}
    </div>
  )
}

// jsdom has no layout: each card is placed on a grid, two per row, 120 px apart; its items sit at the
// top of the card, one under the other.
function placeCards() {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const articles = Array.from(document.querySelectorAll('article'))
    const card = this.closest('article')
    const ci = card ? articles.indexOf(card) : -1
    const left = (Math.max(ci, 0) % 2) * 120
    const top = Math.floor(Math.max(ci, 0) / 2) * 120
    const slot = card ? Array.from(card.querySelectorAll('[data-bigbox-item]')).indexOf(this) : 0
    const y = top + slot * 30
    return { left, top: y, right: left + 100, bottom: y + 30, width: 100, height: 30, x: left, y, toJSON: () => ({}) } as DOMRect
  })
}

const win = window as unknown as { opened?: number; played?: number; untracked?: number }

describe('useBigBoxNav (BIG-002, BIG-003)', () => {
  beforeEach(() => {
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [] })
    placeCards()
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0)
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    delete win.opened
    delete win.played
    delete win.untracked
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('BIG-002/o foco começa no título do primeiro cartão', () => {
    render(<Board onBack={vi.fn()} />)
    expect(document.activeElement).toBe(bigBoxItems()[0])
  })

  it('BIG-002/para baixo: título, Jogar, Sem contar tempo; depois o cartão de baixo', () => {
    render(<Board onBack={vi.fn()} />)
    const [t1, j1, s1, , , , t3] = bigBoxItems()
    expect(document.activeElement).toBe(t1)
    fireEvent.keyDown(window, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(j1)
    fireEvent.keyDown(window, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(s1)
    fireEvent.keyDown(window, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(t3)
  })

  it('BIG-002/para cima volta pela ordem do cartão', () => {
    render(<Board onBack={vi.fn()} />)
    const [t1, j1, s1] = bigBoxItems()
    fireEvent.keyDown(window, { key: 'ArrowDown' })
    fireEvent.keyDown(window, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(s1)
    fireEvent.keyDown(window, { key: 'ArrowUp' })
    expect(document.activeElement).toBe(j1)
    fireEvent.keyDown(window, { key: 'ArrowUp' })
    expect(document.activeElement).toBe(t1)
  })

  it('BIG-002/esquerda vai para o cartão anterior, e no primeiro cartão não faz nada', () => {
    render(<Board onBack={vi.fn()} />)
    const items = bigBoxItems()
    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(document.activeElement).toBe(items[3]) // title of card 2
    fireEvent.keyDown(window, { key: 'ArrowLeft' })
    expect(document.activeElement).toBe(items[0]) // back to card 1
    fireEvent.keyDown(window, { key: 'ArrowLeft' })
    expect(document.activeElement).toBe(items[0]) // first card: stays
  })

  it('BIG-002/a esquerda a partir de Jogar vai para o Jogar do cartão anterior, não para o próprio cartão', () => {
    render(<Board onBack={vi.fn()} />)
    const items = bigBoxItems()
    fireEvent.keyDown(window, { key: 'ArrowRight' }) // card 2 title
    fireEvent.keyDown(window, { key: 'ArrowDown' }) // card 2 Jogar
    expect(document.activeElement).toBe(items[4])
    fireEvent.keyDown(window, { key: 'ArrowLeft' })
    expect(document.activeElement).toBe(items[1]) // card 1 Jogar
  })

  it('BIG-003/Enter ativa o item focado (ativação nativa do navegador)', () => {
    render(<Board onBack={vi.fn()} />)
    fireEvent.keyDown(window, { key: 'ArrowRight' })
    fireEvent.click(document.activeElement as HTMLElement)
    expect(win.opened).toBe(2)
  })

  it('BIG-002/Shift+Enter inicia o jogo do cartão focado', () => {
    render(<Board onBack={vi.fn()} />)
    fireEvent.keyDown(window, { key: 'Enter', shiftKey: true })
    expect(win.played).toBe(1)
  })

  it('BIG-001/Esc chama o voltar da página', () => {
    const onBack = vi.fn()
    render(<Board onBack={onBack} />)
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onBack).toHaveBeenCalledTimes(1)
  })

  it('BIG-002/controle: direcional move, A ativa, Y inicia, B volta', () => {
    const onBack = vi.fn()
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
    render(<Board onBack={onBack} />)

    const step = (index: number) => {
      pad = pressed(index)
      act(() => frame(0))
      pad = pressed(-1)
      act(() => frame(0))
    }
    step(15) // right: card 2 title
    expect(document.activeElement).toBe(bigBoxItems()[3])
    step(0) // A
    expect(win.opened).toBe(2)
    step(3) // Y
    expect(win.played).toBe(2)
    step(1) // B
    expect(onBack).toHaveBeenCalledTimes(1)
  })
})

describe('gridNext (cards)', () => {
  it('BIG-002/cartões em duas linhas: cima e baixo cruzam a linha pela mesma coluna', () => {
    const card = (id: string) => {
      const a = document.createElement('article')
      const items = [0, 1].map((i) => {
        const el = document.createElement('div')
        el.setAttribute('data-bigbox-item', `${id}-${i}`)
        a.appendChild(el)
        return el
      })
      document.body.appendChild(a)
      return { items, el: a }
    }
    const a = card('a')
    const b = card('b')
    const c = card('c')
    // Row 1: a, b. Row 2: c, under a.
    const rows = [[{ items: a.items }, { items: b.items }], [{ items: c.items }]]
    expect(gridNext(rows, a.items[1], 'down')).toBe(c.items[0]) // last item of a: the card below, first item
    expect(gridNext(rows, c.items[0], 'up')).toBe(a.items[1]) // first item of c: last item of the card above
    expect(gridNext(rows, a.items[1], 'right')).toBe(b.items[1]) // same slot of the next card
    expect(gridNext(rows, b.items[0], 'left')).toBe(a.items[0])
    expect(gridNext(rows, b.items[1], 'down')).toBe(c.items[0]) // the column clamps to the card below a
  })
})

describe('enterBigBox / exitBigBox (BIG-001)', () => {
  it('BIG-001/fora do aplicativo desktop a tela cheia não faz nada e não falha', async () => {
    await expect(enterBigBox()).resolves.toBeUndefined()
    await expect(exitBigBox()).resolves.toBeUndefined()
  })
})
