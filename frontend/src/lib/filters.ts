// Filter and sort state for the library (UI-032..UI-035).
// The FilterSpec mirrors the IPC contract (ipc.openrpc.json, FilterSpec [FLT-001..FLT-007]).

export type GameStatus = 'not_played' | 'playing' | 'completed' | 'abandoned' | 'backlog'

export type PlayModeGroup = 'all' | 'single' | 'multi' | 'coop'

export type PlayedFilter = 'any' | 'played' | 'notPlayed'

export type SortKey = 'title' | 'lastPlayed' | 'totalPlayed' | 'year'

// Sort options in the order of the spec (UI-034, BBX-067).
export const sortOptions: ReadonlyArray<{ key: SortKey; label: string }> = [
  { key: 'title', label: 'Título' },
  { key: 'lastPlayed', label: 'Última vez jogado' },
  { key: 'totalPlayed', label: 'Tempo jogado' },
  { key: 'year', label: 'Ano' },
]

const sortKeys: ReadonlySet<string> = new Set(sortOptions.map((o) => o.key))

export function isSortKey(v: string): v is SortKey {
  return sortKeys.has(v)
}

// sortLabel is the label of a sort key.
export function sortLabel(k: SortKey): string {
  return sortOptions.find((o) => o.key === k)?.label ?? k
}

export type FilterSpec = {
  version: 1
  platformIds?: number[]
  genreIds?: number[]
  developerIds?: number[]
  publisherIds?: number[]
  playMode?: PlayModeGroup
  yearFrom?: number | null
  yearTo?: number | null
  statuses?: GameStatus[]
  favorite?: boolean | null
  played?: PlayedFilter
  playSecondsMin?: number | null
  playSecondsMax?: number | null
  text?: string
}

export type LibraryQuery = { filter: FilterSpec; sort: SortKey }

// The desktop library opens by the last played games; Big Box keeps its own order by title.
const defaultSort: SortKey = 'lastPlayed'

function makeDefaultQuery(): LibraryQuery {
  return { filter: { version: 1 }, sort: defaultSort }
}

// Shared default value; callers must not mutate it, use resetQuery() for a fresh copy.
export const defaultQuery: LibraryQuery = makeDefaultQuery()

// Returns a new default query each call, so changes to the result never leak into the default.
export function resetQuery(): LibraryQuery {
  return makeDefaultQuery()
}

// Sets the free-text filter. Blank text (empty or whitespace only) removes the `text` member.
// Other filters and the sort are kept; the input is not mutated.
export function withText(q: LibraryQuery, text: string): LibraryQuery {
  const { text: _previous, ...rest } = q.filter
  const trimmed = text.trim()
  const filter: FilterSpec = trimmed === '' ? rest : { ...rest, text: trimmed }
  return { ...q, filter }
}

// Sets the platform filter; undefined or an empty list removes `platformIds`. Other members and the sort are kept.
export function withPlatforms(q: LibraryQuery, ids: number[] | undefined): LibraryQuery {
  const { platformIds: _previous, ...rest } = q.filter
  const filter: FilterSpec = ids === undefined || ids.length === 0 ? rest : { ...rest, platformIds: ids }
  return { ...q, filter }
}

// Changes the sort order; the filter (including text) is kept as is.
export function withSort(q: LibraryQuery, sort: SortKey): LibraryQuery {
  return { ...q, sort }
}
