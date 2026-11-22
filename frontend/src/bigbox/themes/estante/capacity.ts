// Grid capacity of the Estante theme [ETN-008]: whole rows of 2:3 cards that fit below the hero (temas-v2 section 6.2).
export const ESTANTE_COLUMNS = 5

const GAP_PX = 24
const SAFE_RATIO = 0.9 // safe area: 90% of the window
const TEXT_PX = 56 // title and play time under the cover
const HEADER_RATIO = 0.16 // header with tabs
const FOOTER_RATIO = 0.07 // footer hints
const UNKNOWN_ROWS = 2
const MAX_ROWS = 5

// estanteRows is pure: no window access (the shell passes the viewport).
export function estanteRows(vp: { width: number; height: number }): number {
  if (!(vp.width > 0) || !(vp.height > 0)) return UNKNOWN_ROWS
  const w = (SAFE_RATIO * vp.width - (ESTANTE_COLUMNS - 1) * GAP_PX) / ESTANTE_COLUMNS
  const h = 1.5 * w + TEXT_PX
  const area = (SAFE_RATIO - HEADER_RATIO - FOOTER_RATIO) * vp.height
  const rows = Math.floor((area + GAP_PX) / (h + GAP_PX))
  return Math.min(Math.max(rows, 1), MAX_ROWS)
}
