import { renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  REPEAT_DELAY_MS,
  REPEAT_EVERY_MS,
  STICK_DEAD_ZONE,
  mapDirection,
  useBigBoxInput,
  type InputAction,
} from './useBigBoxInput'

// Standard mapping: 0 A, 1 B, 2 X, 3 Y, 4 LB, 5 RB, 8 Select, 12 up, 13 down, 14 left, 15 right.
const A = 0, B = 1, X = 2, Y = 3, LB = 4, RB = 5, SELECT = 8, UP = 12, DOWN = 13, LEFT = 14, RIGHT = 15

let pressed: Set<number>
let axes: number[]
let rafQueue: Map<number, FrameRequestCallback>
let rafId: number
let cancelSpy: ReturnType<typeof vi.fn>
let padPresent: boolean
let getGamepads: ReturnType<typeof vi.fn>

function makePad(): Gamepad {
  const buttons = Array.from({ length: 17 }, (_, i) => ({
    pressed: pressed.has(i), touched: pressed.has(i), value: pressed.has(i) ? 1 : 0,
  }))
  return { connected: true, mapping: 'standard', buttons, axes: [...axes] } as unknown as Gamepad
}

// Runs one animation frame at the current (fake) time.
function frame() {
  const q = [...rafQueue.entries()]
  rafQueue.clear()
  const now = performance.now()
  for (const [, cb] of q) cb(now)
}

// Advances fake time by ms, running a frame every 16 ms (like a 60 Hz display).
function advance(ms: number) {
  let left = ms
  while (left > 0) {
    const d = Math.min(16, left)
    vi.advanceTimersByTime(d)
    left -= d
    frame()
  }
}

function setup(o: Partial<{ enabled: boolean; axis: 'horizontal' | 'vertical' | 'grid'; screen: 'browse' | 'details' }> = {}) {
  const calls: InputAction[] = []
  const props = { enabled: true, axis: 'horizontal' as const, screen: 'browse' as const, ...o }
  const r = renderHook((p: typeof props) => useBigBoxInput({ ...p, onAction: (t) => calls.push(t) }), { initialProps: props })
  frame()
  return { calls, ...r }
}

function key(k: string, repeat = false) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key: k, repeat, bubbles: true }))
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date', 'performance'] })
  pressed = new Set()
  axes = [0, 0, 0, 0]
  rafQueue = new Map()
  rafId = 0
  padPresent = true
  cancelSpy = vi.fn((id: number) => { rafQueue.delete(id) })
  getGamepads = vi.fn(() => (padPresent ? [makePad()] : []))
  Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: getGamepads })
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
    rafId += 1
    rafQueue.set(rafId, cb)
    return rafId
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(cancelSpy as unknown as (id: number) => void)
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('mapDirection', () => {
  it.each([
    ['horizontal', 'left', 'prev'],
    ['horizontal', 'right', 'next'],
    ['horizontal', 'up', null],
    ['horizontal', 'down', null],
    ['vertical', 'up', 'prev'],
    ['vertical', 'down', 'next'],
    ['vertical', 'left', null],
    ['vertical', 'right', null],
  ] as const)('BBX-024/%s %s -> %s', (axis, dir, want) => {
    expect(mapDirection(axis, dir)).toBe(want)
  })
})

describe('constants', () => {
  it('BBX-006/zona morta 0,6, repetição 400 ms e depois 120 ms', () => {
    expect(STICK_DEAD_ZONE).toBe(0.6)
    expect(REPEAT_DELAY_MS).toBe(400)
    expect(REPEAT_EVERY_MS).toBe(120)
  })
})

