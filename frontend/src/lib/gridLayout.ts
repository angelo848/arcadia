import type { NavKey } from './gridNav'

// Layout of the desktop grid: rows of cards, optionally split by "last played" year dividers.

export const DIVIDER_HEIGHT = 48

export type GridRow = { kind: 'divider'; label: string } | { kind: 'cards'; first: number; count: number }

export type GridLayout = {
  rows: GridRow[]
  // tops[i] is the offset of row i from the top of the content (insets not included).
  tops: number[]
  // rowOf[i] is the row holding game i.
  rowOf: number[]
  height: number
}

// lastPlayedGroup is the divider label of a game: the local year it was last played, or "Nunca jogado".
export function lastPlayedGroup(lastPlayedAt: string): string {
  if (lastPlayedAt === '') {
    return 'Nunca jogado'
  }
  const d = new Date(lastPlayedAt)
  return Number.isNaN(d.getTime()) ? 'Nunca jogado' : String(d.getFullYear())
}

// buildLayout cuts the games into rows of cols cards. With groupOf, a divider row opens every run of games
// with the same group, and a new group always starts a new row.
export function buildLayout<T>(
  games: readonly T[],
  cols: number,
  rowHeight: number,
  groupOf?: (g: T) => string,
): GridLayout {
  const c = Math.max(1, cols)
  const rows: GridRow[] = []
  const tops: number[] = []
  const rowOf: number[] = []
  let y = 0
  const push = (row: GridRow) => {
    rows.push(row)
    tops.push(y)
    y += row.kind === 'divider' ? DIVIDER_HEIGHT : rowHeight
  }
  let i = 0
  while (i < games.length) {
    let end = games.length
    if (groupOf !== undefined) {
      const label = groupOf(games[i] as T)
      push({ kind: 'divider', label })
      end = i + 1
      while (end < games.length && groupOf(games[end] as T) === label) {
        end++
      }
    }
    for (let first = i; first < end; first += c) {
      const count = Math.min(c, end - first)
      for (let k = 0; k < count; k++) {
        rowOf.push(rows.length)
      }
      push({ kind: 'cards', first, count })
    }
    i = end
  }
  return { rows, tops, rowOf, height: y }
}

function cardRow(layout: GridLayout, from: number, step: 1 | -1): GridRow | undefined {
  for (let r = from + step; r >= 0 && r < layout.rows.length; r += step) {
    const row = layout.rows[r]
    if (row?.kind === 'cards') {
      return row
    }
  }
  return undefined
}

// moveInLayout returns the game the focus moves to; edges keep the index [UIX-019]. Up and down keep the
// column, clamped to the length of a shorter row.
export function moveInLayout(layout: GridLayout, index: number, key: NavKey): number {
  const last = layout.rowOf.length - 1
  const r = layout.rowOf[index]
  const row = r === undefined ? undefined : layout.rows[r]
  if (r === undefined || row?.kind !== 'cards') {
    return index
  }
  const col = index - row.first
  switch (key) {
    case 'Home':
      return 0
    case 'End':
      return Math.max(0, last)
    case 'ArrowLeft':
      return col === 0 ? index : index - 1
    case 'ArrowRight':
      return col === row.count - 1 ? index : index + 1
    case 'ArrowUp':
    case 'ArrowDown': {
      const next = cardRow(layout, r, key === 'ArrowDown' ? 1 : -1)
      if (next?.kind !== 'cards') {
        return index
      }
      return next.first + Math.min(col, next.count - 1)
    }
  }
}
