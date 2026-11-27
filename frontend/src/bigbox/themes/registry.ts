import { Faixa } from './faixa/Faixa'
import { faixaCapacity } from './faixa/capacity'
import { Mosaico } from './mosaico/Mosaico'
import { mosaicoCapacity } from './mosaico/capacity'
import { ESTANTE_COLUMNS, estanteRows } from './estante/capacity'
import { Estante } from './estante/Estante'
import { EstanteDetails } from './estante/EstanteDetails'
import { Desktop } from './desktop/Desktop'
import { DesktopDetails } from './desktop/DesktopDetails'
import { DESKTOP_MAX_NODES, desktopGrid } from '../desktopLayout'
import { STRIP_FALLBACK_CAPACITY } from '../viewModel'
import type { BigBoxThemeDef, BigBoxThemeProps } from './types'

export type { BigBoxThemeDef, BigBoxThemeProps }

// Mutable on purpose: tests register a fake theme [BBX-024].
export const BIGBOX_THEMES: BigBoxThemeDef[] = [
  {
    id: 'faixa', name: 'Faixa', available: true, axis: 'horizontal', columns: 1, anchor: 2, listCapacity: faixaCapacity,
    backdrop: 'contain', videoSlot: 'background', tabs: 'platforms', sort: 'title', timeToBeatInBrowse: false, Component: Faixa,
  },
  {
    id: 'mosaico', name: 'Mosaico', available: true, axis: 'horizontal', columns: 1, anchor: 1,
    listCapacity: mosaicoCapacity,
    backdrop: 'plain', videoSlot: 'cover', tabs: 'platforms', sort: 'title', timeToBeatInBrowse: true, Component: Mosaico,
  },
  {
    id: 'estante', name: 'Estante', available: true, axis: 'grid', columns: 5, anchor: 0,
    listCapacity: (vp) => ESTANTE_COLUMNS * estanteRows(vp), gridRows: estanteRows,
    backdrop: 'cover', videoSlot: 'background', tabs: 'collections', sort: 'lastPlayed', timeToBeatInBrowse: false,
    Component: Estante, Details: EstanteDetails,
  },
  {
    id: 'desktop', name: 'Desktop', available: true, axis: 'grid', columns: 1, anchor: 0, layout: 'columns',
    gridColumns: (vp, view) => desktopGrid(vp, view).columns, gridRows: (vp, view) => desktopGrid(vp, view ?? 'grid').rows,
    listCapacity: (vp) => {
      const g = desktopGrid(vp, 'grid')
      return g.columns * g.rows
    },
    maxNodes: DESKTOP_MAX_NODES, gridScroll: 'edges', backdrop: 'plain', videoSlot: 'panel', tabs: 'platforms',
    sort: 'inherit', timeToBeatInBrowse: true, Component: Desktop, Details: DesktopDetails,
  },
  {
    id: 'roda', name: 'Roda', available: false, axis: 'vertical', columns: 1, anchor: 2,
    listCapacity: () => STRIP_FALLBACK_CAPACITY,
    backdrop: 'contain', videoSlot: 'background', tabs: 'platforms', sort: 'title', timeToBeatInBrowse: false,
  },
]

// resolveTheme reads the registry on every call; unknown or unavailable ids fall back to faixa [BBX-023].
export function resolveTheme(id: string | null | undefined): BigBoxThemeDef {
  const found = BIGBOX_THEMES.find((t) => t.id === id && t.available)
  return found ?? BIGBOX_THEMES[0]
}
