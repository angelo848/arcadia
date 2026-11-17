import { Panel } from '../../Panel'
import { infoLine } from '../../viewModel'
import type { BigBoxFocused } from '../../viewModel'

type Props = {
  focused: BigBoxFocused
  activeTab: string
  onPlay: () => void
  onDetails: () => void
}

const sessionText = { running: 'Em execução', paused: 'Pausado' } as const

// Information block of the focused game [BBX-009, BBX-017].
export function FaixaInfo({ focused: f, activeTab, onPlay, onDetails }: Props) {
  let line = infoLine(f.platform, f.year, f.genre)
  // The active tab already names the platform; do not repeat it when it is the only part.
  if (line === activeTab) line = ''
  return (
    <Panel className="flex max-w-[40%] flex-col gap-3">
      <div className="flex items-center gap-4">
        <h1 className="text-[clamp(2.4rem,4vw,4rem)] font-bold leading-tight text-bb-text">{f.game.title}</h1>
        {f.game.session && (
          <span className="rounded bg-bb-surface px-2 py-1 text-sm text-bb-text">{sessionText[f.game.session]}</span>
        )}
      </div>
      {line !== '' && <p className="text-[clamp(1.3rem,1.9vw,1.9rem)] text-bb-text">{line}</p>}
      {f.shortDescription !== '' && <p className="line-clamp-3 text-bb-text-muted">{f.shortDescription}</p>}
      <p className="text-bb-text">
        <span>{f.playTimeLabel}</span>
        {f.lastPlayedLabel && <span>{` · Última vez ${f.lastPlayedLabel}`}</span>}
        <span>{` · ${f.sessionsLabel}`}</span>
      </p>
      <div className="flex gap-3">
        <button type="button" onClick={onPlay} className="rounded bg-bb-accent px-6 py-2 font-semibold text-bb-on-accent focus:ring-2 focus:ring-bb-focus">
          Jogar
        </button>
        <button type="button" onClick={onDetails} className="rounded bg-bb-surface px-6 py-2 text-bb-text focus:ring-2 focus:ring-bb-focus">
          Detalhes
        </button>
      </div>
    </Panel>
  )
}
