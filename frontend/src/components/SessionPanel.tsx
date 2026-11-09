import { useMemo, type JSX } from 'react'
import type { SessionView } from '../lib/games'
import { anchorFrom, formatElapsed, useElapsedSeconds } from '../lib/sessionClock'

// SessionPanel shows the current session's game and its live clock [UI-012].
// Each new session value is a new anchor taken at reception [UI-014]; in grace
// the clock is frozen and "Encerrando…" is shown [UI-015]. Display only: it
// never calls a binding [UI-017].
export function SessionPanel({ session, now }: { session: SessionView; now?: () => number }): JSX.Element {
  // The anchor depends only on the received value: an unstable `now` prop must
  // not reset the reception instant on every render.
  const anchor = useMemo(() => anchorFrom(session, now), [session])
  const seconds = useElapsedSeconds(anchor, now)
  const grace = session.state === 'grace'

  return (
    <section
      role="status"
      aria-label="Sessão em andamento"
      className="mx-6 my-4 flex items-center justify-between gap-4 rounded border border-emerald-800 bg-emerald-950/40 px-4 py-3"
    >
      <div className="flex min-w-0 flex-col">
        <span className="truncate font-medium text-neutral-100">{session.title}</span>
        {grace && <span className="text-sm text-amber-400">Encerrando…</span>}
      </div>
      <span className="font-mono text-xl tabular-nums text-emerald-300">{formatElapsed(seconds)}</span>
    </section>
  )
}