describe('useBigBoxInput gamepad', () => {
  it('BBX-014/A mantido emite accept uma vez; X, LB, RB, Y e B emitem na borda de subida', () => {
    const { calls } = setup()
    pressed.add(A); advance(500)
    expect(calls).toEqual(['accept'])
    pressed.clear(); advance(32)
    pressed.add(X); advance(32); pressed.delete(X); advance(32)
    pressed.add(LB); advance(32); pressed.delete(LB); advance(32)
    pressed.add(RB); advance(32); pressed.delete(RB); advance(32)
    pressed.add(Y); advance(32); pressed.delete(Y); advance(32)
    pressed.add(B); advance(32); pressed.delete(B); advance(32)
    expect(calls).toEqual(['accept', 'details', 'tabPrev', 'tabNext', 'toggleSound', 'back'])
  })

  it('BBX-006/direcional repete depois de 400 ms a cada 120 ms', () => {
    const { calls } = setup()
    pressed.add(RIGHT)
    advance(16)
    expect(calls).toEqual(['next'])
    advance(REPEAT_DELAY_MS - 32)
    expect(calls).toEqual(['next'])
    advance(48)
    expect(calls.length).toBe(2)
    advance(REPEAT_EVERY_MS * 3)
    expect(calls.length).toBeGreaterThanOrEqual(4)
    expect(calls.length).toBeLessThanOrEqual(6)
    const n = calls.length
    pressed.clear()
    advance(1000)
    expect(calls.length).toBe(n)
  })

  it('BBX-006/analógico esquerdo respeita a zona morta de 0,6', () => {
    const { calls } = setup()
    axes[0] = 0.59; advance(100)
    expect(calls).toEqual([])
    axes[0] = 0.61; advance(16)
    expect(calls).toEqual(['next'])
    axes[0] = 0; advance(32)
    axes[0] = -0.9; advance(16)
    expect(calls).toEqual(['next', 'prev'])
  })

  it('BBX-024/eixo vertical: direcional cima/baixo viram prev/next e esquerda/direita são ignorados', () => {
    const { calls } = setup({ axis: 'vertical' })
    pressed.add(UP); advance(16); pressed.clear(); advance(16)
    pressed.add(DOWN); advance(16); pressed.clear(); advance(16)
    pressed.add(LEFT); advance(16); pressed.clear(); advance(16)
    pressed.add(RIGHT); advance(16); pressed.clear(); advance(16)
    expect(calls).toEqual(['prev', 'next'])
  })

  it('BBX-024/eixo horizontal: esquerda/direita viram prev/next', () => {
    const { calls } = setup({ axis: 'horizontal' })
    pressed.add(LEFT); advance(16); pressed.clear(); advance(16)
    pressed.add(RIGHT); advance(16); pressed.clear(); advance(16)
    expect(calls).toEqual(['prev', 'next'])
  })

  it('BBX-016/analógico direito emite scrollUp e scrollDown na tela de detalhes', () => {
    const { calls } = setup({ screen: 'details' })
    axes[3] = -0.9; advance(16)
    axes[3] = 0; advance(32)
    axes[3] = 0.9; advance(16)
    expect(calls).toEqual(['scrollUp', 'scrollDown'])
  })

  it('BBX-036/analógico direito emite scrollUp/scrollDown em details com qualquer foco (a tela não sabe do foco)', () => {
    const { calls } = setup({ screen: 'details' })
    axes[3] = -0.9; advance(16); axes[3] = 0; advance(32)
    axes[3] = 0.9; advance(16); axes[3] = 0; advance(32)
    expect(calls).toEqual(['scrollUp', 'scrollDown'])
  })

  it('BBX-032/em details direcional e analógico esquerdo emitem up, down, prev e next', () => {
    const { calls } = setup({ screen: 'details' })
    pressed.add(UP); advance(16); pressed.clear(); advance(16)
    pressed.add(DOWN); advance(16); pressed.clear(); advance(16)
    pressed.add(LEFT); advance(16); pressed.clear(); advance(16)
    pressed.add(RIGHT); advance(16); pressed.clear(); advance(16)
    axes[1] = -0.9; advance(16); axes[1] = 0; advance(32)
    axes[1] = 0.9; advance(16); axes[1] = 0; advance(32)
    axes[0] = -0.9; advance(16); axes[0] = 0; advance(32)
    axes[0] = 0.9; advance(16)
    expect(calls).toEqual(['up', 'down', 'prev', 'next', 'up', 'down', 'prev', 'next'])
  })

  it('BBX-019/Select apertado: B e Y são ignorados', () => {
    const { calls } = setup()
    pressed.add(SELECT); advance(32)
    pressed.add(B); advance(32); pressed.delete(B); advance(32)
    pressed.add(Y); advance(32); pressed.delete(Y); advance(32)
    expect(calls).toEqual([])
  })

  it('BBX-019/B apertado com Select já apertado não dispara nem ao soltar o Select com B ainda apertado', () => {
    const { calls } = setup()
    pressed.add(SELECT); advance(32)
    pressed.add(B); advance(32)
    pressed.delete(SELECT); advance(64)
    expect(calls).toEqual([])
    pressed.delete(B); advance(32)
    expect(calls).toEqual([])
  })

  // alterado em 2026-11-22 (fase 15): com Select apertado o A também é ignorado (BBX-057); X, LB, RB e → seguem valendo.
  it('BBX-057/X, LB, RB e direcional ←/→ continuam valendo com Select apertado', () => {
    const { calls } = setup()
    pressed.add(SELECT); advance(32)
    for (const b of [X, LB, RB, RIGHT]) {
      pressed.add(b); advance(32); pressed.delete(b); advance(32)
    }
    expect(calls).toEqual(['details', 'tabPrev', 'tabNext', 'next'])
  })

  it('BBX-057/Select apertado: A não emite accept, nem ao soltar o Select com A ainda apertado', () => {
    const { calls } = setup()
    pressed.add(SELECT); advance(32)
    pressed.add(A); advance(32); pressed.delete(A); advance(32)
    expect(calls).toEqual([])
    pressed.add(A); advance(32)
    pressed.delete(SELECT); advance(64)
    expect(calls).toEqual([])
    pressed.delete(A); advance(32)
    expect(calls).toEqual([])
  })

  it('BBX-057/Select apertado: B e Y continuam ignorados e ↑/↓ do direcional (12 e 13) contam como soltos', () => {
    const { calls } = setup({ axis: 'vertical' })
    pressed.add(SELECT); advance(32)
    for (const b of [B, Y, UP, DOWN]) {
      pressed.add(b); advance(32); pressed.delete(b); advance(32)
    }
    expect(calls).toEqual([])
    pressed.add(UP); advance(REPEAT_DELAY_MS + REPEAT_EVERY_MS * 3)
    expect(calls).toEqual([])
  })

  it('BBX-057/↑ apertado com Select e Select solto depois: a borda de subida perdida não emite (eixo vertical e details)', () => {
    for (const o of [{ axis: 'vertical' as const }, { screen: 'details' as const }]) {
      const { calls, unmount } = setup(o)
      pressed.add(SELECT); advance(32)
      pressed.add(UP); pressed.add(DOWN); advance(32)
      pressed.delete(SELECT); advance(REPEAT_DELAY_MS + 200)
      expect(calls).toEqual([])
      pressed.clear(); advance(32)
      // Pressed again without Select it works as before.
      pressed.add(UP); advance(16)
      expect(calls.length).toBe(1)
      pressed.clear(); advance(32)
      unmount()
    }
  })

  it('BBX-057/Select apertado: o analógico esquerdo e o direito continuam valendo', () => {
    const { calls } = setup({ axis: 'vertical' })
    pressed.add(SELECT); advance(32)
    axes[1] = -0.9; advance(16); axes[1] = 0; advance(32)
    axes[1] = 0.9; advance(16); axes[1] = 0; advance(32)
    expect(calls).toEqual(['prev', 'next'])
    const d = setup({ screen: 'details' })
    axes[3] = -0.9; advance(16); axes[3] = 0; advance(32)
    expect(d.calls).toEqual(['scrollUp'])
  })

  it('BBX-057/sem Select, A, B, Y e o direcional ↑/↓ funcionam como antes', () => {
    const { calls } = setup({ axis: 'vertical' })
    for (const b of [A, B, Y, UP, DOWN]) {
      pressed.add(b); advance(32); pressed.delete(b); advance(32)
    }
    expect(calls).toEqual(['accept', 'back', 'toggleSound', 'prev', 'next'])
  })

  it('BBX-057/o teclado não é afetado por Select', () => {
    const { calls } = setup({ axis: 'vertical' })
    pressed.add(SELECT); advance(32)
    key('Enter'); key('ArrowUp'); key('Escape')
    expect(calls).toEqual(['accept', 'prev', 'back'])
  })

  it('BBX-019/Y sem Select liga o som', () => {
    const { calls } = setup()
    pressed.add(Y); advance(32)
    expect(calls).toEqual(['toggleSound'])
  })

  it('BBX-019/desabilitado não lê o controle nem agenda frame', () => {
    const { calls } = setup({ enabled: false })
    pressed.add(A); advance(200)
    key('Enter')
    expect(calls).toEqual([])
    expect(getGamepads).not.toHaveBeenCalled()
    expect(rafQueue.size).toBe(0)
  })

  it('BBX-019/habilitar depois volta a ler; desabilitar de novo cancela o frame', () => {
    const { calls, rerender } = setup({ enabled: false })
    rerender({ enabled: true, axis: 'horizontal', screen: 'browse' })
    frame()
    pressed.add(A); advance(32)
    expect(calls).toEqual(['accept'])
    rerender({ enabled: false, axis: 'horizontal', screen: 'browse' })
    expect(rafQueue.size).toBe(0)
  })

  it('BBX-019/desmontar cancela o frame, remove o listener e não deixa temporizador', () => {
    const { calls, unmount } = setup()
    expect(rafQueue.size).toBe(1)
    pressed.add(RIGHT); advance(16)
    expect(calls).toEqual(['next'])
    unmount()
    expect(cancelSpy).toHaveBeenCalled()
    expect(rafQueue.size).toBe(0)
    expect(vi.getTimerCount()).toBe(0)
    key('Enter'); key('ArrowRight')
    advance(1000)
    expect(calls).toEqual(['next'])
  })

  it('BBX-019/sem controle conectado não quebra e o teclado segue valendo', () => {
    padPresent = false
    const { calls } = setup()
    advance(100)
    key('Enter')
    expect(calls).toEqual(['accept'])
  })
})

