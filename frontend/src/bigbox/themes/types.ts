import type { ReactNode } from 'react'
import type { BigBoxViewModel } from '../viewModel'

// Theme contract [BBX-024]: a theme only draws the model and forwards clicks.
export type BigBoxThemeProps = {
  model: BigBoxViewModel
  details: ReactNode
  onPick: (index: number) => void
  onPlay: (index: number) => void
  onDetails: () => void
}
