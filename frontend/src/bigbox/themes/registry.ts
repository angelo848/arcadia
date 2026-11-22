import { Faixa } from './faixa/Faixa'
import { faixaCapacity } from './faixa/capacity'
import { STRIP_FALLBACK_CAPACITY } from '../viewModel'
import type { BigBoxThemeDef, BigBoxThemeProps } from './types'

export type { BigBoxThemeDef, BigBoxThemeProps }

// Mutable on purpose: tests register a fake theme [BBX-024]. Mosaico and Estante join in T34 and T36.
export const BIGBOX_THEMES: BigBoxThemeDef[] = [
  {
    id: 'faixa', name: 'Faixa', available: true, axis: 'horizontal', columns: 1, anchor: 2, listCapacity: faixaCapacity,
    backdrop: 'contain', videoSlot: 'background', tabs: 'platforms', sort: 'title', timeToBeatInBrowse: false, Component: Faixa,
  },
  {
    id: 'mosaico', name: 'Mosaico (preparado)', available: false, axis: 'horizontal', columns: 1, anchor: 1,
    listCapacity: () => STRIP_FALLBACK_CAPACITY,
    backdrop: 'plain', videoSlot: 'cover', tabs: 'platforms', sort: 'title', timeToBeatInBrowse: true,
  },
  {
    id: 'roda', name: 'Roda (preparado)', available: false, axis: 'vertical', columns: 1, anchor: 2,
    listCapacity: () => STRIP_FALLBACK_CAPACITY,
    backdrop: 'contain', videoSlot: 'background', tabs: 'platforms', sort: 'title', timeToBeatInBrowse: false,
  },
]

// resolveTheme reads the registry on every call; unknown or unavailable ids fall back to faixa [BBX-023].
export function resolveTheme(id: string | null | undefined): BigBoxThemeDef {
  const found = BIGBOX_THEMES.find((t) => t.id === id && t.available)
  return found ?? BIGBOX_THEMES[0]
}
