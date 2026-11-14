// Grid measures and keyboard movement [UIX-005, UIX-014, UIX-019, UIX-020].
export const CARD_MIN_WIDTH = 160
export const GRID_GAP = 16
export const CARD_TITLE_HEIGHT = 44 // two title lines under the 2:3 cover

export const GRID_INSET = 6 // room for the focus ring on each side [AJU-001]
export const SCROLLBAR_SIZE = 8 // matches the themed scrollbar [AJU-004]

// usableWidth is the grid width left after both insets and the scrollbar [AJU-003].
export function usableWidth(boxWidth: number): number {
  return Math.max(0, boxWidth - 2 * GRID_INSET - SCROLLBAR_SIZE)
}

export type NavKey = 'ArrowLeft' | 'ArrowRight' | 'ArrowUp' | 'ArrowDown' | 'Home' | 'End'

const navKeys: readonly string[] = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End']

export function isNavKey(key: string): key is NavKey {
  return navKeys.includes(key)
}

// nextIndex returns the index the focus moves to; edges keep the index [UIX-019].
export function nextIndex(index: number, key: NavKey, cols: number, loaded: number): number {
  const last = loaded - 1
  const c = Math.max(1, cols)
  switch (key) {
    case 'Home':
      return 0
    case 'End':
      return Math.max(0, last)
    case 'ArrowLeft':
      return index % c === 0 ? index : index - 1
    case 'ArrowRight':
      return index % c === c - 1 || index >= last ? index : index + 1
    case 'ArrowUp':
      return index - c < 0 ? index : index - c
    case 'ArrowDown': {
      if (index + c <= last) {
        return index + c
      }
      const sameRow = Math.floor(index / c) === Math.floor(last / c)
      return sameRow ? index : last
    }
  }
}

// columnsFor fits as many cards of at least CARD_MIN_WIDTH as the width allows [UIX-005, UIX-014].
export function columnsFor(width: number): { cols: number; cardWidth: number; rowHeight: number } {
  const cols = Math.max(1, Math.floor((width + GRID_GAP) / (CARD_MIN_WIDTH + GRID_GAP)))
  const cardWidth = (width - (cols - 1) * GRID_GAP) / cols
  return { cols, cardWidth, rowHeight: (cardWidth * 3) / 2 + CARD_TITLE_HEIGHT + GRID_GAP }
}
