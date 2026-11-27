// Capacity of the Desktop theme: how many columns and whole rows of cards fit the window [TDK-007, TDK-034].
import placeholder from '../assets/cover-placeholder.svg'
import { GRID_INSET, LIST_ROW_HEIGHT, columnsFor, usableWidth } from '../lib/gridNav'
import type { LibraryView } from './viewModel'

export { columnsFor, usableWidth, GRID_GAP, GRID_INSET, CARD_MIN_WIDTH, CARD_TITLE_HEIGHT, SCROLLBAR_SIZE, LIST_ROW_HEIGHT } from '../lib/gridNav'

export const DESKTOP_MAX_NODES = 200
export const HEADER_PX = 65
export const CENTER_PAD_X = 24
export const CENTER_PAD_Y = 16
export const TOOLBAR_PX = 32
export const TOOLBAR_GAP = 12
export const FOOTER_PX = 48
export const FOOTER_GAP = 12

export const coverPlaceholderUrl: string = placeholder

// cssClamp mirrors the CSS clamp(min, preferred, max) of --col-left and --col-right [UIX-005].
const cssClamp = (min: number, preferred: number, max: number) => Math.min(Math.max(preferred, min), max)

// desktopGrid is the columns x whole rows the center of the Desktop theme shows for a window and a view.
export function desktopGrid(vp: { width: number; height: number }, view: LibraryView): { columns: number; rows: number } {
  if (!(vp.width > 0) || !(vp.height > 0)) return view === 'list' ? { columns: 1, rows: 8 } : { columns: 4, rows: 2 }
  const left = cssClamp(220, 0.16 * vp.width, 300)
  const right = cssClamp(320, 0.26 * vp.width, 480)
  const content = vp.width - left - right - 2 * CENTER_PAD_X
  const avail = vp.height - HEADER_PX - 2 * CENTER_PAD_Y - TOOLBAR_PX - TOOLBAR_GAP - FOOTER_PX - FOOTER_GAP
  const inner = avail - 2 * GRID_INSET
  if (view === 'list') return { columns: 1, rows: Math.max(1, Math.floor(inner / LIST_ROW_HEIGHT)) }
  const { cols, rowHeight } = columnsFor(usableWidth(content))
  return { columns: cols, rows: Math.max(1, Math.floor(inner / rowHeight)) }
}
