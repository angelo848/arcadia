// Strip capacity of the Faixa theme [BBX-024, BBX-039]: whole covers that fit in the safe area.
import { STRIP_FALLBACK_CAPACITY, STRIP_MAX } from '../../viewModel'

const SAFE_WIDTH_RATIO = 0.9 // 5vw of safe area on each side
const COVER_HEIGHT_RATIO = 0.18 // h-[18vh]
const COVER_GAP_PX = 16
const STRIP_INSET_PX = 8 // lateral inset of the strip inside the safe area
const FOCUS_SCALE_EXTRA = 0.25 // the focused cover is drawn at 1.25x

// faixaCapacity is pure: no window access (the shell passes the viewport).
export function faixaCapacity(vp: { width: number; height: number }): number {
  if (!(vp.width > 0) || !(vp.height > 0)) return STRIP_FALLBACK_CAPACITY
  const usable = vp.width * SAFE_WIDTH_RATIO - STRIP_INSET_PX
  const cover = COVER_HEIGHT_RATIO * vp.height * (2 / 3)
  const v = Math.floor((usable - FOCUS_SCALE_EXTRA * cover + COVER_GAP_PX) / (cover + COVER_GAP_PX))
  return Math.min(Math.max(v, 1), STRIP_MAX - 1)
}
