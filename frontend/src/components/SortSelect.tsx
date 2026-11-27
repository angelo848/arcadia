import type { JSX } from 'react'
import { isSortKey, sortOptions, type SortKey } from '../lib/filters'

// SortSelect chooses the library sort order [UI-034]. The page reset is done by the caller.
export function SortSelect({ sort, onChange }: { sort: SortKey; onChange: (s: SortKey) => void }): JSX.Element {
  return (
    <label className="flex items-center gap-2 text-sm text-muted">
      <span>Ordenar por</span>
      <select
        value={sort}
        onChange={(e) => {
          if (isSortKey(e.target.value)) {
            onChange(e.target.value)
          }
        }}
        className="rounded bg-raised px-2 py-1.5 text-sm text-fg"
      >
        {sortOptions.map((o) => (
          <option key={o.key} value={o.key}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}
