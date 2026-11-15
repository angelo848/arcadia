import type { JSX } from 'react'
import type { BatchStatusView } from '../lib/metadataBatch'

// Indicator of the metadata batch in the header of the main window (phase 12, T38; MET-066, MET-067).
// It shows nothing without a batch, except the one-time notice of an interrupted search (agent restart).

export type MetadataBatchChipProps = {
  status: BatchStatusView | null
  interrupted: boolean
  onOpenReview: () => void
  onCancel: () => void
  onDiscard: () => void
  onCloseSummary: () => void
}

const interruptedText = 'A busca de metadados foi interrompida. Use Buscar metadados no jogo.'

const chipButton =
  'rounded bg-raised px-3 py-1.5 text-sm text-fg hover:bg-hover'
const primaryButton =
  'rounded bg-accent px-3 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover'

export function MetadataBatchChip(p: MetadataBatchChipProps): JSX.Element | null {
  const { status, interrupted } = p

  if (status === null) {
    if (!interrupted) {
      return null
    }
    return <p className="text-sm text-muted">{interruptedText}</p>
  }

  const { state, done, total } = status

  if (state === 'searching') {
    return (
      <div className="flex items-center gap-2">
        <span className="text-sm text-fg">{`Buscando metadados ${done}/${total}`}</span>
        <button type="button" onClick={p.onCancel} className={chipButton}>
          Cancelar
        </button>
      </div>
    )
  }

  if (state === 'ready' || state === 'cancelled') {
    return (
      <div className="flex items-center gap-2">
        <button type="button" onClick={p.onOpenReview} className={primaryButton}>
          {`Revisar metadados (${total})`}
        </button>
        <button type="button" onClick={p.onDiscard} className={chipButton}>
          Descartar
        </button>
      </div>
    )
  }

  if (state === 'applying') {
    return (
      <div className="flex items-center gap-2">
        <span className="text-sm text-fg">{`Aplicando metadados ${done}/${total}`}</span>
        <button type="button" onClick={p.onCancel} className={chipButton}>
          Cancelar
        </button>
      </div>
    )
  }

  // done: the summary of the apply; Fechar ends the batch [MET-066].
  const result = status.result
  const applied = result?.applied ?? 0
  const imagesQueued = result?.imagesQueued ?? 0
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-fg">
        {`Metadados aplicados em ${applied} jogos; ${imagesQueued} imagens na fila; ${status.failed} falharam`}
      </span>
      <button type="button" onClick={p.onCloseSummary} className={chipButton}>
        Fechar
      </button>
    </div>
  )
}
