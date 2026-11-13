import { useEffect, useRef } from 'react'
import { WindowFullscreen, WindowIsFullscreen, WindowUnfullscreen } from '../../wailsjs/runtime/runtime'

// Big Box (phase 8, BIG-001..BIG-004): the library in full screen, driven by the gamepad or the keyboard.

export type Direction = 'up' | 'down' | 'left' | 'right'

// Box is the position of a focusable item on the screen, in pixels.
export type Box = { x: number; y: number; w: number; h: number }

// pickNext returns the index of the item to focus when the focus moves in a direction from items[current]
// (BIG-002). It takes the nearest item in that direction, preferring the same row or column. When no item
// lies that way, the focus stays. It is used on the pages that have no cards.
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
    const dx = it.x + it.w / 2 - cx
    const dy = it.y + it.h / 2 - cy
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

// Card is a game card of the library: its focusable items in order (title, Jogar, Sem contar tempo).
export type Card = { items: HTMLElement[] }

// cardGrid arranges the cards of the page in rows and columns, by where they are on the screen. The
// rows are the cards with the same top edge (within 40 px); the columns follow the left edge. It returns
// null when the page has no cards.
export function cardGrid(items: HTMLElement[]): Card[][] | null {
  const articles: HTMLElement[] = []
  for (const it of items) {
    const card = it.closest<HTMLElement>('article')
    if (card && !articles.includes(card)) {
      articles.push(card)
    }
  }
  if (articles.length === 0) {
    return null
  }
  const placed = articles
    .map((el) => {
      const r = el.getBoundingClientRect()
      return { el, top: r.top, left: r.left }
    })
    .sort((a, b) => a.top - b.top || a.left - b.left)
  const rows: Array<Array<{ el: HTMLElement; top: number; left: number }>> = []
  for (const p of placed) {
    const last = rows[rows.length - 1]
    if (last && Math.abs(last[0].top - p.top) <= 40) {
      last.push(p)
    } else {
      rows.push([p])
    }
  }
  return rows.map((row) =>
    row
      .sort((a, b) => a.left - b.left)
      .map((p) => ({ items: items.filter((it) => p.el.contains(it)) })),
  )
}

// gridNext moves the focus between the cards: left and right go to the neighbouring card, keeping the
// same item of the card; up and down go through the items of a card and then to the next or previous
// row. It stays where it is when there is no card that way (BIG-002).
export function gridNext(grid: Card[][], current: HTMLElement, dir: Direction): HTMLElement {
  let row = -1
  let col = -1
  let slot = -1
  grid.forEach((cards, r) =>
    cards.forEach((card, c) => {
      const s = card.items.indexOf(current)
      if (s >= 0) {
        row = r
        col = c
        slot = s
      }
    }),
  )
  if (row < 0) {
    return current
  }
  const cards = grid[row]
  const card = cards[col]
  switch (dir) {
    case 'left':
    case 'right': {
      const next = col + (dir === 'left' ? -1 : 1)
      if (next < 0 || next >= cards.length) {
        return current
      }
      const target = cards[next]
      return target.items[Math.min(slot, target.items.length - 1)]
    }
    case 'down': {
      if (slot < card.items.length - 1) {
        return card.items[slot + 1]
      }
      if (row + 1 >= grid.length) {
        return current
      }
      const below = grid[row + 1]
      return below[Math.min(col, below.length - 1)].items[0]
    }
    case 'up': {
      if (slot > 0) {
        return card.items[slot - 1]
      }
      if (row - 1 < 0) {
        return current
      }
      const above = grid[row - 1]
      const target = above[Math.min(col, above.length - 1)]
      return target.items[target.items.length - 1]
    }
  }
}

// enterBigBox puts the window in full screen (BIG-001). It asks the window first, so a window that is
// already full screen is not toggled back. Outside the desktop shell (tests, browser) it does nothing.
export async function enterBigBox(): Promise<void> {
  try {
    if (!(await WindowIsFullscreen())) {
      WindowFullscreen()
    }
  } catch {
    // no window runtime: nothing to change
  }
}

// exitBigBox leaves full screen (BIG-001), only when the window is in full screen.
export async function exitBigBox(): Promise<void> {
  try {
    if (await WindowIsFullscreen()) {
      WindowUnfullscreen()
    }
  } catch {
    // no window runtime: nothing to change
  }
}

// bigBoxItems returns the focusable items of the current page, in reading order.
export function bigBoxItems(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>('[data-bigbox-item]'))
}

