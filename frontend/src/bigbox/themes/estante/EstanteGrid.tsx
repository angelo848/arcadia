import { Panel } from '../../Panel'
import { CoverArt } from '../../widgets'
import type { BigBoxViewModel } from '../../viewModel'
import { ESTANTE_COLUMNS } from './capacity'
import { sessionText } from './EstanteHero'

type Props = { model: BigBoxViewModel; onPick: (index: number) => void; onPlay: (index: number) => void }

const focusStyle = { outline: '3px solid var(--arc-bb-focus)', outlineOffset: '2px', transform: 'scale(1.05)' }

// Grid of 2:3 cards; the window starts at the focus row and the row below is cut by the overflow [ETN-005, ETN-007].
export function EstanteGrid({ model, onPick, onPlay }: Props) {
  return (
    <div
      data-testid="est-grid"
      data-columns={ESTANTE_COLUMNS}
      className="grid min-h-0 flex-1 grid-cols-5 content-start gap-x-6 gap-y-6 overflow-hidden px-3 pt-3"
    >
      {model.games.map((g, i) => {
        const index = model.stripStart + i
        const focused = index === model.focus
        return (
          <div key={g.id} data-testid="est-card" className="flex min-w-0 flex-col gap-2">
            <div
              data-testid="bb-cover"
              data-focused={focused ? 'true' : 'false'}
              role="button"
              aria-label={g.title}
              onClick={() => onPick(index)}
              onDoubleClick={() => onPlay(index)}
              style={focused ? focusStyle : undefined}
              className={`relative flex aspect-[2/3] w-full items-center justify-center overflow-hidden rounded-lg text-center text-bb-text transition-transform duration-150 ${
                g.coverUrl ? 'bg-bb-surface' : 'bg-bb-placeholder'
              } ${focused ? 'z-10' : ''}`}
            >
              <CoverArt game={g} />
              {g.session && (
                <span className="absolute bottom-0 left-0 right-0 bg-bb-accent px-1 py-0.5 text-xs font-semibold text-bb-on-accent">
                  {sessionText[g.session]}
                </span>
              )}
            </div>
            <Panel className="flex min-w-0 flex-col py-1">
              <span data-testid="est-card-title" className="truncate font-semibold text-bb-text">
                {g.title}
              </span>
              <span data-testid="est-card-time" className="truncate text-sm text-bb-text-muted">
                {g.playTimeLabel ?? ''}
              </span>
            </Panel>
          </div>
        )
      })}
    </div>
  )
}
