import type { JSX } from 'react'

// EmptyResults is shown when the active filters match no game [UI-035].
export function EmptyResults({ onClear }: { onClear: () => void }): JSX.Element {
  return (
    <section className="flex flex-col items-center justify-center gap-3 py-24 text-center">
      <p className="text-xl font-medium text-neutral-200">Nenhum jogo com estes filtros.</p>
      <button
        type="button"
        onClick={onClear}
        className="rounded bg-neutral-800 px-3 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700"
      >
        Limpar filtros
      </button>
    </section>
  )
}
