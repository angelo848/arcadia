import { Panel } from '../../Panel'
import type { BigBoxViewModel } from '../../viewModel'

export const sessionText = { running: 'Em execução', paused: 'Pausado' } as const

function PlayIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" width="1.1em" height="1.1em" fill="currentColor">
      <path d="M8 5v14l11-7z" />
    </svg>
  )
}

// Heart is only an indicator of the favorite flag: no button, no handler [ETN-002].
function Heart({ filled }: { filled: boolean }) {
  return (
    <span role="img" aria-label={filled ? 'Favorito' : 'Não favorito'} className="inline-flex text-bb-heading">
      <svg viewBox="0 0 24 24" width="2rem" height="2rem" fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth={2}>
        <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z" />
      </svg>
    </span>
  )
}

type Props = { model: BigBoxViewModel; onPlay: (index: number) => void; onDetails: () => void }

// Highlight of the focused game while the focus is on row 0 [ETN-002, ETN-003].
export function EstanteHero({ model, onPlay, onDetails }: Props) {
  const f = model.focused
  if (!f) return null
  const session = f.game.session
  const never = f.lastPlayedAt === null || f.lastPlayedRelative === 'Nunca jogado'
  return (
    <div data-testid="est-hero" className="flex h-full min-h-0 items-end justify-between gap-8 pb-2">
      <div className="flex min-w-0 max-w-[62%] flex-col items-start gap-4">
        <Panel className="flex min-w-0 flex-col gap-1 py-3">
          <div className="flex min-w-0 items-center gap-4">
            <h1
              data-testid="est-title"
              className="line-clamp-2 min-w-0 text-[length:clamp(2.4rem,4vw,4.5rem)] font-extrabold leading-[1.05] tracking-tight text-bb-heading"
            >
              {f.game.title}
            </h1>
            {session && (
              <span className="shrink-0 rounded-full bg-bb-accent px-3 py-0.5 text-sm font-semibold text-bb-on-accent">
                {sessionText[session]}
              </span>
            )}
          </div>
          <p data-testid="est-last" className="text-lg text-bb-text-muted">
            {never ? 'Nunca jogado' : `Última sessão ${f.lastPlayedRelative}`}
          </p>
        </Panel>
        <div className="flex items-center gap-4">
          <button
            type="button"
            onClick={() => onPlay(model.focus)}
            className="inline-flex items-center gap-2 rounded-lg bg-bb-accent px-7 py-3 text-xl font-bold text-bb-on-accent"
          >
            <PlayIcon />
            Iniciar jogo
          </button>
          <button type="button" onClick={onDetails} className="rounded-lg bg-bb-surface px-7 py-3 text-xl font-semibold text-bb-text">
            Detalhes
          </button>
          <Heart filled={f.favorite} />
        </div>
      </div>
      <div className="flex shrink-0 gap-4">
        <Panel className="min-w-[10rem] py-3">
          <div data-testid="est-stat-time" className="flex flex-col gap-1">
            <span className="text-sm uppercase tracking-wider text-bb-text-muted">Tempo jogado</span>
            <span className="text-2xl font-bold text-bb-heading">{f.playTimeLabel}</span>
          </div>
        </Panel>
        <Panel className="min-w-[8rem] py-3">
          <div data-testid="est-stat-sessions" className="flex flex-col gap-1">
            <span className="text-sm uppercase tracking-wider text-bb-text-muted">Sessões</span>
            <span className="text-2xl font-bold text-bb-heading">{String(f.playCount)}</span>
          </div>
        </Panel>
      </div>
    </div>
  )
}
