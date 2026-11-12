import { useEffect, useId, useRef, type JSX, type KeyboardEvent } from 'react'

export type DeleteDialogProps = {
  title: string
  // pending is true while DeleteGame is in flight: "Remover" is disabled, "Cancelar" stays enabled.
  pending: boolean
  // error carries the message shown in the dialog (DSK-018 for -32002); the parent clears it.
  error: string | null
  onConfirm: () => void
  onCancel: () => void
}

const focusableSelector = [
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'a[href]',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

// DeleteDialog asks for confirmation before DeleteGame [UI-041]. It is modal with focus trapped inside:
// "Cancelar" and Esc call onCancel and never onConfirm; "Remover" calls onConfirm once per click.
// The dialog does not close itself: the parent removes it after success (or after the user cancels).
export function DeleteDialog(props: DeleteDialogProps): JSX.Element {
  const { title, pending, error, onConfirm, onCancel } = props
  const titleId = useId()
  const dialogRef = useRef<HTMLDivElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)

  // Focus starts on "Cancelar", the safe choice for a destructive action; on unmount the focus goes back.
  useEffect(() => {
    const previous = document.activeElement
    cancelRef.current?.focus()
    return () => {
      if (previous instanceof HTMLElement) {
        previous.focus()
      }
    }
  }, [])

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onCancel()
      return
    }
    if (event.key !== 'Tab') {
      return
    }
    const dialog = dialogRef.current
    if (dialog === null) {
      return
    }
    const items = Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector))
    const first = items[0]
    const last = items[items.length - 1]
    if (first === undefined || last === undefined) {
      return
    }
    const active = document.activeElement
    const outside = active === null || !dialog.contains(active)
    if (event.shiftKey && (outside || active === first)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && (outside || active === last)) {
      event.preventDefault()
      first.focus()
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onKeyDown={onKeyDown}
        className="flex w-full max-w-md flex-col gap-4 rounded-lg bg-neutral-900 p-6 shadow-xl"
      >
        <h2 id={titleId} className="text-lg font-semibold text-neutral-100">
          Remover jogo
        </h2>
        <p className="text-sm text-neutral-200">
          {`Remover «${title}»? As sessões e as pausas deste jogo também serão apagadas. Esta ação não pode ser desfeita.`}
        </p>
        {error !== null && (
          <p role="alert" className="text-sm text-red-400">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button
            ref={cancelRef}
            type="button"
            onClick={() => onCancel()}
            className="rounded px-3 py-1 text-sm font-medium text-neutral-300 hover:bg-neutral-800"
          >
            Cancelar
          </button>
          <button
            type="button"
            disabled={pending}
            onClick={() => onConfirm()}
            className="rounded bg-red-600 px-3 py-1 text-sm font-medium text-white hover:bg-red-500 disabled:opacity-50"
          >
            Remover
          </button>
        </div>
      </div>
    </div>
  )
}