// alterado em 2026-11-22 (fase 15): eixo 'grid' (BBX-048).
describe('useBigBoxInput eixo grid', () => {
  it('BBX-048/grid: direcional emite prev/next para ←/→ e up/down para ↑/↓ em Navegar', () => {
    const { calls } = setup({ axis: 'grid' })
    for (const b of [LEFT, RIGHT, UP, DOWN]) {
      pressed.add(b); advance(16); pressed.clear(); advance(16)
    }
    expect(calls).toEqual(['prev', 'next', 'up', 'down'])
  })

  it('BBX-048/grid: analógico esquerdo emite prev/next no eixo X e up/down no eixo Y', () => {
    const { calls } = setup({ axis: 'grid' })
    axes[0] = -0.9; advance(16); axes[0] = 0; advance(32)
    axes[0] = 0.9; advance(16); axes[0] = 0; advance(32)
    axes[1] = -0.9; advance(16); axes[1] = 0; advance(32)
    axes[1] = 0.9; advance(16)
    expect(calls).toEqual(['prev', 'next', 'up', 'down'])
  })

  it('BBX-048/grid: o teclado emite prev/next e up/down', () => {
    const { calls } = setup({ axis: 'grid' })
    key('ArrowLeft'); key('ArrowRight'); key('ArrowUp'); key('ArrowDown')
    expect(calls).toEqual(['prev', 'next', 'up', 'down'])
  })

  it('BBX-048/grid: segurar ↓ repete depois de 400 ms a cada 120 ms', () => {
    const { calls } = setup({ axis: 'grid' })
    pressed.add(DOWN)
    advance(16)
    expect(calls).toEqual(['down'])
    advance(REPEAT_DELAY_MS + REPEAT_EVERY_MS)
    expect(calls.length).toBeGreaterThanOrEqual(2)
    expect(calls.every((c) => c === 'down')).toBe(true)
  })

  it('BBX-048/grid: em Detalhes as direções seguem BBX-032 (up, down, prev, next)', () => {
    const { calls } = setup({ axis: 'grid', screen: 'details' })
    key('ArrowUp'); key('ArrowDown'); key('ArrowLeft'); key('ArrowRight')
    expect(calls).toEqual(['up', 'down', 'prev', 'next'])
  })

  it('BBX-057/grid: Select apertado ignora ↑/↓ do direcional mas ←/→ e o analógico valem', () => {
    const { calls } = setup({ axis: 'grid' })
    pressed.add(SELECT); advance(32)
    for (const b of [UP, DOWN]) {
      pressed.add(b); advance(32); pressed.delete(b); advance(32)
    }
    expect(calls).toEqual([])
    pressed.add(RIGHT); advance(16); pressed.delete(RIGHT); advance(32)
    axes[1] = 0.9; advance(16)
    expect(calls).toEqual(['next', 'down'])
  })

  it('BBX-048/horizontal e vertical continuam como antes (↑/↓ sem ação no horizontal; ←/→ sem ação no vertical)', () => {
    const h = setup({ axis: 'horizontal' })
    key('ArrowUp'); key('ArrowDown')
    expect(h.calls).toEqual([])
    h.unmount()
    const v = setup({ axis: 'vertical' })
    key('ArrowLeft'); key('ArrowRight')
    expect(v.calls).toEqual([])
  })
})

