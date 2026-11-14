import type { JSX } from 'react'
import type { LibraryView } from '../lib/libraryView'

// ViewToggle switches the library between the card grid and the list [UI-030]. The
// button names the view it switches to, so from the grid it reads "Lista" and from
// the list it reads "Grade".
export function ViewToggle({
  view,
  onChange,
}: {
  view: LibraryView
  onChange: (v: LibraryView) => void
}): JSX.Element {
  const target: LibraryView = view === 'grid' ? 'list' : 'grid'
  const label = target === 'list' ? 'Lista' : 'Grade'
  const aria = target === 'list' ? 'Mostrar em lista' : 'Mostrar em grade'

  return (
    <button
      type="button"
      aria-label={aria}
      onClick={() => onChange(target)}
      className="shrink-0 rounded bg-raised px-3 py-1.5 text-sm font-medium text-fg hover:bg-hover"
    >
      {label}
    </button>
  )
}
