import { useRef, type JSX } from 'react'
import { Modal } from './Modal'
import type { SessionItemView } from '../lib/library'
import { formatPlayTime } from '../lib/games'
import { formatLocalDateTime } from '../lib/formatDate'

export type SessionDeleteDialogProps = {
  session: SessionItemView
  pending: boolean
  error: string | null
  onConfirm: () => void
  onCancel: () => void
}

// SessionDeleteDialog asks for confirmation before deleting a session (SEL-012). It never calls the agent.
export function SessionDeleteDialog(p: SessionDeleteDialogProps): JSX.Element {
  const { session, pending, error, onConfirm, onCancel } = p
  const cancelRef = useRef<HTMLButtonElement>(null)
  const start = formatLocalDateTime(session.startedAt)
  const text =
    session.state === 'discarded'
      ? `Excluir a sessão descartada de «${start}»? Esta ação não pode ser desfeita.`
      : `Excluir a sessão de «${start}» («${session.activeSeconds > 0 ? formatPlayTime(session.activeSeconds) : '0 min'}»)? O tempo dela sai do tempo total do jogo. Esta ação não pode ser desfeita.`

  return (
    <Modal
      size="compact"
      title="Excluir sessão"
      onClose={onCancel}
      initialFocus={cancelRef}
      closeDisabled={pending}
      footer={
        <>
          <button
            ref={cancelRef}
            type="button"
            onClick={() => onCancel()}
            className="rounded px-3 py-1 text-sm font-medium text-muted hover:bg-hover"
          >
            Cancelar
          </button>
          <button
            type="button"
            disabled={pending}
            onClick={() => onConfirm()}
            className="rounded bg-danger-strong px-3 py-1 text-sm font-medium text-on-accent hover:opacity-90 disabled:opacity-50"
          >
            {pending ? 'Excluindo…' : 'Excluir'}
          </button>
        </>
      }
    >
      <p className="text-sm text-fg">{text}</p>
      {error !== null && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
    </Modal>
  )
}
