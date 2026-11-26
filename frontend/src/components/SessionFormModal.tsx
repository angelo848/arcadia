import { useRef, useState, type JSX, type KeyboardEvent } from 'react'
import { Modal } from './Modal'
import type { GameDetailView, SessionItemView } from '../lib/library'
import { formatPlayTime } from '../lib/games'
import { isSessionNotFound, useCreateSession, useUpdateSession } from '../lib/sessionEdit'
import {
  changeField,
  createFormState,
  editFormState,
  manualAdjustmentSeconds,
  showIncludedInTotal,
  toCreateInput,
  toUpdatePatch,
  validateSessionForm,
  type SessionFormState,
  type SessionFormValues,
} from '../lib/sessionForm'

export type SessionFormModalProps = {
  mode: 'create' | 'edit'
  game: GameDetailView
  session?: SessionItemView // edit only
  now?: () => Date // injectable clock of the UI
  onClose: () => void // cancel or successful save
  onGone: () => void // the session no longer exists: close and reload the list
}

const inputClass =
  'rounded border border-line bg-app px-2 py-1 text-fg focus:border-accent aria-[invalid=true]:border-danger'

// SessionFormModal creates or edits one session of a game (SEL-005..SEL-011, SEL-015).
export function SessionFormModal(p: SessionFormModalProps): JSX.Element {
  const { mode, game, session, onClose, onGone } = p
  const gameId = game.id
  const create = useCreateSession(gameId)
  const update = useUpdateSession(gameId)
  const startRef = useRef<HTMLInputElement>(null)
  const [state, setState] = useState<SessionFormState>(() =>
    mode === 'edit' && session !== undefined ? editFormState(session) : createFormState((p.now ?? (() => new Date()))()),
  )
  const [agentError, setAgentError] = useState<string | null>(null)
  const sending = useRef(false)
  const nowFn = p.now ?? (() => new Date())

  const { values } = state
  const showBox = showIncludedInTotal(mode, game)
  const errors = validateSessionForm(values, {
    now: nowFn(),
    source: session?.source ?? 'manual',
    adjustmentSeconds: mode === 'create' ? manualAdjustmentSeconds(game) : null,
  })
  const hasErrors = errors.start !== undefined || errors.end !== undefined || errors.active !== undefined
  const patch = mode === 'edit' && session !== undefined ? toUpdatePatch(state.initial, values, session.activeSeconds) : {}
  const changed = mode === 'create' || Object.keys(patch).length > 0
  const pending = create.isPending || update.isPending
  const canSave = !hasErrors && changed && !pending

  function set(field: keyof SessionFormValues, value: string | boolean) {
    setAgentError(null)
    setState((s) => changeField(s, field, value))
  }

  async function save() {
    if (!canSave || sending.current) {
      return
    }
    sending.current = true
    setAgentError(null)
    try {
      if (mode === 'edit' && session !== undefined) {
        await update.mutateAsync({ sessionId: session.sessionId, patch })
      } else {
        await create.mutateAsync(toCreateInput(values))
      }
      onClose()
    } catch (err) {
      if (isSessionNotFound(err)) {
        onGone()
        return
      }
      setAgentError(err instanceof Error ? err.message : String(err))
    } finally {
      sending.current = false
    }
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      void save()
    }
  }

  const startErrId = 'session-start-error'
  const endErrId = 'session-end-error'
  const activeErrId = 'session-active-error'

  return (
    <Modal
      size="compact"
      title={mode === 'create' ? 'Adicionar sessão' : 'Editar sessão'}
      onClose={onClose}
      initialFocus={startRef}
      closeDisabled={pending}
      footer={
        <>
          <button
            type="button"
            disabled={pending}
            onClick={() => onClose()}
            className="rounded px-3 py-1 text-sm font-medium text-muted hover:bg-hover disabled:opacity-50"
          >
            Cancelar
          </button>
          <button
            type="button"
            disabled={!canSave}
            onClick={() => void save()}
            className="rounded bg-accent px-3 py-1 text-sm font-medium text-on-accent hover:opacity-90 disabled:opacity-50"
          >
            {pending ? 'Salvando…' : 'Salvar'}
          </button>
        </>
      }
    >
      <div className="flex flex-col gap-1 text-sm">
        <label htmlFor="session-start" className="text-fg">
          Início
        </label>
        <input
          id="session-start"
          ref={startRef}
          type="datetime-local"
          step="60"
          value={values.start}
          onChange={(e) => set('start', e.target.value)}
          onKeyDown={onKeyDown}
          aria-invalid={errors.start !== undefined}
          aria-describedby={errors.start !== undefined ? startErrId : undefined}
          className={inputClass}
        />
        {errors.start !== undefined && (
          <p id={startErrId} className="text-xs text-danger">
            {errors.start}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-1 text-sm">
        <label htmlFor="session-end" className="text-fg">
          Fim
        </label>
        <input
          id="session-end"
          type="datetime-local"
          step="60"
          value={values.end}
          onChange={(e) => set('end', e.target.value)}
          onKeyDown={onKeyDown}
          aria-invalid={errors.end !== undefined}
          aria-describedby={errors.end !== undefined ? endErrId : undefined}
          className={inputClass}
        />
        {errors.end !== undefined && (
          <p id={endErrId} className="text-xs text-danger">
            {errors.end}
          </p>
        )}
      </div>

      <fieldset className="flex flex-col gap-1 text-sm">
        <legend className="text-fg">Tempo jogado</legend>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-1 text-muted">
            <input
              type="number"
              min={0}
              value={values.hours}
              onChange={(e) => set('hours', e.target.value)}
              onKeyDown={onKeyDown}
              aria-invalid={errors.active !== undefined}
              aria-describedby={errors.active !== undefined ? activeErrId : undefined}
              className={`${inputClass} w-20`}
            />
            horas
          </label>
          <label className="flex items-center gap-1 text-muted">
            <input
              type="number"
              min={0}
              value={values.minutes}
              onChange={(e) => set('minutes', e.target.value)}
              onKeyDown={onKeyDown}
              aria-invalid={errors.active !== undefined}
              aria-describedby={errors.active !== undefined ? activeErrId : undefined}
              className={`${inputClass} w-20`}
            />
            minutos
          </label>
        </div>
        {errors.active !== undefined && (
          <p id={activeErrId} className="text-xs text-danger">
            {errors.active}
          </p>
        )}
      </fieldset>

      {showBox && (
        <div className="flex flex-col gap-1 text-sm">
          <label className="flex items-center gap-2 text-fg">
            <input
              type="checkbox"
              checked={values.includedInTotal}
              onChange={(e) => set('includedInTotal', e.target.checked)}
            />
            Já está no tempo total
          </label>
          <p className="text-xs text-muted">
            {`O tempo total deste jogo tem ${formatPlayTime(manualAdjustmentSeconds(game))} a mais que as sessões. Marque se esta sessão já faz parte desse tempo.`}
          </p>
        </div>
      )}

      {agentError !== null && (
        <p role="alert" className="text-sm text-danger">
          {agentError}
        </p>
      )}
    </Modal>
  )
}
