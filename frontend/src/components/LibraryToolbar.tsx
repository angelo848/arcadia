import type { JSX } from 'react'
import type { SortKey } from '../lib/filters'
import type { LibraryView } from '../lib/libraryView'
import { SortSelect } from './SortSelect'
import { ViewToggle } from './ViewToggle'

// LibraryToolbar groups the game count, sort and view toggle above the grid [UIX-013].
export function LibraryToolbar({
  total,
  sort,
  view,
  onSortChange,
  onViewChange,
}: {
  total: number | undefined
  sort: SortKey
  view: LibraryView
  onSortChange: (s: SortKey) => void
  onViewChange: (v: LibraryView) => void
}): JSX.Element {
  return (
    <div className="flex items-center gap-3">
      {total !== undefined && total > 0 && <span className="text-sm text-muted">{total} jogos</span>}
      <div className="ml-auto flex items-center gap-3">
        <SortSelect sort={sort} onChange={onSortChange} />
        <ViewToggle view={view} onChange={onViewChange} />
      </div>
    </div>
  )
}
