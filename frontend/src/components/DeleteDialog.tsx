import { useRef, type JSX } from 'react'
import { Modal } from './Modal'

export type DeleteDialogProps = {
  title: string
  // pending is true while DeleteGame is in flight: "Remover" is disabled, "Cancelar" stays enabled.
  pending: boolean
  // error carries the message shown in the dialog (DSK-018 for -32002); the parent clears it.
  error: string | null
  onConfirm: () => void
  onCancel: () => void
}

// DeleteDialog asks for confirmation before DeleteGame [UI-041]. It is modal with focus trapped inside:
// "Cancelar" and Esc call onCancel and never onConfirm; "Remover" calls onConfirm once per click.
// The dialog does not close itself: the parent removes it after success (or after the user cancels).
export function DeleteDialog(props: DeleteDialogProps): JSX.Element {
  const { title, pending, error, onConfirm, onCancel } = props
  const cancelRef = useRef<HTMLButtonElement>(null)

  return (
    <Modal
      size="compact"
      title="Remover jogo"
      onClose={onCancel}
      initialFocus={cancelRef}
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
            Remover
          </button>
        </>
      }
    >
      <p className="text-sm text-fg">
        {`Remover «${title}»? As sessões e as pausas deste jogo também serão apagadas. Esta ação não pode ser desfeita.`}
      </p>
      {error !== null && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
    </Modal>
  )
}
