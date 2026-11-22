import type { JSX } from 'react'
import type { BigBoxGame } from './viewModel'

// CoverArt draws the cover of a game, or its title when there is no image [BBX-006, BBX-039].
export function CoverArt({ game }: { game: BigBoxGame }): JSX.Element {
  return game.coverUrl ? (
    <img src={game.coverUrl} alt="" className="h-full w-full object-contain" />
  ) : (
    <span className="px-2 text-sm">{game.title}</span>
  )
}
