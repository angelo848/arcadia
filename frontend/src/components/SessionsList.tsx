import type { JSX } from 'react'
import { useGameSessions } from '../lib/library'
import type { SessionItemView } from '../lib/library'
import { formatPlayTime, useToggleSession } from '../lib/games'
import type { SessionState, SessionView } from '../lib/games'
import { formatLocalDateTime } from '../lib/formatDate'
import { SessionPanel } from './SessionPanel'

const stateLabels: Record<SessionState, string> = {
  running: 'Em andamento',
  paused: 'Em andamento',
  grace: 'Em andamento',
  ended: 'Encerrada',
  discarded: 'Descartada',
}

const stateClasses: Record<SessionState, string> = {
  running: 'font-medium text-emerald-400',
  paused: 'font-medium text-emerald-400',
  grace: 'font-medium text-emerald-400',
  ended: 'text-neutral-400',
  discarded: 'text-amber-400',
}

// A session without active time is shown as 0 min; formatPlayTime(0) reads "Nunca jogado".
function durationText(activeSeconds: number): string {
  return activeSeconds > 0 ? formatPlayTime(activeSeconds) : '0 min'
}

// SessionsList shows the sessions of one game, 50 per page [UI-038]. The open session of
// this game is shown with the phase 4 SessionPanel above the list [UI-012, UI-014]. A
// discarded session shows its own active time but is never summed anywhere [UI-038].
export function SessionsList({
  gameId,
  current,
}: {
  gameId: number
  current: SessionView | null
}): JSX.Element {
  const q = useGameSessions(gameId)
  const toggle = useToggleSession()
  const sessions: SessionItemView[] = q.data?.pages.flatMap((p) => p.sessions) ?? []

  return (
    <section className="flex flex-col gap-2">
      {current !== null && current.gameId === gameId && (
        <SessionPanel session={current} onToggle={() => toggle.mutate()} pending={toggle.isPending} />
      )}

      {q.isError ? (
        <p role="alert" className="text-sm text-red-400">
          {q.error?.message}
        </p>
      ) : q.isPending ? (
        <p className="text-sm text-neutral-400">Carregando sessões…</p>
      ) : sessions.length === 0 ? (
        <p className="text-sm text-neutral-400">Nenhuma sessão registrada.</p>
      ) : (
        <ul className="flex flex-col">
          {sessions.map((s) => (
            <li
              key={s.sessionId}
              className="flex items-center justify-between gap-4 border-b border-neutral-800 py-2 text-sm"
            >
              <span className="text-neutral-200">{formatLocalDateTime(s.startedAt)}</span>
              <span className="text-neutral-400">{s.endedAt ? formatLocalDateTime(s.endedAt) : '—'}</span>
              <span className="tabular-nums text-neutral-200">{durationText(s.activeSeconds)}</span>
              <span className={stateClasses[s.state]}>{stateLabels[s.state]}</span>
            </li>
          ))}
        </ul>
      )}

      {q.hasNextPage && (
        <button
          type="button"
          onClick={() => void q.fetchNextPage()}
          disabled={q.isFetchingNextPage}
          className="self-center rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700 disabled:cursor-not-allowed disabled:text-neutral-500"
        >
          Carregar mais
        </button>
      )}
    </section>
  )
}
