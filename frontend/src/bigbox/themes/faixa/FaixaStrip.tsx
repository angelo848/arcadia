import { CoverArt } from '../../CoverArt'
import { Panel } from '../../Panel'
import type { BigBoxTab, BigBoxGame } from '../../viewModel'

type Props = {
  tabs: BigBoxTab[]
  tab: number
  games: BigBoxGame[]
  focus: number
  stripStart: number
  counter: string | null
  onPick: (index: number) => void
  onPlay: (index: number) => void
}

const sessionText = { running: 'Em execução', paused: 'Pausado' } as const

// Tab bar plus the cover strip; `games` is the window of the model starting at `stripStart` [BBX-006, BBX-008, BBX-017, BBX-039, BBX-045].
export function FaixaStrip({ tabs, tab, games, focus, stripStart, counter, onPick, onPlay }: Props) {
  return (
    <div>
      <Panel className="w-fit max-w-full">
        <div role="tablist" className="flex gap-6 overflow-hidden text-bb-text-muted">
          {tabs.map((t, i) => (
            <button
              key={t.key}
              role="tab"
              aria-selected={i === tab}
              tabIndex={-1}
              className={i === tab ? 'whitespace-nowrap border-b-2 border-bb-focus font-semibold text-bb-text' : 'whitespace-nowrap'}
            >
              {t.label}
            </button>
          ))}
        </div>
      </Panel>
      <div className="flex items-center gap-4 overflow-hidden px-2 py-6">
        {games.map((g, i) => {
          const index = stripStart + i
          const focused = index === focus
          return (
            <div
              key={g.id}
              data-testid="bb-cover"
              data-focused={focused ? 'true' : 'false'}
              role="button"
              aria-label={g.title}
              onClick={() => onPick(index)}
              onDoubleClick={() => onPlay(index)}
              className={`relative flex aspect-[2/3] h-[18vh] shrink-0 items-center justify-center overflow-hidden rounded bg-bb-surface text-center text-bb-text ${
                focused ? 'z-10 scale-125 ring-2 ring-bb-focus' : ''
              }`}
            >
              <CoverArt game={g} />
              {g.session && (
                <span className="absolute bottom-0 left-0 right-0 bg-bb-surface px-1 text-xs text-bb-text">{sessionText[g.session]}</span>
              )}
            </div>
          )
        })}
      </div>
      {counter !== null && (
        <div className="flex justify-end">
          <Panel>
            <span data-testid="bb-strip-counter">{counter}</span>
          </Panel>
        </div>
      )}
    </div>
  )
}
