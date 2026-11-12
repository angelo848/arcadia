import { useMemo, type JSX } from 'react'
import type { SessionView } from '../lib/games'
import { anchorFrom, formatElapsed, useElapsedSeconds } from '../lib/sessionClock'

// SessionPanel shows the current session's game and its live clock [UI-012].
// Each new session value is a new anchor taken at reception [UI-014]; in grace
// the clock is frozen and "Encerrando…" is shown [UI-015]; in paused it is frozen
// and "Pausado" is shown [UI-022]. The one toggle button reads "Pausar" in running
// and "Retomar" in paused, and is disabled in grace and while its call is pending
// [UI-027, UI-028]. The label comes only from session.state. The panel never
// calls a binding itself [UI-017, UI-025]: onToggle is the only action.
export function SessionPanel({
  session,
  onToggle,
  pending,
  now,
}: {
  session: SessionView
  onToggle: () => void
  pending: boolean
  now?: () => number
}): JSX.Element {
  // The anchor depends only on the received value: an unstable `now` prop must
  // not reset the reception instant on every render.
  const anchor = useMemo(() => anchorFrom(session, now), [session])
  const seconds = useElapsedSeconds(anchor, now)
  const grace = session.state === 'grace'
  const paused = session.state === 'paused'

  return (
    <section
      role="status"
      aria-label="Sessão em andamento"
      className="mx-6 my-4 flex items-center justify-between gap-4 rounded border border-emerald-800 bg-emerald-950/40 px-4 py-3"
    >
      <div className="flex min-w-0 flex-col">
        <span className="truncate font-medium text-neutral-100">{session.title}</span>
        {grace && <span className="text-sm text-amber-400">Encerrando…</span>}
        {paused && <span className="text-sm font-medium text-sky-400">Pausado</span>}
      </div>
      <div className="flex shrink-0 items-center gap-4">
        <span className="font-mono text-xl tabular-nums text-emerald-300">{formatElapsed(seconds)}</span>
        <button
          type="button"
          onClick={onToggle}
          disabled={grace || pending}
          className="rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700 disabled:cursor-not-allowed disabled:text-neutral-500"
        >
          {paused ? 'Retomar' : 'Pausar'}
        </button>
      </div>
    </section>
  )
}
