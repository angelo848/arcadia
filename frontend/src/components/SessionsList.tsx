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

const iconProps = {
  width: 16,
  height: 16,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
  'aria-hidden': true,
}

function PencilIcon(): JSX.Element {
  return (
    <svg {...iconProps}>
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
    </svg>
  )
}

function TrashIcon(): JSX.Element {
  return (
    <svg {...iconProps}>
      <path d="M3 6h18" />
      <path d="M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2" />
      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
      <path d="M10 11v6M14 11v6" />
    </svg>
  )
}

// A session without active time is shown as 0 min; formatPlayTime(0) reads "Nunca jogado".
function durationText(activeSeconds: number): string {
  return activeSeconds > 0 ? formatPlayTime(activeSeconds) : '0 min'
}

// SessionsList shows the sessions of one game, 10 per page [UI-038]. The open session of
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

  const { hasNextPage, isFetchingNextPage, fetchNextPage } = q

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
                        title="Editar"
                        className="rounded p-1 text-fg hover:bg-hover"
                      >
                        <PencilIcon />
                      </button>
                    )}
                    <button
                      type="button"
                      aria-label={`Excluir sessão de ${formatLocalDateTime(s.startedAt)}`}
                      onClick={() => {
                        setDeleteError(null)
                        setDeleting(s)
                      }}
                      title="Excluir"
                      className="rounded p-1 text-danger hover:bg-hover"
                    >
                      <TrashIcon />
                    </button>
                  </span>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      {hasNextPage && (
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
