// Gamepad and keyboard input of the Big Box [BBX-006, BBX-014, BBX-019, BBX-024].
import { useEffect, useRef } from 'react'
import type { NavAction } from './nav'

export const STICK_DEAD_ZONE = 0.6
export const REPEAT_DELAY_MS = 400
export const REPEAT_EVERY_MS = 120

export type InputAction = NavAction['type'] | 'scrollUp' | 'scrollDown'
type Axis = 'horizontal' | 'vertical' | 'grid'
type Dir = 'left' | 'right' | 'up' | 'down'

export function mapDirection(axis: Axis, dir: Dir): 'prev' | 'next' | 'up' | 'down' | null {
  if (axis === 'grid') return dir === 'left' ? 'prev' : dir === 'right' ? 'next' : dir
  if (axis === 'horizontal') return dir === 'left' ? 'prev' : dir === 'right' ? 'next' : null
  return dir === 'up' ? 'prev' : dir === 'down' ? 'next' : null
}

const BTN = { A: 0, B: 1, X: 2, Y: 3, LB: 4, RB: 5, SELECT: 8, UP: 12, DOWN: 13, LEFT: 14, RIGHT: 15 }
const pressedBtn = (pad: Gamepad, i: number) => !!pad.buttons[i]?.pressed

type Opts = { enabled: boolean; axis: Axis; screen: 'browse' | 'details'; onAction: (type: InputAction) => void }

export function useBigBoxInput(o: Opts): void {
  const ref = useRef(o)
  ref.current = o
  const { enabled } = o

  useEffect(() => {
    if (!enabled) return
    const dirAction = (dir: Dir): InputAction | null => {
      const { axis, screen } = ref.current
      if (screen === 'details') {
        return dir === 'left' ? 'prev' : dir === 'right' ? 'next' : dir
      }
      return mapDirection(axis, dir)
    }
    const emit = (a: InputAction | null) => {
      if (a) ref.current.onAction(a)
    }

    const onKey = (e: KeyboardEvent) => {
      const arrows: Record<string, Dir> = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' }
      if (arrows[e.key]) {
        const a = dirAction(arrows[e.key])
        if (a) {
          e.preventDefault()
          emit(a)
        }
        return
      }
      const inDetails = ref.current.screen === 'details'
      const keys: Record<string, InputAction> = {
        Enter: 'accept', d: 'details', D: 'details',
        PageUp: inDetails ? 'scrollUp' : 'tabPrev', PageDown: inDetails ? 'scrollDown' : 'tabNext',
        Escape: 'back', m: 'toggleSound', M: 'toggleSound',
      }
      const a = keys[e.key]
      if (!a) return
      e.preventDefault()
      if (!e.repeat) emit(a)
    }
    window.addEventListener('keydown', onKey)

    const prevBtn: Record<number, boolean> = {}
    const blocked: Record<number, boolean> = {}
    let primed = false
    const held = new Map<string, number>() // repeat key -> next emission time
    let raf = 0

    const repeat = (key: string, down: boolean, t: number, action: InputAction | null) => {
      if (!down) {
        held.delete(key)
        return
      }
      const next = held.get(key)
      if (next === undefined) {
        held.set(key, t + REPEAT_DELAY_MS)
        emit(action)
      } else if (t >= next) {
        held.set(key, t + REPEAT_EVERY_MS)
        emit(action)
      }
    }

    const tick = (t: number) => {
      raf = window.requestAnimationFrame(tick)
      const pad = Array.from(navigator.getGamepads?.() ?? []).find((p) => p && p.connected)
      if (!pad) {
        held.clear()
        return
      }
      const select = pressedBtn(pad, BTN.SELECT)
      const rise = (i: number) => {
        const now = pressedBtn(pad, i)
        const was = prevBtn[i] ?? (primed ? false : now)
        prevBtn[i] = now
        return now && !was
      }
      const aR = rise(BTN.A), xR = rise(BTN.X), lbR = rise(BTN.LB), rbR = rise(BTN.RB), yR = rise(BTN.Y), bR = rise(BTN.B)
      if (primed) {
        if (!select && aR) emit('accept')
        if (xR) emit('details')
        if (lbR) emit('tabPrev')
        if (rbR) emit('tabNext')
        if (!select && yR) emit('toggleSound')
        if (!select && bR) emit('back')
      }
      primed = true
      const ax = (i: number) => pad.axes[i] ?? 0
      // Select + up/down is the volume chord of the agent: the d-pad up/down count as released while Select is
      // held and until they are released, so letting Select go first does not emit [BBX-057].
      const vert = (i: number) => {
        const down = pressedBtn(pad, i)
        if (!down) {
          blocked[i] = false
        } else if (select) {
          blocked[i] = true
        }
        return down && !blocked[i]
      }
      const dirs: Array<[Dir, boolean]> = [
        ['left', pressedBtn(pad, BTN.LEFT) || ax(0) < -STICK_DEAD_ZONE],
        ['right', pressedBtn(pad, BTN.RIGHT) || ax(0) > STICK_DEAD_ZONE],
        ['up', vert(BTN.UP) || ax(1) < -STICK_DEAD_ZONE],
        ['down', vert(BTN.DOWN) || ax(1) > STICK_DEAD_ZONE],
      ]
      for (const [d, down] of dirs) repeat(d, down, t, dirAction(d))
      const details = ref.current.screen === 'details'
      repeat('scrollUp', details && ax(3) < -STICK_DEAD_ZONE, t, 'scrollUp')
      repeat('scrollDown', details && ax(3) > STICK_DEAD_ZONE, t, 'scrollDown')
    }
    raf = window.requestAnimationFrame(tick)

    return () => {
      window.removeEventListener('keydown', onKey)
      window.cancelAnimationFrame(raf)
    }
  }, [enabled])
}
