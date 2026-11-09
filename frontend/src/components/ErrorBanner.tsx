import type { JSX } from 'react'

// ErrorBanner shows the last failed action's message at the top of the list,
// with a "Fechar" button [UI-008].
export function ErrorBanner({ message, onClose }: { message: string; onClose: () => void }): JSX.Element {
  return (
    <div
      role="alert"
      className="mx-6 my-4 flex items-start justify-between gap-4 rounded border border-red-800 bg-red-950 px-4 py-3 text-sm text-red-200"
    >
      <p>{message}</p>
      <button
        type="button"
        onClick={onClose}
        className="shrink-0 rounded px-2 py-0.5 font-medium text-red-300 hover:bg-red-900"
      >
        Fechar
      </button>
    </div>
  )
}
