import type { BigBoxThemeProps } from '../types'
import { FaixaInfo } from './FaixaInfo'
import { FaixaStrip } from './FaixaStrip'

// Faixa theme: tab bar on top, cover strip, information block on the left [BBX-006..BBX-021].
export function Faixa({ model, details, onPick, onPlay, onDetails }: BigBoxThemeProps) {
  if (model.screen === 'details') return <div className="relative flex h-full flex-col text-bb-text">{details}</div>
  const empty = model.total === 0 || !model.focused
  const emptyText = model.tab === 0 ? 'Nenhum jogo na biblioteca' : 'Nenhum jogo nesta plataforma'
  return (
    <div className="relative flex h-full flex-col text-bb-text">
      <FaixaStrip tabs={model.tabs} tab={model.tab} games={model.games} focus={model.focus} onPick={onPick} onPlay={onPlay} />
      {empty ? (
        <p className="px-8 py-10 text-2xl text-bb-text-muted">{emptyText}</p>
      ) : (
        <FaixaInfo
          focused={model.focused!}
          activeTab={model.tabs[model.tab]?.label ?? ''}
          onPlay={() => onPlay(model.focus)}
          onDetails={onDetails}
        />
      )}
      {model.alert && (
        <div role="alert" className="mx-8 mt-4 rounded bg-bb-surface px-4 py-2 text-bb-danger">
          {model.alert}
        </div>
      )}
    </div>
  )
}
