import type { JSX } from 'react'
import type { Timers } from '../lib/debounce'
import type { LibraryQuery } from '../lib/filters'
import { withPlatforms } from '../lib/filters'
import type { CatalogView } from '../lib/library'
import { FilterPanel } from './FilterPanel'
import { PlatformNav } from './PlatformNav'
import { SearchBox } from './SearchBox'

// Sidebar is the left column: search, platform list and the collapsible filters [UIX-008, UIX-010..UIX-012].
export function Sidebar({
  search,
  onSearchChange,
  query,
  catalog,
  onChange,
  timers,
}: {
  search: string
  onSearchChange: (text: string) => void
  query: LibraryQuery
  catalog: CatalogView | undefined
  onChange: (q: LibraryQuery) => void
  timers?: Timers
}): JSX.Element {
  return (
    <div className="flex flex-col gap-4">
      <SearchBox text={search} onChange={onSearchChange} timers={timers} />
      <PlatformNav
        platforms={catalog?.platforms}
        selectedIds={query.filter.platformIds}
        onChange={(ids) => onChange(withPlatforms(query, ids))}
      />
      <FilterPanel query={query} catalog={catalog} onChange={onChange} />
    </div>
  )
}
