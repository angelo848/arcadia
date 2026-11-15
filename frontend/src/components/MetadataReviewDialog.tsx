import { memo, useCallback, useEffect, useMemo, useRef, useState, type JSX } from 'react'
import { CoverImage } from './CoverImage'
import { Modal } from './Modal'
import { scoreLabel } from '../lib/metadata'
import {
  buildReviewRows,
  chooseCandidate,
  loadBatchRows,
  reviewCounts,
  toChoices,
  type BatchChoice,
  type BatchRowView,
  type BatchStatusView,
  type ReviewRow,
} from '../lib/metadataBatch'

// Confirmation window "Confirmar metadados" (phase 12, T37; MET-060..064, MET-066, MET-068, MET-069).
// The rows are read from the agent in pages of 100 (MET-057). Colors come only from the --arc-* tokens (UIX-006).

export type MetadataReviewDialogProps = {
  batch: BatchStatusView
  onClose: () => void
  onApply: (choices: BatchChoice[]) => Promise<void>
  onDiscard: () => Promise<void>
}

type Pending = 'apply' | 'discard' | null

type Progress = { loaded: number; total: number }

const DASH = '—'
const UNKNOWN_PLATFORM = 'Plataforma desconhecida'

const cellClass = 'border-b border-line px-2 py-1.5 align-middle'
const headerClass = 'sticky top-0 z-10 border-b border-line bg-surface px-2 py-2 text-left text-xs font-semibold text-muted'

// situacaoOf is the status column: the user's choice wins, otherwise the agent's confidence [MET-060, MET-062].
function situacaoOf(line: ReviewRow): string {
  if (line.byUser) {
    return 'escolhido por você'
  }
  switch (line.row.confidence) {
    case 'match':
      return 'provável'
    case 'doubt':
      return 'em dúvida'
    default:
      return 'nenhum'
  }
}

// optionLabel is the text of one alternative: "título (ano), plataforma" with the year omitted when unknown [MET-062].
function optionLabel(c: BatchRowView['candidates'][number]): string {
  const head = c.year > 0 ? `${c.title} (${c.year})` : c.title
  return `${head}, ${c.platform || UNKNOWN_PLATFORM}`
}

type LineProps = {
  line: ReviewRow
  index: number
  busy: boolean
  onToggle: (i: number, on: boolean) => void
  onChoose: (i: number, choice: number) => void
}

// ReviewLine is one row of the table. Memoized with stable callbacks: changing one row re-renders only that row [MET-060].
const ReviewLine = memo(function ReviewLine({ line, index, busy, onToggle, onChoose }: LineProps): JSX.Element {
  const { row, choice, checked } = line
  const hasItems = row.candidates.length > 0
  const picked = choice >= 0 ? row.candidates[choice] : undefined
  const selectable = hasItems && choice >= 0

  return (
    <tr>
      <td className={cellClass}>
        <input
          type="checkbox"
          aria-label={`Selecionar ${row.title}`}
          checked={checked}
          disabled={busy || !selectable}
          onChange={(e) => onToggle(index, e.target.checked)}
        />
      </td>
      <td className={cellClass}>
        <div className="w-12">
          {/* Keyed by the image: a new candidate remounts the cover, so no extra commit runs in CoverImage's effect [MET-068]. */}
          <CoverImage key={picked?.thumbUrl ?? ''} src={picked?.thumbUrl} referrerPolicy="no-referrer" />
        </div>
      </td>
      <td className={cellClass}>
        <div className="flex flex-col">
          <span className="text-fg">{row.title}</span>
          <span className="text-xs text-muted">{row.platform || UNKNOWN_PLATFORM}</span>
        </div>
      </td>
      <td className={cellClass}>
        {picked ? (
          <div className="flex flex-col">
            <span className="text-fg">{picked.title}</span>
            {picked.year > 0 && <span className="text-xs text-muted">({picked.year})</span>}
            <span className="text-xs text-muted">{picked.platform || UNKNOWN_PLATFORM}</span>
            <span className="text-xs text-muted">{scoreLabel(picked.score)}</span>
          </div>
        ) : (
          <span className="text-muted">{DASH}</span>
        )}
      </td>
      <td className={cellClass}>
        <span className="text-sm text-fg">{situacaoOf(line)}</span>
      </td>
      <td className={cellClass}>
        {hasItems ? (
          <select
            aria-label={`Alternativas de ${row.title}`}
            value={String(choice)}
            disabled={busy}
            onChange={(e) => onChoose(index, Number(e.target.value))}
            className="w-full min-w-40 rounded border border-line bg-app px-2 py-1 text-sm text-fg disabled:opacity-50"
          >
            {row.candidates.map((c, i) => (
              <option key={i} value={String(i)}>
                {optionLabel(c)}
              </option>
            ))}
            <option value="-1">Nenhum</option>
          </select>
        ) : (
          <span className="text-muted">{DASH}</span>
        )}
      </td>
    </tr>
  )
})

