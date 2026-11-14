import type { JSX } from 'react'
import { EmptyResults } from './EmptyResults'

// LibraryStates shows what the library grid replaces: the connecting state while the list is
// not loaded (or the call fails), the empty library or the filtered-empty result [UI-044, UI-035].
// With games it renders nothing: the count is in LibraryToolbar [UIX-013].
// onClear is optional only because the caller may not wire it yet; the button is always rendered in the filtered-empty state.
export function LibraryStates({
  loaded,
  total,
  filtered,
  onClear,
}: {
  loaded: boolean
  total: number | undefined
  filtered: boolean
  onClear?: () => void
}): JSX.Element | null {
  if (!loaded || total === undefined) {
    return (
      <section className="flex flex-col items-center justify-center py-24 text-center">
        <p className="text-sm text-muted">Conectando ao agente…</p>
      </section>
    )
  }

  if (total === 0) {
    if (filtered) {
      return <EmptyResults onClear={onClear ?? noop} />
    }
    return (
      <section className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-xl font-medium text-fg">Nenhum jogo ainda</p>
        <p className="text-sm text-muted">Use Importar para adicionar jogos.</p>
      </section>
    )
  }

  return null
}

function noop(): void {}