// focusedIndex is the index of the item that has the focus, or -1.
function focusedIndex(items: HTMLElement[]): number {
  return items.indexOf(document.activeElement as HTMLElement)
}

// playButtonOf finds the "Jogar" button of the card that holds the item, if any (BIG-002).
function playButtonOf(item: HTMLElement | undefined): HTMLElement | null {
  return item?.closest('article')?.querySelector<HTMLElement>('[data-bigbox-play]') ?? null
}

// nextFocus is the item to focus when the focus moves in a direction from the focused item. On the cards
// the card grid decides; on the other pages the geometry does.
export function nextFocus(items: HTMLElement[], focused: HTMLElement | null, dir: Direction): HTMLElement {
  const current = focused && items.includes(focused) ? items.indexOf(focused) : 0
  const grid = cardGrid(items)
  if (grid && focused && items.includes(focused) && focused.closest('article')) {
    return gridNext(grid, focused, dir)
  }
  const boxes: Box[] = items.map((el) => {
    const r = el.getBoundingClientRect()
    return { x: r.left, y: r.top, w: r.width, h: r.height }
  })
  return items[pickNext(boxes, current, dir)]
}

// useBigBoxNav drives the Big Box with the keyboard and the gamepad (BIG-002, BIG-003). The directions
// move the focus between the items of the page; A (or Enter) activates the focused item; Y (or Shift+Enter)
// starts the game of the focused card; B (or Esc) goes back one page, and on the library it leaves the
// Big Box. Gamepad buttons follow the standard mapping of the Gamepad API. Each page (pageKey) starts with
// the focus on its first item.
export function useBigBoxNav({ enabled, pageKey, onBack }: { enabled: boolean; pageKey: string; onBack: () => void }): void {
  const backRef = useRef(onBack)
  backRef.current = onBack

  // A new page: the focus goes to its first item, once the page is in the DOM.
  useEffect(() => {
    if (enabled) {
      bigBoxItems()[0]?.focus()
      bigBoxItems()[0]?.scrollIntoView?.({ block: 'nearest' })
    }
  }, [enabled, pageKey])

  useEffect(() => {
    if (!enabled) {
      return undefined
    }
    const move = (dir: Direction) => {
      const items = bigBoxItems()
      if (items.length === 0) {
        return
      }
      const focused = document.activeElement as HTMLElement | null
      const next = nextFocus(items, focused, dir)
      next.focus()
      next.scrollIntoView?.({ block: 'nearest' })
    }
    const activate = () => {
      const focused = document.activeElement as HTMLElement | null
      if (focused && bigBoxItems().includes(focused)) {
        focused.click()
      }
    }
    const play = () => {
      const items = bigBoxItems()
      const i = focusedIndex(items)
      playButtonOf(i >= 0 ? items[i] : undefined)?.click()
    }

    const onKey = (e: KeyboardEvent) => {
      switch (e.key) {
        case 'ArrowUp':
          move('up')
          break
        case 'ArrowDown':
          move('down')
          break
        case 'ArrowLeft':
          move('left')
          break
        case 'ArrowRight':
          move('right')
          break
        case 'Escape':
          backRef.current()
          break
        case 'Enter':
          // Enter on the focused item is the browser's own activation; Shift+Enter starts the game.
          if (!e.shiftKey) {
            return
          }
          play()
          break
        default:
          return
      }
      e.preventDefault()
    }

    // Standard gamepad mapping: 0 = A, 1 = B, 3 = Y, 12..15 = the directional pad.
    const buttonActions: Array<[number, () => void]> = [
      [12, () => move('up')],
      [13, () => move('down')],
      [14, () => move('left')],
      [15, () => move('right')],
      [0, activate],
      [3, play],
      [1, () => backRef.current()],
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
            action()
          }
          previous.set(button, down)
        }
        // The left stick works like the directional pad, past a dead zone.
        const ax = pad.axes[0] ?? 0
        const ay = pad.axes[1] ?? 0
        const stick = Math.abs(ax) > Math.abs(ay) ? (ax > 0.6 ? 'right' : ax < -0.6 ? 'left' : null) : ay > 0.6 ? 'down' : ay < -0.6 ? 'up' : null
        if (stick && !previous.get(-1)) {
          move(stick)
        }
        previous.set(-1, stick !== null)
      }
      frame = requestAnimationFrame(poll)
    }

    window.addEventListener('keydown', onKey)
    frame = requestAnimationFrame(poll)
    return () => {
      window.removeEventListener('keydown', onKey)
      cancelAnimationFrame(frame)
    }
  }, [enabled])
}
