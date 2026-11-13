import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render } from '@testing-library/react'
import { bigBoxItems, enterBigBox, exitBigBox, pickNext, useBigBoxNav, type Box } from './bigbox'

// A 2 x 2 grid: 0 top-left, 1 top-right, 2 bottom-left, 3 bottom-right.
const grid: Box[] = [
  { x: 0, y: 0, w: 100, h: 100 },
  { x: 120, y: 0, w: 100, h: 100 },
  { x: 0, y: 120, w: 100, h: 100 },
  { x: 120, y: 120, w: 100, h: 100 },
]

describe('pickNext (BIG-002)', () => {
  it('BIG-002/move o foco na direção pedida e fica quando não há item', () => {
    expect(pickNext(grid, 0, 'right')).toBe(1)
    expect(pickNext(grid, 0, 'down')).toBe(2)
    expect(pickNext(grid, 3, 'left')).toBe(2)
    expect(pickNext(grid, 1, 'down')).toBe(3)
    expect(pickNext(grid, 0, 'left')).toBe(0)
    expect(pickNext(grid, 0, 'up')).toBe(0)
  })

  it('BIG-002/prefere o item da mesma linha ou coluna ao que está na diagonal', () => {
    const boxes: Box[] = [
      { x: 0, y: 0, w: 100, h: 100 },
      { x: 300, y: 130, w: 100, h: 100 }, // far right, one row down
      { x: 130, y: 0, w: 100, h: 100 },   // right, same row
    ]
    expect(pickNext(boxes, 0, 'right')).toBe(2)
  })
})

// Board renders focusable cards with a play button, the way the library grid does.
function Board({ onExit }: { onExit: () => void }) {
  useBigBoxNav({ enabled: true, onExit })
  return (
    <div>
      {[1, 2, 3].map((n) => (
        <div key={n} data-bigbox-item tabIndex={0} data-testid={`item-${n}`} onClick={() => (window as unknown as { opened?: number }).opened = n}>
          <button type="button" data-bigbox-play onClick={() => ((window as unknown as { played?: number }).played = n)}>
            Jogar {n}
          </button>
        </div>
      ))}
    </div>
  )
}

describe('useBigBoxNav (BIG-002, BIG-003)', () => {
  beforeEach(() => {
    Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: () => [] })
    // jsdom has no layout: place the items as a grid, two per row, 120 px apart.
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const i = Array.from(document.querySelectorAll('[data-bigbox-item]')).indexOf(this)
      const left = (i % 2) * 120
      const top = Math.floor(i / 2) * 120
      return { left, top, right: left + 100, bottom: top + 100, width: 100, height: 100, x: left, y: top, toJSON: () => ({}) } as DOMRect
    })
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0)
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    delete (window as unknown as { opened?: number }).opened
    delete (window as unknown as { played?: number }).played
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('BIG-003/setas movem o foco e Enter abre o item focado', () => {
    render(<Board onExit={vi.fn()} />)
    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(document.activeElement).toBe(bigBoxItems()[1])
    fireEvent.keyDown(window, { key: 'Enter' })
    expect((window as unknown as { opened?: number }).opened).toBe(2)
  })

  it('BIG-002/Shift+Enter inicia o jogo focado pelo botão Jogar', () => {
    render(<Board onExit={vi.fn()} />)
    fireEvent.keyDown(window, { key: 'Enter', shiftKey: true })
    expect((window as unknown as { played?: number }).played).toBe(1)
  })

  it('BIG-001/Esc sai do Big Box', () => {
    const onExit = vi.fn()
    render(<Board onExit={onExit} />)
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onExit).toHaveBeenCalledTimes(1)
  })

  it('BIG-002/controle: direcional move o foco e A abre, Y inicia, B sai', () => {
    const onExit = vi.fn()
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
    render(<Board onExit={onExit} />)

    const step = (index: number) => {
      pad = pressed(index)
      act(() => frame(0))
      pad = pressed(-1)
      act(() => frame(0))
    }
    step(15) // right
    expect(document.activeElement).toBe(bigBoxItems()[1])
    step(0) // A
    expect((window as unknown as { opened?: number }).opened).toBe(2)
    step(3) // Y
    expect((window as unknown as { played?: number }).played).toBe(2)
    step(1) // B
    expect(onExit).toHaveBeenCalledTimes(1)
  })
})

describe('enterBigBox / exitBigBox (BIG-001)', () => {
  it('BIG-001/fora do aplicativo desktop a tela cheia não faz nada e não falha', () => {
    expect(() => enterBigBox()).not.toThrow()
    expect(() => exitBigBox()).not.toThrow()
  })
})
