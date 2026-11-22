import type { ComponentType, ReactNode } from 'react'
import type { BigBoxDetailsProps } from '../BigBoxDetails'
import type { BigBoxThemeId, BigBoxViewModel } from '../viewModel'

// Theme contract v2 [BBX-024, BBX-047]: a theme only draws the model and forwards clicks.
export type BigBoxThemeProps = {
  model: BigBoxViewModel
  details: ReactNode
  onPick: (index: number) => void
  onPlay: (index: number) => void
  onDetails: () => void
  onVideoError: (gameId: number) => void
}

export type BigBoxThemeDef = {
  id: BigBoxThemeId
  name: string
  available: boolean
  axis: 'horizontal' | 'vertical' | 'grid'
  columns: number
  anchor: number
  listCapacity: (viewport: { width: number; height: number }) => number
  gridRows?: (viewport: { width: number; height: number }) => number
  backdrop: 'contain' | 'cover' | 'plain'
  videoSlot: 'background' | 'cover'
  tabs: 'platforms' | 'collections'
  sort: 'title' | 'lastPlayed'
  timeToBeatInBrowse: boolean
  Component?: ComponentType<BigBoxThemeProps>
  Details?: ComponentType<BigBoxDetailsProps>
}
