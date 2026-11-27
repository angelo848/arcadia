// Read-only toolbar of the Desktop theme: count, order and view as plain text [TDK-004].
import type { JSX } from 'react'
import { TOOLBAR_GAP, TOOLBAR_PX } from '../../desktopLayout'
import type { LibraryView } from '../../viewModel'

export type DesktopToolbarProps = { total: number; sortLabel: string; view: LibraryView }

export function DesktopToolbar(p: DesktopToolbarProps): JSX.Element {
  return (
    <div
      className="flex shrink-0 items-center justify-between text-sm text-bb-text-muted"
      style={{ height: TOOLBAR_PX, marginBottom: TOOLBAR_GAP }}
    >
      <span>{`${p.total} ${p.total === 1 ? 'jogo' : 'jogos'}`}</span>
      <span className="flex gap-4">
        <span>{`Ordenar por: ${p.sortLabel}`}</span>
        <span>{p.view === 'list' ? 'Lista' : 'Grade'}</span>
      </span>
    </div>
  )
}
