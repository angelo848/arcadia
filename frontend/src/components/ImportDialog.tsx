import { useCallback, useId, useMemo, useRef, useState, useEffect, type JSX } from 'react'
import { Modal } from './Modal'
import { ImportReviewTable } from './ImportReviewTable'
import { useCatalog } from '../lib/library'
import {
  applyImportInLots,
  buildReviewRows,
  defaultPlatformName,
  markImported,
  needsPath,
  pickFile,
  pickFolder,
  repeatedFlags,
  scanImport,
  selectionCounts,
  skipReasonLabels,
  sourceLabels,
  titleProblem,
  type ImportApplyResult,
  type ImportCandidate,
  type ImportSource,
  type ReviewRow,
} from '../lib/imports'

const inputClass =
  'rounded border border-line bg-raised px-3 py-1.5 text-fg focus:border-accent'

export type ImportDialogProps = {
  // onImported runs after at least one game was written, so the library reloads (IMP-010).
  onImported: () => void
  onClose: () => void
}

// ImportDialog imports games from another launcher in two steps: it reads the
// source and shows the review rows, then writes only the ones the user checked
// [IMP-001, IMV-040]. Candidates already in the library are shown and cannot be
// checked [IMP-006]. It is modal with focus trapped inside, like DeleteDialog [UI-041].
export function ImportDialog({ onImported, onClose }: ImportDialogProps): JSX.Element {
  const [source, setSource] = useState<ImportSource>('steam')
  const [path, setPath] = useState('')
  // platformChoice is the user's pick; null means the default (IMV-061).
  const [platformChoice, setPlatformChoice] = useState<string | null>(null)
  const [titleFrom, setTitleFrom] = useState<'file' | 'folder'>('file')
  const [subfolders, setSubfolders] = useState(true)
  const [busy, setBusy] = useState(false)
  const [rows, setRows] = useState<ReviewRow[] | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  const [result, setResult] = useState<ImportApplyResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [progress, setProgress] = useState<{ sent: number; total: number } | null>(null)
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  const pathId = useId()
  const platformId = useId()
  const titleFromId = useId()
  const subfoldersId = useId()
  const noPlatformId = useId()

  const catalog = useCatalog()
  const platforms = useMemo(
    () => [...(catalog.data?.platforms ?? [])].sort((a, b) => a.name.localeCompare(b.name, 'pt-BR')),
    [catalog.data],
  )
  // The "no platform" message waits for the catalog to load (IMV-062).
  const noPlatformMessage = !catalog.isPending && platforms.length === 0
  const platform = platformChoice ?? defaultPlatformName(platforms)

  const fileLike = source === 'file' || source === 'folder'
  const noPlatform = fileLike && platform === null
  const repeated = useMemo(
    () => (rows === null ? [] : repeatedFlags(rows.map((r) => ({ title: r.title, exists: r.candidate.exists })))),
    [rows],
  )
  const counts = rows === null ? null : selectionCounts(rows)
  const hasInvalid = rows?.some((r) => r.checked && titleProblem(r.title) !== null) ?? false

  // resetList drops the review list: the scan that built it no longer matches the form (IMV-049).
  function resetList() {
    setRows(null)
    setWarnings([])
    setResult(null)
  }

  async function choosePath() {
    setError(null)
    try {
      const picked = source === 'file' ? await pickFile() : await pickFolder()
      if (picked !== '' && picked !== path) {
        setPath(picked)
        resetList()
      }
    } catch (e) {
      setError(messageOf(e))
    }
  }

  async function scan() {
    setBusy(true)
    setError(null)
    setResult(null)
    try {
      const res = await scanImport(source, needsPath(source) ? path : '', {
        platform: fileLike ? (platform ?? '') : '',
        titleFrom: source === 'folder' ? titleFrom : 'file',
        recursive: source === 'folder' ? subfolders : true,
      })
      setRows(buildReviewRows(res.candidates))
      setWarnings(res.warnings)
    } catch (e) {
      setRows(null)
      setWarnings([])
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  async function apply() {
    if (rows === null) {
      return
    }
    // Checked rows, in table order; file and folder rows take the chosen platform (IMV-063).
    const chosen = rows.filter((r) => r.checked)
    const candidates: ImportCandidate[] = chosen.map((r) => ({
      ...r.candidate,
      title: r.title.trim(),
      platform: fileLike ? (platform ?? '') : r.candidate.platform,
    }))
    setBusy(true)
    setError(null)
    setResult(null)
    const out = await applyImportInLots(candidates, source === 'launchbox' ? path : '', (sent, total) => {
      if (mounted.current) {
        setProgress({ sent, total })
      }
    })
    // Rows not sent because a lot failed stay checked (IMV-050).
    const sent = new Set(chosen.slice(0, chosen.length - out.notSent))
    if (out.result.applied.length > 0) {
      onImported()
    }
    if (!mounted.current) {
      return
    }
    setRows((prev) => markRows(prev, sent, out.result.applied))
    setResult(out.result)
    if (out.error) {
      setError(`${messageOf(out.error)}. ${out.notSent} jogos não foram enviados.`)
    }
    setProgress(null)
    setBusy(false)
  }

  const onToggle = useCallback((i: number, on: boolean) => {
    setRows((prev) => (prev === null ? prev : prev.map((r, idx) => (idx === i ? { ...r, checked: on } : r))))
  }, [])

  const onToggleAll = useCallback((on: boolean) => {
    setRows((prev) => (prev === null ? prev : prev.map((r) => (r.candidate.exists ? r : { ...r, checked: on }))))
  }, [])

  const onTitle = useCallback((i: number, value: string) => {
    setRows((prev) => (prev === null ? prev : prev.map((r, idx) => (idx === i ? { ...r, title: value } : r))))
  }, [])

  return (
    <Modal
      size="large"
      title="Importar jogos"
      onClose={onClose}
      closeDisabled={busy}
      footer={
        <>
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="rounded px-4 py-1.5 text-sm font-medium text-muted hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
          >
            Fechar
          </button>
          <button
            type="button"
            onClick={() => void apply()}
            disabled={busy || counts === null || counts.checked === 0 || hasInvalid || noPlatform}
            className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
          >
            Importar selecionados
          </button>
        </>
      }
    >
      <div className="flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1 text-sm text-muted">
          Fonte
          <select
            value={source}
            disabled={busy}
            onChange={(e) => {
              const next = e.target.value as ImportSource
              // A path picked for Arquivo is not a folder for Pasta, and the other way round (IMV-049).
              if ((source === 'file' && next === 'folder') || (source === 'folder' && next === 'file')) {
                setPath('')
              }
              setSource(next)
              resetList()
            }}
            className={inputClass}
          >
            {(Object.keys(sourceLabels) as ImportSource[]).map((s) => (
              <option key={s} value={s}>
                {sourceLabels[s]}
              </option>
            ))}
          </select>
        </label>

        {needsPath(source) && (
          <div className="flex min-w-0 flex-1 flex-col gap-1 text-sm text-muted">
            <label htmlFor={pathId}>{source === 'file' ? 'Arquivo' : 'Pasta'}</label>
            <div className="flex gap-2">
              <input id={pathId} type="text" readOnly value={path} className={`${inputClass} min-w-0 flex-1`} />
              <button
                type="button"
                onClick={() => void choosePath()}
                disabled={busy}
                className="shrink-0 rounded bg-raised px-3 py-1.5 text-sm text-fg hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
              >
                {source === 'file' ? 'Escolher arquivo' : 'Escolher pasta'}
              </button>
            </div>
          </div>
        )}

        {fileLike && (
          <div className="flex flex-col gap-1 text-sm text-muted">
            <label htmlFor={platformId}>Plataforma do arquivo</label>
            <select
              id={platformId}
              value={platform ?? ''}
              aria-describedby={noPlatformMessage ? noPlatformId : undefined}
              disabled={busy || platforms.length === 0}
              onChange={(e) => {
                setPlatformChoice(e.target.value)
                resetList()
              }}
              className={`${inputClass} w-48`}
            >
              {platforms.map((p) => (
                <option key={p.id} value={p.name}>
                  {p.name}
                </option>
              ))}
            </select>
            {noPlatformMessage && (
              <span id={noPlatformId} className="text-xs text-warning">
                Nenhuma plataforma. Crie uma em Plataformas.
              </span>
            )}
          </div>
        )}

        {source === 'folder' && (
          <>
            <label htmlFor={titleFromId} className="flex flex-col gap-1 text-sm text-muted">
              Título a partir de
              <select
                id={titleFromId}
                value={titleFrom}
                disabled={busy}
                onChange={(e) => {
                  setTitleFrom(e.target.value as 'file' | 'folder')
                  resetList()
                }}
                className={inputClass}
              >
                <option value="file">Nome do arquivo</option>
                <option value="folder">Nome da pasta do jogo</option>
              </select>
            </label>
            <label htmlFor={subfoldersId} className="flex items-center gap-2 pb-1.5 text-sm text-muted">
              <input
                id={subfoldersId}
                type="checkbox"
                checked={subfolders}
                disabled={busy}
                onChange={(e) => {
                  setSubfolders(e.target.checked)
                  resetList()
                }}
                className="h-4 w-4 accent-[var(--arc-accent)]"
              />
              Ler subpastas
            </label>
          </>
        )}

        <button
          type="button"
          onClick={() => void scan()}
          disabled={busy || (needsPath(source) && path === '') || noPlatform}
          className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
        >
          Buscar
        </button>
      </div>

      {warnings.length > 0 && (
        <ul role="status" className="flex flex-col gap-1 text-sm text-warning">
          {warnings.map((w) => (
            <li key={w}>{w}</li>
          ))}
        </ul>
      )}

      {progress !== null && (
        <p role="status" className="text-sm text-muted">
          Importando {progress.sent} de {progress.total}…
        </p>
      )}

      {error !== null && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}

      {rows !== null && rows.length === 0 && <p className="text-sm text-muted">Nenhum jogo encontrado nesta fonte.</p>}

      {rows !== null && rows.length > 0 && counts !== null && (
        <>
          <p className="text-sm text-muted">
            {`${counts.checked} de ${counts.selectable} jogos selecionados${counts.existing > 0 ? ` (${counts.existing} já existem)` : ''}`}
          </p>
          <ImportReviewTable
            rows={rows}
            source={source}
            busy={busy}
            repeated={repeated}
            onToggle={onToggle}
            onToggleAll={onToggleAll}
            onTitle={onTitle}
          />
        </>
      )}

      {result !== null && (
        <div role="status" className="text-sm text-muted">
          <p>Importados: {result.applied.length}</p>
          {result.skipped.length > 0 && (
            <ul className="mt-1 flex flex-col gap-0.5 text-muted">
              {result.skipped.map((s, i) => (
                <li key={i}>
                  {s.title}: {skipReasonLabels[s.reason] ?? s.reason}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Modal>
  )
}

// markRows applies a finished operation to the review rows: the sent rows lose their
// check and take the trimmed title, the applied titles become "já existe" (IMV-050).
function markRows(
  prev: ReviewRow[] | null,
  sent: Set<ReviewRow>,
  applied: { title: string }[],
): ReviewRow[] | null {
  if (prev === null) {
    return prev
  }
  const normalized = prev.map((r) => (sent.has(r) ? { ...r, title: r.title.trim() } : r))
  const sentNow = normalized.filter((_, i) => sent.has(prev[i]))
  return markImported(normalized, sentNow, applied)
}

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível importar. Tente de novo.'
}
