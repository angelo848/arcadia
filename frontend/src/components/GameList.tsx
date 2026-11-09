import type { JSX } from 'react'
import { formatPlayTime, type GameView, type SessionView } from '../lib/games'

function isLive(s: SessionView | null): s is SessionView {
  return s !== null && (s.state === 'running' || s.state === 'grace')
}

// GameList renders one row per game in the received order, with the title, the
// total play time and the "Jogar" button [UI-004]. The row of the current
// session's game shows "Em execução" [UI-006]; the button of a pending launch
// shows "Iniciando…" and is disabled [UI-008].
export function GameList({
  games,
  current,
  pendingId,
  onPlay,
}: {
  games: GameView[]
  current: SessionView | null
  pendingId: number | null
  onPlay: (id: number) => void
}): JSX.Element {
  const runningId = isLive(current) ? current.gameId : null

  return (
    <ul className="divide-y divide-neutral-800">
      {games.map((g) => {
        const pending = pendingId === g.id
        return (
          <li key={g.id} className="flex items-center justify-between gap-4 px-6 py-3">
            <div className="flex min-w-0 flex-col">
              <span className="truncate font-medium text-neutral-100">{g.title}</span>
              <span className="flex items-center gap-2 text-sm text-neutral-400">
                <span>{formatPlayTime(g.totalPlaySeconds)}</span>
                {runningId === g.id && <span className="font-medium text-emerald-400">Em execução</span>}
              </span>
            </div>
            <button
              type="button"
              disabled={pending}
              onClick={() => onPlay(g.id)}
              className="shrink-0 rounded bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400"
            >
              {pending ? 'Iniciando…' : 'Jogar'}
            </button>
          </li>
        )
      })}
    </ul>
  )
}
