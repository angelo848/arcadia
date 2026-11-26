import { useEffect, useRef, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { libraryKeys, useGameSessions } from '../lib/library'
import type { GameDetailView, SessionItemView } from '../lib/library'
import { isSessionNotFound, useDeleteSession } from '../lib/sessionEdit'
import { SessionFormModal } from './SessionFormModal'
import { SessionDeleteDialog } from './SessionDeleteDialog'
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
  running: 'font-medium text-running',
  paused: 'font-medium text-running',
  grace: 'font-medium text-running',
  ended: 'text-muted',
  discarded: 'text-warning',
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
  game,
}: {
  gameId: number
  current: SessionView | null
  game: GameDetailView
}): JSX.Element {
  const q = useGameSessions(gameId)
  const toggle = useToggleSession()
  const qc = useQueryClient()
  const del = useDeleteSession(gameId)
  const addRef = useRef<HTMLButtonElement>(null)
  const [form, setForm] = useState<{
    mode: 'create' | 'edit'
    session?: SessionItemView
  } | null>(null)
  const [deleting, setDeleting] = useState<SessionItemView | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [focusAdd, setFocusAdd] = useState(0)
  const sessions: SessionItemView[] = q.data?.pages.flatMap((p) => p.sessions) ?? []

  // Infinite scroll: the next page is asked for when the end of the list scrolls into view (UI-038).
  const endRef = useRef<HTMLDivElement>(null)
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = q
  const canObserve = typeof IntersectionObserver !== 'undefined'
  useEffect(() => {
    const el = endRef.current
    if (!canObserve || el === null || !hasNextPage || isFetchingNextPage) {
      return
    }
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) {
        void fetchNextPage()
      }
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [canObserve, hasNextPage, isFetchingNextPage, fetchNextPage, sessions.length])

  // The row that held the focus is gone after a delete: the focus goes to "Adicionar sessão" (SEL-015).
  useEffect(() => {
    if (focusAdd > 0) {
      addRef.current?.focus()
    }
  }, [focusAdd])

  function reload() {
    void qc.invalidateQueries({ queryKey: libraryKeys.sessions(gameId) })
  }

  async function confirmDelete() {
    if (deleting === null) {
      return
    }
    setDeleteError(null)
    try {
      await del.mutateAsync(deleting.sessionId)
      setDeleting(null)
      setFocusAdd((n) => n + 1)
    } catch (err) {
      if (isSessionNotFound(err)) {
        setDeleting(null)
        setFocusAdd((n) => n + 1)
        reload()
        return
      }
      setDeleteError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <section className="flex flex-col gap-2">
      {current !== null && current.gameId === gameId && (
        <SessionPanel session={current} onToggle={() => toggle.mutate()} pending={toggle.isPending} />
      )}

      <button
        ref={addRef}
        type="button"
        onClick={() => setForm({ mode: 'create' })}
        className="self-start rounded bg-raised px-3 py-1 text-sm font-medium text-fg hover:bg-hover"
      >
        Adicionar sessão
      </button>

      {q.isError ? (
        <p role="alert" className="text-sm text-danger">
          {q.error?.message}
        </p>
      ) : q.isPending ? (
        <p className="text-sm text-muted">Carregando sessões…</p>
      ) : sessions.length === 0 ? (
        <p className="text-sm text-muted">Nenhuma sessão registrada.</p>
      ) : (
        <ul className="flex flex-col">
          {sessions.map((s) => (
            <li key={s.sessionId} className="flex flex-col gap-1 border-b border-line py-2 text-sm">
              <div className="grid grid-cols-[1fr_1fr_auto] items-baseline gap-x-3">
                <span className="whitespace-nowrap text-fg">{formatLocalDateTime(s.startedAt)}</span>
                <span className="whitespace-nowrap text-muted">{s.endedAt ? formatLocalDateTime(s.endedAt) : '—'}</span>
                <span className="whitespace-nowrap text-right tabular-nums text-fg">
                  {durationText(s.activeSeconds)}
                </span>
              </div>
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className={stateClasses[s.state]}>{stateLabels[s.state]}</span>
                {s.endReason === 'closed-by-hotkey' && <span className="text-muted">Fechado pelo controle</span>}
                {s.source === 'manual' && <span className="text-muted">Manual</span>}
                {s.source === 'import' && <span className="text-muted">Importada</span>}
                {s.editedAt !== '' && <span className="text-warning">Editada</span>}
                {s.state !== 'running' && s.state !== 'paused' && s.state !== 'grace' && (
                  <span className="ml-auto flex gap-2">
                    {s.state === 'ended' && (
                      <button
                        type="button"
                        aria-label={`Editar sessão de ${formatLocalDateTime(s.startedAt)}`}
                        onClick={() => setForm({ mode: 'edit', session: s })}
                        className="rounded px-2 py-0.5 text-xs text-fg hover:bg-hover"
                      >
                        Editar
                      </button>
                    )}
                    <button
                      type="button"
                      aria-label={`Excluir sessão de ${formatLocalDateTime(s.startedAt)}`}
                      onClick={() => {
                        setDeleteError(null)
                        setDeleting(s)
                      }}
                      className="rounded px-2 py-0.5 text-xs text-danger hover:bg-hover"
                    >
                      Excluir
                    </button>
                  </span>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      {hasNextPage && canObserve && <div ref={endRef} aria-hidden="true" className="h-px" />}
      {isFetchingNextPage && <p className="text-center text-sm text-muted">Carregando sessões…</p>}
      {hasNextPage && !canObserve && (
        <button
          type="button"
          onClick={() => void fetchNextPage()}
          disabled={isFetchingNextPage}
          className="self-center rounded bg-raised px-4 py-1.5 text-sm font-medium text-fg hover:bg-hover disabled:cursor-not-allowed disabled:text-muted"
        >
          Carregar mais
        </button>
      )}

      {form !== null && (
        <SessionFormModal
          mode={form.mode}
          game={game}
          session={form.session}
          onClose={() => setForm(null)}
          onGone={() => {
            setForm(null)
            setFocusAdd((n) => n + 1)
            reload()
          }}
        />
      )}
      {deleting !== null && (
        <SessionDeleteDialog
          session={deleting}
          pending={del.isPending}
          error={deleteError}
          onConfirm={() => void confirmDelete()}
          onCancel={() => setDeleting(null)}
        />
      )}
    </section>
  )
}
