// Strip capacity of the Mosaico theme [MOS-012]: whole square covers that fit in the safe area.
import { STRIP_FALLBACK_CAPACITY } from '../../viewModel'

const SAFE_WIDTH_RATIO = 0.9
const COVER_HEIGHT_RATIO = 0.3 // square cover side = 30vh
const COVER_GAP_PX = 16
const FOCUS_EXTRA_RATIO = 0.09 // the focused cover is drawn at 1.09x
const FOCUS_EXTRA_PX = 18 // outline (5px) and its offset (4px) on both sides
const MAX = 14

// mosaicoCapacity is pure: no window access (the shell passes the viewport).
export function mosaicoCapacity(vp: { width: number; height: number }): number {
  if (!(vp.width > 0) || !(vp.height > 0)) return STRIP_FALLBACK_CAPACITY
  const w = vp.width * SAFE_WIDTH_RATIO
  const s = COVER_HEIGHT_RATIO * vp.height
  const extra = FOCUS_EXTRA_RATIO * s + FOCUS_EXTRA_PX
  const v = Math.floor((w - extra + COVER_GAP_PX) / (s + COVER_GAP_PX))
  return Math.min(Math.max(v, 1), MAX)
}
