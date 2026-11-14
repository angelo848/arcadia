import type { JSX } from 'react'

// ErrorBanner shows the last failed action's message at the top of the list,
// with a "Fechar" button [UI-008].
export function ErrorBanner({ message, onClose }: { message: string; onClose: () => void }): JSX.Element {
  return (
    <div
      role="alert"
      className="mx-6 my-4 flex items-start justify-between gap-4 rounded border border-danger-strong bg-surface px-4 py-3 text-sm text-danger"
    >
      <p>{message}</p>
      <button
        type="button"
        onClick={onClose}
        className="shrink-0 rounded px-2 py-0.5 font-medium text-danger hover:bg-hover"
      >
        Fechar
      </button>
    </div>
  )
}
