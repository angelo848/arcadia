import type { JSX } from 'react'
import type { SessionView } from '../lib/games'
import type { GameView } from '../lib/library'
import type { CardImage } from '../lib/metadata'
import { CoverImage } from './CoverImage'

// LiveMark is the session marking of a card [UI-006, UI-043]: "running" (running or
// grace) shows "Em execução", "paused" shows "Pausado", and null shows nothing.
export type LiveMark = 'running' | 'paused' | null

// liveOf maps the current session to the marking of one game [UI-006, UI-043].
// Ended and discarded sessions, and sessions of other games, mark nothing.
export function liveOf(current: SessionView | null, gameId: number): LiveMark {
  if (current === null || current.gameId !== gameId) {
    return null
  }
  switch (current.state) {
    case 'running':
    case 'grace':
      return 'running'
    case 'paused':
      return 'paused'
    default:
      return null
  }
}

// GameCard shows one game of the grid: the cover (2:3), the title in up to two lines and the
// session mark over the cover [UIX-014, UIX-015]. One click puts the game in focus, a double
// click launches it [UIX-017, UIX-018]. The card has no buttons.
export function GameCard({
  game,
  image,
  live,
  selected,
  tabbable,
  onSelect,
  onLaunch,
}: {
  game: GameView
  // image is the card image of the game, when it has one [AJU-015].
  image?: CardImage
  live: LiveMark
  selected: boolean
  tabbable: boolean
  onSelect: (id: number) => void
  onLaunch: (id: number) => void
}): JSX.Element {
  return (
    <div
      role="option"
      aria-selected={selected}
      data-game-id={game.id}
      data-bigbox-item
      data-bigbox-card
      tabIndex={tabbable ? 0 : -1}
      onClick={() => onSelect(game.id)}
      onDoubleClick={() => onLaunch(game.id)}
      className={`relative flex min-w-0 cursor-pointer flex-col gap-1 rounded p-1 ${
        selected ? 'bg-raised ring-2 ring-accent' : 'hover:bg-hover'
      }`}
    >
      <CoverImage src={image?.url} fit={image?.fit} />
      <span title={game.title} className="line-clamp-2 text-sm font-medium text-fg">
        {game.title}
      </span>
      {' '}
      {live === 'running' && (
        <span className="absolute left-2 top-2 rounded bg-overlay px-1.5 py-0.5 text-xs font-medium text-running">
          Em execução
        </span>
      )}
      {live === 'paused' && (
        <span className="absolute left-2 top-2 rounded bg-overlay px-1.5 py-0.5 text-xs font-medium text-paused">
          Pausado
        </span>
      )}
    </div>
  )
}