// MetadataReviewDialog lists the batch rows, lets the user mark them and applies the marked ones [MET-060..064].
// While an apply is in flight, or the batch is "applying", the dialog is busy: no change, Esc ignored [MET-063, MET-069].
export function MetadataReviewDialog({ batch, onClose, onApply, onDiscard }: MetadataReviewDialogProps): JSX.Element {
  const batchId = batch.batchId
  const [rows, setRows] = useState<ReviewRow[] | null>(null)
  const [progress, setProgress] = useState<Progress | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [pending, setPending] = useState<Pending>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const headerRef = useRef<HTMLInputElement>(null)

  // Reads every page of the batch; the progress is shown while it runs [MET-060, MET-057].
  useEffect(() => {
    let alive = true
    loadBatchRows(batchId, (loaded, total) => {
      if (alive) {
        setProgress({ loaded, total })
      }
    })
      .then((all) => {
        if (alive) {
          setRows(buildReviewRows(all))
        }
      })
      .catch((err: unknown) => {
        if (alive) {
          setLoadError(err instanceof Error ? err.message : 'Não foi possível ler os resultados.')
        }
      })
    return () => {
      alive = false
    }
  }, [batchId])

  const counts = useMemo(() => reviewCounts(rows ?? []), [rows])
  const allChecked = counts.selectable > 0 && counts.checked === counts.selectable
  const partial = counts.checked > 0 && !allChecked
  const busy = pending !== null || batch.state === 'applying'

  useEffect(() => {
    if (headerRef.current) {
      headerRef.current.indeterminate = partial
    }
  }, [partial])

  // The callbacks are stable; a row object changes only when that row changes [MET-060].
  const onToggle = useCallback((i: number, on: boolean) => {
    setRows((prev) => prev && prev.map((r, k) => (k === i && r.checked !== on ? { ...r, checked: on } : r)))
  }, [])

  const onChoose = useCallback((i: number, choice: number) => {
    setRows((prev) => prev && prev.map((r, k) => (k === i ? chooseCandidate(r, choice) : r)))
  }, [])

  const toggleAll = useCallback((on: boolean) => {
    setRows(
      (prev) =>
        prev &&
        prev.map((r) => {
          const want = on && r.row.candidates.length > 0 && r.choice >= 0
          return r.checked === want ? r : { ...r, checked: want }
        }),
    )
  }, [])

  async function apply(): Promise<void> {
    if (rows === null) {
      return
    }
    setPending('apply')
    setActionError(null)
    try {
      await onApply(toChoices(rows))
      onClose()
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Não foi possível aplicar os metadados.')
    } finally {
      setPending(null)
    }
  }

  async function discard(): Promise<void> {
    setPending('discard')
    setActionError(null)
    try {
      await onDiscard()
      onClose()
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Não foi possível descartar a busca.')
    } finally {
      setPending(null)
    }
  }

  function statusText(): string {
    if (rows === null) {
      return progress ? `${progress.loaded} de ${progress.total} jogos` : 'Carregando jogos…'
    }
    const base = `${counts.checked} de ${counts.selectable} jogos selecionados`
    return counts.none > 0 ? `${base} (${counts.none} sem correspondência)` : base
  }

  const footer = (
    <>
      <button
        type="button"
        onClick={() => void discard()}
        disabled={busy}
        className="rounded bg-raised px-3 py-1.5 text-sm text-fg hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
      >
        Descartar tudo
      </button>
      <button
        type="button"
        onClick={() => void apply()}
        disabled={busy || rows === null || counts.checked === 0}
        className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
      >
        Aplicar selecionados
      </button>
    </>
  )

  return (
    <Modal size="large" title="Confirmar metadados" onClose={onClose} closeDisabled={busy} footer={footer}>
      <div className="flex shrink-0 flex-col gap-1">
        <p className="text-sm text-muted">{statusText()}</p>
        {loadError !== null && (
          <p role="alert" className="text-sm text-danger">
            {loadError}
          </p>
        )}
        {actionError !== null && (
          <p role="alert" className="text-sm text-danger">
            {actionError}
          </p>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto rounded border border-line">
        <table className="w-full border-collapse text-sm text-fg">
          <thead>
            <tr>
              <th scope="col" className={headerClass}>
                <input
                  ref={headerRef}
                  type="checkbox"
                  aria-label="Selecionar todos"
                  checked={allChecked}
                  disabled={busy || counts.selectable === 0}
                  onChange={() => toggleAll(!allChecked)}
                />
              </th>
              <th scope="col" className={headerClass}>
                Capa
              </th>
              <th scope="col" className={headerClass}>
                Jogo
              </th>
              <th scope="col" className={headerClass}>
                Encontrado
              </th>
              <th scope="col" className={headerClass}>
                Situação
              </th>
              <th scope="col" className={headerClass}>
                Alternativas
              </th>
            </tr>
          </thead>
          <tbody>
            {(rows ?? []).map((line, i) => (
              <ReviewLine
                key={line.row.gameId}
                line={line}
                index={i}
                busy={busy}
                onToggle={onToggle}
                onChoose={onChoose}
              />
            ))}
          </tbody>
        </table>
      </div>
    </Modal>
  )
}
