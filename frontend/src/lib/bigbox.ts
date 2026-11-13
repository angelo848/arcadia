import { useEffect, useRef } from 'react'
import { WindowFullscreen, WindowUnfullscreen } from '../../wailsjs/runtime/runtime'

// Big Box (phase 8, BIG-001..BIG-004): the library in full screen, driven by the gamepad or the keyboard.

export type Direction = 'up' | 'down' | 'left' | 'right'

// Box is the position of a focusable item on the screen, in pixels.
export type Box = { x: number; y: number; w: number; h: number }

// pickNext returns the index of the item to focus when the focus moves in a direction from items[current]
// (BIG-002). It takes the nearest item in that direction, preferring the same row or column. When no item
// lies that way, the focus stays.
export function pickNext(items: Box[], current: number, dir: Direction): number {
  const from = items[current]
  if (!from) {
    return current
  }
  const cx = from.x + from.w / 2
  const cy = from.y + from.h / 2
  let best = current
  let bestScore = Infinity
  items.forEach((it, i) => {
    if (i === current) {
      return
    }
    const x = it.x + it.w / 2
    const y = it.y + it.h / 2
    const dx = x - cx
    const dy = y - cy
    // Distance along the direction of movement, and across it.
    const along = dir === 'left' ? -dx : dir === 'right' ? dx : dir === 'up' ? -dy : dy
    if (along <= 0) {
      return
    }
    const across = dir === 'left' || dir === 'right' ? Math.abs(dy) : Math.abs(dx)
    const score = along + 2 * across
    if (score < bestScore) {
      bestScore = score
      best = i
    }
  })
  return best
}

// enterBigBox puts the window in full screen (BIG-001). Outside the desktop shell (tests, browser) the
// call does nothing, so the page still works.
export function enterBigBox(): void {
  try {
    WindowFullscreen()
  } catch {
    // no window runtime: nothing to change
  }
}

// exitBigBox leaves full screen (BIG-001).
export function exitBigBox(): void {
  try {
    WindowUnfullscreen()
  } catch {
    // no window runtime: nothing to change
  }
}

// bigBoxItems returns the focusable items of the Big Box, in reading order.
export function bigBoxItems(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>('[data-bigbox-item]'))
}

// useBigBoxNav reads the keyboard and the gamepad while the Big Box is on: the directions move the focus,
// A (or Enter) opens the focused game, Y (or Shift+Enter) starts it, B (or Esc) leaves the Big Box (BIG-002,
// BIG-003). Gamepad buttons follow the standard mapping of the Gamepad API.
export function useBigBoxNav({ enabled, onExit }: { enabled: boolean; onExit: () => void }): void {
  const exitRef = useRef(onExit)
  exitRef.current = onExit

  useEffect(() => {
    if (!enabled) {
      return undefined
    }
    let current = 0
    const focusAt = (i: number) => {
      const items = bigBoxItems()
      if (items.length === 0) {
        return
      }
      current = Math.min(Math.max(i, 0), items.length - 1)
      items[current].focus()
      items[current].scrollIntoView?.({ block: "nearest" })
    }
    const boxes = (): Box[] =>
      bigBoxItems().map((el) => {
        const r = el.getBoundingClientRect()
        return { x: r.left, y: r.top, w: r.width, h: r.height }
      })
    const move = (dir: Direction) => {
      const items = bigBoxItems()
      const focused = items.indexOf(document.activeElement as HTMLElement)
      if (focused >= 0) {
        current = focused
      }
      focusAt(pickNext(boxes(), current, dir))
    }
    const open = () => {
      const items = bigBoxItems()
      items[current]?.click()
    }
    const play = () => {
      const items = bigBoxItems()
      const btn = items[current]?.querySelector<HTMLElement>('[data-bigbox-play]')
      btn?.click()
    }
    const press = (action: 'up' | 'down' | 'left' | 'right' | 'open' | 'play' | 'exit') => {
      switch (action) {
        case 'up':
        case 'down':
        case 'left':
        case 'right':
          move(action)
          return
        case 'open':
          open()
          return
        case 'play':
          play()
          return
        case 'exit':
          exitRef.current()
      }
    }

    const onKey = (e: KeyboardEvent) => {
      const map: Record<string, Parameters<typeof press>[0] | undefined> = {
        ArrowUp: 'up',
        ArrowDown: 'down',
        ArrowLeft: 'left',
        ArrowRight: 'right',
        Escape: 'exit',
      }
      if (e.key === 'Enter') {
        press(e.shiftKey ? 'play' : 'open')
      } else if (map[e.key]) {
        press(map[e.key] as Parameters<typeof press>[0])
      } else {
        return
      }
      e.preventDefault()
    }

    // Standard gamepad mapping: 0 = A, 1 = B, 3 = Y, 12..15 = the directional pad.
    const buttonActions: Array<[number, Parameters<typeof press>[0]]> = [
      [12, 'up'],
      [13, 'down'],
      [14, 'left'],
      [15, 'right'],
      [0, 'open'],
      [3, 'play'],
      [1, 'exit'],
    ]
    const previous = new Map<number, boolean>()
    let frame = 0
    const poll = () => {
      const pads = typeof navigator.getGamepads === 'function' ? navigator.getGamepads() : []
      const pad = Array.from(pads).find((p) => p !== null && p !== undefined)
      if (pad) {
        for (const [button, action] of buttonActions) {
          const down = pad.buttons[button]?.pressed === true
          if (down && !previous.get(button)) {
            press(action)
          }
          previous.set(button, down)
        }
        // The left stick works like the directional pad, past a dead zone.
        const ax = pad.axes[0] ?? 0
        const ay = pad.axes[1] ?? 0
        const stick = Math.abs(ax) > Math.abs(ay) ? (ax > 0.6 ? 'right' : ax < -0.6 ? 'left' : null) : ay > 0.6 ? 'down' : ay < -0.6 ? 'up' : null
        if (stick && !previous.get(-1)) {
          press(stick)
        }
        previous.set(-1, stick !== null)
      }
      frame = requestAnimationFrame(poll)
    }

    window.addEventListener('keydown', onKey)
    frame = requestAnimationFrame(poll)
    focusAt(0)
    return () => {
      window.removeEventListener('keydown', onKey)
      cancelAnimationFrame(frame)
    }
  }, [enabled])
}
