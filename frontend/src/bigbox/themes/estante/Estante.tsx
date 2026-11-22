import { Panel } from '../../Panel'
import { ESTANTE_COLUMNS } from './capacity'
import type { BigBoxThemeProps } from '../types'
import { EstanteGrid } from './EstanteGrid'
import { EstanteHeader } from './EstanteHeader'
import { EstanteHero } from './EstanteHero'

// Estante theme: highlight of the focused game, fixed library header and a 5 column grid of covers [ETN-001..ETN-009].
export function Estante({ model, details, onPick, onPlay, onDetails }: BigBoxThemeProps) {
  if (model.screen === 'details') return <div className="relative flex h-full min-h-0 flex-col text-bb-text">{details}</div>
  const ready = !model.loading && model.total > 0 && model.focused !== null
  const heroOpen = ready && Math.floor(model.focus / ESTANTE_COLUMNS) === 0
  return (
    <div className="relative flex h-full min-h-0 flex-col gap-4 text-bb-text">
      <div
        data-testid="est-hero-slot"
        className="shrink-0 overflow-hidden transition-[flex-basis,opacity] duration-300 ease-out"
        style={{ flexBasis: heroOpen ? '45%' : '0%', opacity: heroOpen ? 1 : 0 }}
      >
        {heroOpen && <EstanteHero model={model} onPlay={onPlay} onDetails={onDetails} />}
      </div>
      <EstanteHeader model={model} />
      {model.loading ? (
        <Panel className="w-fit text-2xl text-bb-text-muted">
          <p>Carregando jogos…</p>
        </Panel>
      ) : !ready ? (
        <Panel className="w-fit text-2xl text-bb-text-muted">
          <p>Nenhum jogo nesta aba</p>
        </Panel>
      ) : (
        <EstanteGrid model={model} onPick={onPick} onPlay={onPlay} />
      )}
      {model.alert && (
        <div role="alert" className="shrink-0 rounded bg-bb-surface px-4 py-2 text-bb-danger">
          {model.alert}
        </div>
      )}
    </div>
  )
}
