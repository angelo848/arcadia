// Filter and sort state for the library (UI-032..UI-035).
// The FilterSpec mirrors the IPC contract (ipc.openrpc.json, FilterSpec [FLT-001..FLT-007]).

export type GameStatus = 'not_played' | 'playing' | 'completed' | 'abandoned' | 'backlog'

export type PlayModeGroup = 'all' | 'single' | 'multi' | 'coop'

export type PlayedFilter = 'any' | 'played' | 'notPlayed'

export type SortKey = 'title' | 'lastPlayed' | 'totalPlayed' | 'year'

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

const defaultSort: SortKey = 'title'

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

// Changes the sort order; the filter (including text) is kept as is.
export function withSort(q: LibraryQuery, sort: SortKey): LibraryQuery {
  return { ...q, sort }
}
