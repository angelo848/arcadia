// LibraryView is how the library is shown: the card grid or the list [UI-030].
export type LibraryView = 'grid' | 'list'

// The choice is a local preference kept in the UI itself; it never goes to the agent [UI-030].
const STORAGE_KEY = 'arcadia.libraryView'

// readLibraryView returns the saved view, or 'grid' when there is none or the storage
// is unavailable [UI-030].
export function readLibraryView(storage?: Pick<Storage, 'getItem'>): LibraryView {
  try {
    const s = storage ?? window.localStorage
    return s.getItem(STORAGE_KEY) === 'list' ? 'list' : 'grid'
  } catch {
    return 'grid'
  }
}

// writeLibraryView saves the view. If the storage refuses, the choice simply lasts
// until the window is closed [UI-030].
export function writeLibraryView(v: LibraryView, storage?: Pick<Storage, 'setItem'>): void {
  try {
    const s = storage ?? window.localStorage
    s.setItem(STORAGE_KEY, v)
  } catch {
    // Storage unavailable: the preference is not kept.
  }
}
