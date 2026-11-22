import { Panel } from '../../Panel'
import type { BigBoxThemeProps } from '../types'
import { MosaicoPanel } from './MosaicoPanel'
import { MosaicoRow, sessionText } from './MosaicoRow'

// Mosaico theme: platform pills, title, square cover row and an information panel [MOS-001..MOS-014].
export function Mosaico({ model, details, onPick, onPlay, onVideoError }: BigBoxThemeProps) {
  if (model.screen === 'details') return <div className="relative flex h-full min-h-0 flex-col text-bb-text">{details}</div>
  const empty = model.total === 0 || !model.focused
  const emptyText = model.tab === 0 ? 'Nenhum jogo na biblioteca' : 'Nenhum jogo nesta plataforma'
  const session = model.focused?.game.session ?? null
  return (
    <div className="relative flex h-full min-h-0 flex-col gap-2 text-bb-text">
      <div className="flex items-center justify-between gap-4">
        <div role="tablist" className="flex min-w-0 gap-2 overflow-hidden">
          {model.tabs.map((t, i) => (
            <button
              key={t.key}
              role="tab"
              aria-selected={i === model.tab}
              tabIndex={-1}
              className={`whitespace-nowrap rounded-full px-5 py-1.5 font-semibold ${
                i === model.tab ? 'bg-bb-accent text-bb-on-accent' : 'bg-bb-surface text-bb-text-muted'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>
        {!model.loading && !empty && (
          <span data-testid="bb-count" className="shrink-0 text-bb-text-muted">
            {model.total === 1 ? '1 jogo' : `${model.total} jogos`}
          </span>
        )}
      </div>
      {!model.loading && !empty && (
        <div className="flex items-baseline gap-4 px-3">
          <h2 data-testid="mos-title" className="min-w-0 truncate whitespace-nowrap text-4xl font-bold text-bb-heading">
            {model.focused!.game.title}
          </h2>
          {session && <span className="shrink-0 rounded-full bg-bb-accent px-3 text-sm text-bb-on-accent">{sessionText[session]}</span>}
        </div>
      )}
      {!model.loading && !empty && (
        <MosaicoRow model={model} onPick={onPick} onPlay={onPlay} onVideoError={onVideoError} />
      )}
      {model.loading ? (
        <Panel className="w-fit text-2xl text-bb-text-muted">
          <p>Carregando jogos…</p>
        </Panel>
      ) : empty ? (
        <Panel className="w-fit text-2xl text-bb-text-muted">
          <p>{emptyText}</p>
        </Panel>
      ) : (
        <MosaicoPanel focused={model.focused!} />
      )}
      {model.alert && (
        <div role="alert" className="rounded bg-bb-surface px-4 py-2 text-bb-danger">
          {model.alert}
        </div>
      )}
    </div>
  )
}
