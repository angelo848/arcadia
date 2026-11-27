import type { ComponentType, ReactNode } from 'react'
import type { BigBoxDetailsProps } from '../BigBoxDetails'
import type { BigBoxThemeId, BigBoxViewModel, LibraryView } from '../viewModel'

export type { LibraryView }

type Viewport = { width: number; height: number }

// Theme contract v2 [BBX-024, BBX-047, BBX-061]: a theme only draws the model and forwards clicks.
export type BigBoxThemeProps = {
  model: BigBoxViewModel
  details: ReactNode
  onPick: (index: number) => void
  onPlay: (index: number) => void
  onDetails: () => void
  onVideoError: (gameId: number) => void
  onTab?: (index: number) => void // BBX-065
}

export type BigBoxThemeDef = {
  id: BigBoxThemeId
  name: string
  available: boolean
  axis: 'horizontal' | 'vertical' | 'grid'
  columns: number
  anchor: number
  listCapacity: (viewport: Viewport) => number
  gridRows?: (viewport: Viewport, view?: LibraryView) => number // existing themes ignore `view`
  backdrop: 'contain' | 'cover' | 'plain'
  videoSlot: 'background' | 'cover' | 'panel'
  tabs: 'platforms' | 'collections'
  sort: 'title' | 'lastPlayed' | 'inherit'
  timeToBeatInBrowse: boolean
  layout?: 'screens' | 'columns'
  gridColumns?: (viewport: Viewport, view: LibraryView) => number
  maxNodes?: number
  gridScroll?: 'focusTop' | 'edges'
  Component?: ComponentType<BigBoxThemeProps>
  Details?: ComponentType<BigBoxDetailsProps>
}
