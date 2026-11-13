import type { JSX } from 'react'
import { formatPlayTime, type SessionView } from '../lib/games'
import type { GameView } from '../lib/library'

// LiveMark is the session marking of a card [UI-006, UI-043]: "running" (running or
// grace) shows "Em execução", "paused" shows "Pausado", and null shows nothing.
export type LiveMark = 'running' | 'paused' | null

// BusyMode is the launch of this game that is pending, if any [UI-021, UI-031].
export type BusyMode = 'tracked' | 'untracked' | null

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

// GameCard shows one game of the library grid: title, platform and total play
// time [UI-029, UI-005], the session marking [UI-006, UI-043], and the two launch
// buttons "Jogar" and "Sem contar tempo" [UI-031]. While a launch of this game is
// pending, both buttons are disabled and the triggered one shows "Iniciando…"
// [UI-021].
export function GameCard({
  game,
  coverUrl,
  live,
  busy,
  onOpen,
  onPlay,
  onPlayUntracked,
}: {
  game: GameView
  // coverUrl is the cover thumbnail of the game, when it has one (MID-006).
  coverUrl?: string
  live: LiveMark
  busy: BusyMode
  onOpen: (id: number) => void
  onPlay: (id: number) => void
  onPlayUntracked: (id: number) => void
}): JSX.Element {
  const buttonClass =
    'shrink-0 rounded px-4 py-1.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400'
  const blocked = busy !== null

  return (
    <article className="flex h-full flex-col justify-between gap-3 rounded-lg bg-neutral-900 p-4">
      {coverUrl && <img src={coverUrl} alt="" className="h-36 w-full rounded object-cover" />}
      <div
        role="link"
        data-bigbox-item
        tabIndex={0}
        onClick={() => onOpen(game.id)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            onOpen(game.id)
          }
        }}
        className="flex min-w-0 cursor-pointer flex-col"
      >
        <span className="truncate font-medium text-neutral-100">{game.title}</span>
        <span className="truncate text-sm text-neutral-400">{game.platform}</span>
        <span className="flex items-center gap-2 text-sm text-neutral-400">
          <span>{formatPlayTime(game.totalPlaySeconds)}</span>
          {live === 'running' && <span className="font-medium text-emerald-400">Em execução</span>}
          {live === 'paused' && <span className="font-medium text-sky-400">Pausado</span>}
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          disabled={blocked}
          data-bigbox-play
          data-bigbox-item
          onClick={() => onPlay(game.id)}
          className={`${buttonClass} bg-emerald-600 hover:bg-emerald-500`}
        >
          {busy === 'tracked' ? 'Iniciando…' : 'Jogar'}
        </button>
        <button
          type="button"
          disabled={blocked}
          data-bigbox-item
          onClick={() => onPlayUntracked(game.id)}
          className={`${buttonClass} bg-neutral-800 text-neutral-100 hover:bg-neutral-700`}
        >
          {busy === 'untracked' ? 'Iniciando…' : 'Sem contar tempo'}
        </button>
      </div>
    </article>
  )
}
