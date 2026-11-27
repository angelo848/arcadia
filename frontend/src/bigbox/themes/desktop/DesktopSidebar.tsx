// Platform column of the Desktop theme [TDK-003, TDK-021, TDK-023, TDK-029, BBX-065].
import { useEffect, useRef } from 'react'
import type { JSX } from 'react'
import type { BigBoxTab } from '../../viewModel'

export type DesktopSidebarProps = { tabs: BigBoxTab[]; tab: number; focused: boolean; onTab: (index: number) => void }

export function DesktopSidebar(p: DesktopSidebarProps): JSX.Element {
  const activeRef = useRef<HTMLButtonElement>(null)

  // The active item stays in view; jsdom has no scrollIntoView [TDK-021].
  useEffect(() => {
    activeRef.current?.scrollIntoView?.({ block: 'nearest' })
  }, [p.tab])

  return (
    <nav aria-label="Plataformas" className="flex min-h-0 flex-col gap-1 overflow-y-auto p-3">
      <p className="px-2 pb-2 text-sm font-semibold uppercase text-bb-heading">Plataformas</p>
      {p.tabs.map((t, i) => {
        const active = i === p.tab
        const focused = active && p.focused
        const name = t.count === undefined || t.count === null ? t.label : `${t.label}, ${t.count} jogos`
        const look = active ? 'bg-bb-surface text-bb-text' : 'text-bb-text-muted'
        return (
          <button
            key={t.key}
            ref={active ? activeRef : undefined}
            type="button"
            aria-label={name}
            aria-current={active ? 'true' : undefined}
            data-focused={focused ? 'true' : undefined}
            className={`flex items-center gap-2 rounded-md px-2 py-2 text-left ${look}${focused ? ' outline-2 outline-offset-2 outline-bb-focus' : ''}`}
            onClick={() => p.onTab(i)}
          >
            {t.iconUrl ? <img src={t.iconUrl} alt="" className="h-5 w-5 shrink-0 object-contain" /> : null}
            <span className="min-w-0 flex-1 truncate">{t.label}</span>
            {t.count !== undefined && t.count !== null ? <span className="text-xs text-bb-text-muted">{t.count}</span> : null}
          </button>
        )
      })}
    </nav>
  )
}