describe('useBigBoxInput teclado', () => {
  it('BBX-014/teclas Enter, D, PageUp, PageDown, Esc e M emitem accept, details, tabPrev, tabNext, back e toggleSound', () => {
    const { calls } = setup()
    for (const k of ['Enter', 'd', 'PageUp', 'PageDown', 'Escape', 'm']) key(k)
    expect(calls).toEqual(['accept', 'details', 'tabPrev', 'tabNext', 'back', 'toggleSound'])
  })

  it('BBX-014/Enter com repeat não emite accept de novo', () => {
    const { calls } = setup()
    key('Enter')
    key('Enter', true)
    key('Enter', true)
    expect(calls).toEqual(['accept'])
  })

  it('BBX-014/D, PageUp, PageDown, Esc e M com repeat são ignorados', () => {
    const { calls } = setup()
    for (const k of ['d', 'PageUp', 'PageDown', 'Escape', 'm']) key(k, true)
    expect(calls).toEqual([])
  })

  it('BBX-006/setas repetem com repeat (eixo horizontal)', () => {
    const { calls } = setup()
    key('ArrowRight'); key('ArrowRight', true); key('ArrowLeft', true)
    expect(calls).toEqual(['next', 'next', 'prev'])
  })

  it('BBX-024/eixo vertical: setas cima/baixo viram prev/next e esquerda/direita são ignoradas', () => {
    const { calls } = setup({ axis: 'vertical' })
    key('ArrowUp'); key('ArrowDown'); key('ArrowLeft'); key('ArrowRight')
    expect(calls).toEqual(['prev', 'next'])
  })

  it('BBX-016/setas cima/baixo na tela de detalhes rolam ou navegam sem erro (eixo horizontal)', () => {
    const { calls } = setup({ screen: 'details' })
    key('ArrowDown')
    expect(calls.every((c) => ['down', 'scrollDown'].includes(c))).toBe(true)
  })

  it('BBX-036/em details PageUp e PageDown emitem scrollUp e scrollDown', () => {
    const { calls } = setup({ screen: 'details' })
    key('PageUp'); key('PageDown')
    expect(calls).toEqual(['scrollUp', 'scrollDown'])
  })

  it('BBX-036/em browse PageUp e PageDown continuam tabPrev e tabNext', () => {
    const { calls } = setup({ screen: 'browse' })
    key('PageUp'); key('PageDown')
    expect(calls).toEqual(['tabPrev', 'tabNext'])
  })

  it('BBX-032/em details as setas emitem up, down, prev e next', () => {
    const { calls } = setup({ screen: 'details' })
    key('ArrowUp'); key('ArrowDown'); key('ArrowLeft'); key('ArrowRight')
    expect(calls).toEqual(['up', 'down', 'prev', 'next'])
  })

  it('BBX-019/desabilitado ignora o teclado e desmontar remove o listener', () => {
    const off = setup({ enabled: false })
    key('Enter')
    expect(off.calls).toEqual([])
    const on = setup()
    on.unmount()
    key('Enter')
    expect(on.calls).toEqual([])
  })
})
