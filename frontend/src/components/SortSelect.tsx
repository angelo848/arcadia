import type { JSX } from 'react'
import type { SortKey } from '../lib/filters'

// Sort options in the order of the spec (UI-034).
const sortOptions: ReadonlyArray<{ key: SortKey; label: string }> = [
  { key: 'title', label: 'Título' },
  { key: 'lastPlayed', label: 'Última vez jogado' },
  { key: 'totalPlayed', label: 'Tempo jogado' },
  { key: 'year', label: 'Ano' },
]

const sortKeys: ReadonlySet<string> = new Set(sortOptions.map((o) => o.key))

function isSortKey(v: string): v is SortKey {
  return sortKeys.has(v)
}

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
