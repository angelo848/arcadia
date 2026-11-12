import type { JSX } from 'react'
import { formatPlayTime, type GameView, type SessionView } from '../lib/games'

// PendingLaunch is the launch whose call is in flight: the game and the mode it
// was started with. Both buttons of that game stay disabled until it ends.
export type PendingLaunch = { id: number; mode: 'tracked' | 'untracked' } | null

// liveLabel is the marking of the game of the current session [UI-006, UI-024]:
// running and grace show "Em execução", paused shows "Pausado", and ended or
// discarded (or another game) show nothing.
function liveLabel(current: SessionView | null, gameId: number): string | null {
  if (current === null || current.gameId !== gameId) {
    return null
  }
  switch (current.state) {
    case 'running':
    case 'grace':
      return 'Em execução'
    case 'paused':
      return 'Pausado'
    default:
      return null
  }
}

// GameList renders one row per game in the received order, with the title, the
// total play time and two launch buttons: "Jogar" (tracked) and "Sem contar
// tempo" [UI-004, UI-019]. The row of the current session's game shows "Em
// execução" or "Pausado" [UI-006, UI-024]. While a launch of a game is pending,
// both of its buttons are disabled and the one that was triggered shows
// "Iniciando…" [UI-008, UI-021].
export function GameList({
  games,
  current,
  pending,
  onPlay,
  onPlayUntracked,
}: {
  games: GameView[]
  current: SessionView | null
  pending: PendingLaunch
  onPlay: (id: number) => void
  onPlayUntracked: (id: number) => void
}): JSX.Element {
  const buttonClass =
    'shrink-0 rounded px-4 py-1.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400'

  return (
    <ul className="divide-y divide-neutral-800">
      {games.map((g) => {
        const busy = pending !== null && pending.id === g.id
        const label = liveLabel(current, g.id)
        const trackedPending = busy && pending?.mode === 'tracked'
        const untrackedPending = busy && pending?.mode === 'untracked'
        return (
          <li key={g.id} className="flex items-center justify-between gap-4 px-6 py-3">
            <div className="flex min-w-0 flex-col">
              <span className="truncate font-medium text-neutral-100">{g.title}</span>
              <span className="flex items-center gap-2 text-sm text-neutral-400">
                <span>{formatPlayTime(g.totalPlaySeconds)}</span>
                {label !== null && (
                  <span className={label === 'Pausado' ? 'font-medium text-sky-400' : 'font-medium text-emerald-400'}>
                    {label}
                  </span>
                )}
              </span>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <button
                type="button"
                disabled={busy}
                onClick={() => onPlay(g.id)}
                className={`${buttonClass} bg-emerald-600 hover:bg-emerald-500`}
              >
                {trackedPending ? 'Iniciando…' : 'Jogar'}
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => onPlayUntracked(g.id)}
                className={`${buttonClass} bg-neutral-800 text-neutral-100 hover:bg-neutral-700`}
              >
                {untrackedPending ? 'Iniciando…' : 'Sem contar tempo'}
              </button>
            </div>
          </li>
        )
      })}
    </ul>
  )
}
