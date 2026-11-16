import { Panel } from '../../Panel'
import type { BigBoxThemeProps } from '../types'
import { FaixaInfo } from './FaixaInfo'
import { FaixaStrip } from './FaixaStrip'

// Faixa theme: tab bar on top, cover strip, information block on the left [BBX-006..BBX-021, BBX-037..BBX-039].
export function Faixa({ model, details, onPick, onPlay, onDetails }: BigBoxThemeProps) {
  if (model.screen === 'details') return <div className="relative flex h-full min-h-0 flex-col text-bb-text">{details}</div>
  const empty = model.total === 0 || !model.focused
  const emptyText = model.tab === 0 ? 'Nenhum jogo na biblioteca' : 'Nenhum jogo nesta plataforma'
  return (
    <div className="relative flex h-full min-h-0 flex-col gap-3 text-bb-text">
      <FaixaStrip
        tabs={model.tabs}
        tab={model.tab}
        games={model.games}
        focus={model.focus}
        stripStart={model.stripStart}
        counter={model.counter}
        onPick={onPick}
        onPlay={onPlay}
      />
      {model.loading ? (
        <Panel className="w-fit text-2xl text-bb-text-muted">
          <p>Carregando jogos…</p>
        </Panel>
      ) : empty ? (
        <Panel className="w-fit text-2xl text-bb-text-muted">
          <p>{emptyText}</p>
        </Panel>
      ) : (
        <FaixaInfo
          focused={model.focused!}
          activeTab={model.tabs[model.tab]?.label ?? ''}
          onPlay={() => onPlay(model.focus)}
          onDetails={onDetails}
        />
      )}
      {model.alert && (
        <div role="alert" className="rounded bg-bb-surface px-4 py-2 text-bb-danger">
          {model.alert}
        </div>
      )}
    </div>
  )
}
