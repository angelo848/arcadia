// Cards of the Desktop theme [TDK-005, TDK-019, TDK-023, TDK-025, TDK-029]. The listbox belongs to the parent; no virtualizer.
import type { JSX } from 'react'
import { GRID_INSET, coverPlaceholderUrl } from '../../desktopLayout'
import type { BigBoxGame } from '../../viewModel'

export type DesktopItemsProps = {
  games: BigBoxGame[]
  start: number
  focus: number
  columns: number
  zoneFocused: boolean
  onPick: (index: number) => void
  onPlay: (index: number) => void
}

// itemClass is the selection look shared by cards and rows [TDK-023].
export function itemClass(base: string, selected: boolean, zoneFocused: boolean): string {
  if (!selected) return base
  return zoneFocused
    ? `${base} bg-bb-surface outline-2 outline-offset-2 outline-bb-focus`
    : `${base} bg-bb-surface ring-1 ring-bb-text-muted`
}

export function SessionBadge({ session }: { session: BigBoxGame['session'] }): JSX.Element | null {
  if (session === null) return null
  const cls = session === 'running' ? 'bg-bb-accent text-bb-on-accent' : 'bg-bb-chip text-bb-on-chip'
  return <span className={`rounded px-2 text-xs ${cls}`}>{session === 'running' ? 'Em execução' : 'Pausado'}</span>
}

export function wasPlayed(text: string | undefined): text is string {
  return !!text && text !== 'Nunca'
}

export function DesktopGrid(p: DesktopItemsProps): JSX.Element {
  return (
    <div
      className="grid content-start overflow-hidden"
      style={{ gridTemplateColumns: `repeat(${Math.max(p.columns, 1)}, minmax(0, 1fr))`, gap: GRID_INSET, padding: GRID_INSET }}
    >
      {p.games.map((g, i) => {
        const index = p.start + i
        const selected = index === p.focus
        const focused = selected && p.zoneFocused
        return (
          <div
            key={g.id}
            role="option"
            aria-label={g.title}
            aria-selected={selected}
            data-focused={focused ? 'true' : undefined}
            className={itemClass('flex min-w-0 flex-col gap-1 rounded-lg p-2 text-bb-text', selected, p.zoneFocused)}
            onClick={() => p.onPick(index)}
            onDoubleClick={() => p.onPlay(index)}
          >
            <div className="relative aspect-[3/4] w-full overflow-hidden rounded-md bg-bb-placeholder">
              <img
                src={g.coverUrl ?? coverPlaceholderUrl}
                alt=""
                className={`h-full w-full ${g.coverFit === 'cover' ? 'object-cover' : 'object-contain'}`}
              />
              <div className="absolute right-1 top-1">
                <SessionBadge session={g.session} />
              </div>
            </div>
            <p title={g.title} className="line-clamp-2 text-sm font-semibold text-bb-text">
              {g.title}
            </p>
            {wasPlayed(g.lastPlayedText) && <p className="text-xs text-bb-text-muted">{`Jogado ${g.lastPlayedText}`}</p>}
          </div>
        )
      })}
    </div>
  )
}
