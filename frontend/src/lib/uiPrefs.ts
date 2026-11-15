import { resolveTheme } from '../bigbox/themes/registry'
import type { FilterSpec } from './filters'

// Local UI preferences; they never go to the agent [UI-030, UIX-012, UIX-020].

// LibraryView is how the library is shown: the card grid or the list.
export type LibraryView = 'grid' | 'list'

const VIEW_KEY = 'arcadia.libraryView'
const FILTERS_KEY = 'arcadia.filtersOpen'

export function readLibraryView(storage?: Pick<Storage, 'getItem'>): LibraryView {
  try {
    const s = storage ?? window.localStorage
    return s.getItem(VIEW_KEY) === 'list' ? 'list' : 'grid'
  } catch {
    return 'grid'
  }
}

export function writeLibraryView(v: LibraryView, storage?: Pick<Storage, 'setItem'>): void {
  try {
    const s = storage ?? window.localStorage
    s.setItem(VIEW_KEY, v)
  } catch {
    // Storage unavailable: the preference is not kept.
  }
}

export type BigBoxThemeId = 'faixa' | 'mosaico' | 'roda'

const BIGBOX_THEME_KEY = 'arcadia.bigBoxTheme'

// readBigBoxTheme: only the available themes are accepted; anything else is 'faixa' [BBX-023].
export function readBigBoxTheme(storage?: Pick<Storage, 'getItem'>): BigBoxThemeId {
  try {
    const s = storage ?? window.localStorage
    return resolveTheme(s.getItem(BIGBOX_THEME_KEY)).id
  } catch {
    return 'faixa'
  }
}

export function writeBigBoxTheme(id: BigBoxThemeId, storage?: Pick<Storage, 'setItem'>): void {
  try {
    const s = storage ?? window.localStorage
    s.setItem(BIGBOX_THEME_KEY, id)
  } catch {
    // Storage unavailable: the preference is not kept.
  }
}

// readFiltersOpen is false when nothing is saved or the storage is unavailable [UIX-012].
export function readFiltersOpen(storage?: Pick<Storage, 'getItem'>): boolean {
  try {
    const s = storage ?? window.localStorage
    return s.getItem(FILTERS_KEY) === 'true'
  } catch {
    return false
  }
}

export function writeFiltersOpen(open: boolean, storage?: Pick<Storage, 'setItem'>): void {
  try {
    const s = storage ?? window.localStorage
    s.setItem(FILTERS_KEY, open ? 'true' : 'false')
  } catch {
    // Storage unavailable: the preference is not kept.
  }
}

// activeFacetCount counts the facets in use; version, text and platformIds are not facets [UIX-012].
export function activeFacetCount(f: FilterSpec): number {
  let n = 0
  const nonEmpty = (a: unknown[] | undefined) => a !== undefined && a.length > 0
  if (nonEmpty(f.genreIds)) n++
  if (nonEmpty(f.developerIds)) n++
  if (nonEmpty(f.publisherIds)) n++
  if (nonEmpty(f.statuses)) n++
  if (f.playMode !== undefined && f.playMode !== 'all') n++
  if (f.yearFrom != null) n++
  if (f.yearTo != null) n++
  if (f.favorite != null) n++
  if (f.played !== undefined && f.played !== 'any') n++
  if (f.playSecondsMin != null) n++
  if (f.playSecondsMax != null) n++
  return n
}
