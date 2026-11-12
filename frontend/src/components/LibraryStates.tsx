import type { JSX } from 'react'
import { EmptyResults } from './EmptyResults'

// LibraryStates shows what the library grid replaces: the connecting state while the list is
// not loaded (or the call fails), the empty library, the filtered-empty result, or the count [UI-044, UI-035].
// onClear is optional only because the caller may not wire it yet; the button is always rendered in the filtered-empty state.
// addPending disables the "Adicionar jogo" button of the empty library while AddGame is in flight [UI-007].
export function LibraryStates({
  loaded,
  total,
  filtered,
  onAdd,
  onClear,
  addPending = false,
}: {
  loaded: boolean
  total: number | undefined
  filtered: boolean
  onAdd: () => void
  onClear?: () => void
  addPending?: boolean
}): JSX.Element {
  if (!loaded || total === undefined) {
    return (
      <section className="flex flex-col items-center justify-center py-24 text-center">
        <p className="text-sm text-neutral-400">Conectando ao agente…</p>
      </section>
    )
  }

  if (total === 0) {
    if (filtered) {
      return <EmptyResults onClear={onClear ?? noop} />
    }
    return (
      <section className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-xl font-medium text-neutral-200">Nenhum jogo ainda</p>
        <button
          type="button"
          disabled={addPending}
          onClick={onAdd}
          className="rounded bg-neutral-800 px-3 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700 disabled:cursor-not-allowed disabled:text-neutral-500"
        >
          Adicionar jogo
        </button>
      </section>
    )
  }

  // Plural is fixed by UI-044, also for one game.
  return <p className="text-sm text-neutral-400">{`${total} jogos`}</p>
}

function noop(): void {}
