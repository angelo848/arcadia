import { useEffect, useRef } from 'react'
import { Panel } from '../../Panel'
import type { BigBoxViewModel } from '../../viewModel'

// Fixed header: "Sua biblioteca", the order label and the tabs with the LB/RB chips [ETN-004, BBX-054, BBX-055].
export function EstanteHeader({ model }: { model: BigBoxViewModel }) {
  const listRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    // The active tab stays in view; jsdom has no scrollIntoView.
    listRef.current?.querySelector<HTMLElement>('[aria-selected="true"]')?.scrollIntoView?.({ inline: 'nearest', block: 'nearest' })
  }, [model.tab])
  return (
    <div data-testid="est-header" className="shrink-0">
      <Panel className="flex flex-col gap-2 py-3">
        <div className="flex items-baseline justify-between gap-4">
          <h2 className="text-2xl font-bold text-bb-heading">Sua biblioteca</h2>
          <span data-testid="est-order" className="text-bb-text-muted">
            {`Ordem: ${model.sortLabel}`}
          </span>
        </div>
        <div className="flex items-center gap-3">
          <span data-testid="est-chip-lb" className="shrink-0 rounded-md bg-bb-chip px-2 py-0.5 text-sm font-bold text-bb-on-chip">
            LB
          </span>
          <div ref={listRef} role="tablist" className="flex min-w-0 flex-1 gap-2 overflow-hidden">
            {model.tabs.map((t, i) => {
              const active = i === model.tab
              return (
                <button
                  key={t.key}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  tabIndex={-1}
                  className={`whitespace-nowrap rounded-full px-5 py-1.5 font-semibold ${
                    active ? 'bg-bb-accent text-bb-on-accent' : 'bg-bb-surface text-bb-text-muted'
                  }`}
                >
                  {active && model.tabTotal !== null ? `${t.label} (${model.tabTotal})` : t.label}
                </button>
              )
            })}
          </div>
          <span data-testid="est-chip-rb" className="shrink-0 rounded-md bg-bb-chip px-2 py-0.5 text-sm font-bold text-bb-on-chip">
            RB
          </span>
        </div>
      </Panel>
    </div>
  )
}
