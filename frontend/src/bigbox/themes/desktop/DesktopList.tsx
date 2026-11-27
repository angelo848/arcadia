// Rows of the Desktop list view [TDK-006, TDK-023, TDK-025, TDK-029]. The listbox belongs to the parent; no virtualizer.
import type { JSX } from 'react'
import { GRID_INSET, LIST_ROW_HEIGHT } from '../../desktopLayout'
import { SessionBadge, itemClass } from './DesktopGrid'
import type { DesktopItemsProps } from './DesktopGrid'

export function DesktopList(p: DesktopItemsProps): JSX.Element {
  return (
    <div className="flex flex-col overflow-hidden" style={{ padding: GRID_INSET }}>
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
            className={itemClass('flex items-center gap-4 rounded-md px-3 text-bb-text', selected, p.zoneFocused)}
            style={{ height: LIST_ROW_HEIGHT }}
            onClick={() => p.onPick(index)}
            onDoubleClick={() => p.onPlay(index)}
          >
            <span title={g.title} className="min-w-0 flex-1 truncate font-semibold">
              {g.title}
            </span>
            <span className="w-40 truncate text-bb-text-muted">{g.platform ?? ''}</span>
            <span className="w-28 truncate text-bb-text-muted">{g.playTimeText ?? ''}</span>
            <span className="w-40 truncate text-bb-text-muted">{g.lastPlayedText || 'Nunca'}</span>
            <span className="w-24">
              <SessionBadge session={g.session} />
            </span>
          </div>
        )
      })}
    </div>
  )
}
