import type { JSX } from 'react'
import type { PlatformView } from '../lib/library'
import { coverPlaceholderUrl } from './CoverImage'

function sameSelection(selectedIds: number[] | undefined, id: number): boolean {
  return selectedIds !== undefined && selectedIds.length === 1 && selectedIds[0] === id
}

// PlatformNav is the left-hand platform list [UIX-009, UIX-010]. Platforms with no games
// are hidden unless selected.
export function PlatformNav({
  platforms,
  selectedIds,
  onChange,
}: {
  platforms: PlatformView[] | undefined
  selectedIds: number[] | undefined
  onChange: (ids: number[] | undefined) => void
}): JSX.Element {
  const hasCatalog = platforms !== undefined
  const visible = (platforms ?? []).filter((p) => p.gameCount > 0 || selectedIds?.includes(p.id))
  const total = (platforms ?? []).reduce((sum, p) => sum + p.gameCount, 0)
  const allCurrent = selectedIds === undefined

  const base = 'flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-hover'
  const cls = (current: boolean) => `${base} ${current ? 'bg-raised text-fg' : 'text-muted'}`

  return (
    <nav className="flex flex-col gap-1">
      <h2 className="px-2 text-xs font-semibold uppercase text-muted">Plataformas</h2>
      <button
        type="button"
        aria-label={hasCatalog ? `Todos, ${total} jogos` : 'Todos'}
        aria-current={allCurrent ? 'true' : undefined}
        onClick={() => {
          if (!allCurrent) onChange(undefined)
        }}
        className={cls(allCurrent)}
      >
        <span className="flex-1 truncate">Todos</span>
        {hasCatalog && <span className="text-xs text-muted">{total}</span>}
      </button>
      {visible.map((p) => {
        const current = sameSelection(selectedIds, p.id)
        return (
          <button
            key={p.id}
            type="button"
            aria-label={`${p.name}, ${p.gameCount} jogos`}
            aria-current={current ? 'true' : undefined}
            onClick={() => {
              if (!current) onChange([p.id])
            }}
            className={cls(current)}
          >
            <img src={p.iconUrl || coverPlaceholderUrl} alt="" className="h-5 w-5 shrink-0 object-contain" />
            <span className="flex-1 truncate">{p.name}</span>
            <span className="text-xs text-muted">{p.gameCount}</span>
          </button>
        )
      })}
    </nav>
  )
}
