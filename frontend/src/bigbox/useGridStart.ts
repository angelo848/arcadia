// First visible row of a grid theme [BBX-049, BBX-063]: `focusTop` keeps the focus row first, `edges` scrolls only at the borders.
import { useCallback, useRef } from 'react'
import { centeredRow, edgeScrollRow } from './viewModel'

export type GridStartInput = {
  mode: 'focusTop' | 'edges'
  focus: number
  total: number
  columns: number
  rows: number // R': whole rows shown
  tabKey: string
}

export function useGridStart(i: GridStartInput): { startRow: number; recenter: () => void } {
  const start = useRef(0)
  const lastTab = useRef<string | null>(null)
  const center = useRef(false)

  const c = Math.max(i.columns, 1)
  const r = Math.max(i.rows, 1)
  const row = Math.floor(Math.max(i.focus, 0) / c)
  const totalRows = Math.ceil(Math.max(i.total, 0) / c)

  let s: number
  if (i.mode === 'focusTop') {
    s = Math.min(Math.max(row, 0), Math.max(totalRows - r, 0))
    center.current = false
  } else if (center.current) {
    s = centeredRow(row, r, totalRows)
    center.current = false
  } else if (lastTab.current === null || lastTab.current !== i.tabKey) {
    // Entry and tab change start from the top: a focus already inside the first whole rows keeps it there.
    s = row < r ? edgeScrollRow(0, row, r, totalRows) : centeredRow(row, r, totalRows)
  } else {
    s = edgeScrollRow(start.current, row, r, totalRows)
  }
  lastTab.current = i.tabKey
  start.current = s

  // The next calculation puts the focus row in the middle (entry, closed game).
  const recenter = useCallback(() => {
    center.current = true
  }, [])
  return { startRow: s, recenter }
}
