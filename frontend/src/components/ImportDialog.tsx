import { useId, useState, type JSX } from 'react'
import { Modal } from './Modal'
import {
  applyImport,
  needsPath,
  pickFile,
  pickFolder,
  scanImport,
  skipReasonLabels,
  sourceLabels,
  type ImportApplyResult,
  type ImportCandidate,
  type ImportSource,
} from '../lib/imports'

const inputClass =
  'rounded border border-line bg-raised px-3 py-1.5 text-fg focus:border-accent focus:outline-none'

export type ImportDialogProps = {
  // onImported runs after at least one game was written, so the library reloads (IMP-010).
  onImported: () => void
  onClose: () => void
}

// ImportDialog imports games from another launcher in two steps: it reads the
// source and lists the candidates, then writes only the ones the user checked
// [IMP-001]. Candidates already in the library are shown and cannot be checked
// [IMP-006]. It is modal with focus trapped inside, like DeleteDialog [UI-041].
export function ImportDialog({ onImported, onClose }: ImportDialogProps): JSX.Element {
  const [source, setSource] = useState<ImportSource>('steam')
  const [path, setPath] = useState('')
  const [platform, setPlatform] = useState('PC')
  const [busy, setBusy] = useState(false)
  const [candidates, setCandidates] = useState<ImportCandidate[] | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  const [checked, setChecked] = useState<Set<number>>(new Set())
  const [result, setResult] = useState<ImportApplyResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const titleId = useId()
  const pathId = useId()
  const platformId = useId()

  const checkable = (candidates ?? []).map((c, i) => ({ c, i })).filter(({ c }) => !c.exists)
  const selectedCount = checkable.filter(({ i }) => checked.has(i)).length

  async function choosePath() {
    setError(null)
    try {
      const picked = source === 'file' ? await pickFile() : await pickFolder()
      if (picked !== '') {
        setPath(picked)
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
      const res = await scanImport(source, needsPath(source) ? path : '')
      setCandidates(res.candidates)
      setWarnings(res.warnings)
      setChecked(new Set())
    } catch (e) {
      setCandidates(null)
      setWarnings([])
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  async function apply() {
    const chosen = checkable
      .filter(({ i }) => checked.has(i))
      .map(({ c }) => ({ ...c, platform: c.platform || (source === 'file' || source === 'folder' ? platform.trim() : c.platform) }))
    setBusy(true)
    setError(null)
    try {
      const res = await applyImport(chosen, source === 'launchbox' ? path : '')
      setResult(res)
      setChecked(new Set())
      if (res.applied.length > 0) {
        onImported()
      }
    } catch (e) {
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  function toggle(i: number, on: boolean) {
    setChecked((prev) => {
      const next = new Set(prev)
      if (on) {
        next.add(i)
      } else {
        next.delete(i)
      }
      return next
    })
  }

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
            className="rounded px-4 py-1.5 text-sm font-medium text-muted hover:bg-hover"
          >
            Fechar
          </button>
          <button
            type="button"
            onClick={() => void apply()}
            disabled={busy || selectedCount === 0}
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
              onChange={(e) => {
                setSource(e.target.value as ImportSource)
                setCandidates(null)
                setResult(null)
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
                  className="shrink-0 rounded bg-raised px-3 py-1.5 text-sm text-fg hover:bg-hover"
                >
                  {source === 'file' ? 'Escolher arquivo' : 'Escolher pasta'}
                </button>
              </div>
            </div>
          )}

          <button
            type="button"
            onClick={() => void scan()}
            disabled={busy || (needsPath(source) && path === '')}
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

        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}

        {candidates !== null && candidates.length === 0 && (
          <p className="text-sm text-muted">Nenhum jogo encontrado nesta fonte.</p>
        )}

        {candidates !== null && candidates.length > 0 && (
          <ul className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto rounded border border-line p-2">
            {candidates.map((c, i) => (
              <li key={`${c.source}-${c.launchTarget}-${i}`} className="flex items-center gap-2 text-sm text-fg">
                <input
                  id={`${titleId}-c${i}`}
                  type="checkbox"
                  disabled={c.exists || busy}
                  checked={checked.has(i)}
                  onChange={(e) => toggle(i, e.target.checked)}
                  className="h-4 w-4 accent-[var(--arc-accent)]"
                />
                <label htmlFor={`${titleId}-c${i}`} className="flex min-w-0 flex-1 items-center gap-2">
                  <span className="truncate">{c.title}</span>
                  {c.exists && <span className="shrink-0 text-xs text-muted">já existe</span>}
                </label>
              </li>
            ))}
          </ul>
        )}

        {candidates !== null && (source === 'file' || source === 'folder') && (
          <div className="flex items-center gap-2 text-sm text-muted">
            <label htmlFor={platformId}>Plataforma do arquivo</label>
            <input
              id={platformId}
              type="text"
              value={platform}
              onChange={(e) => setPlatform(e.target.value)}
              className={`${inputClass} w-40`}
            />
          </div>
        )}

        {result !== null && (
          <div role="status" className="text-sm text-muted">
            <p>Importados: {result.applied.length}</p>
            {result.skipped.length > 0 && (
              <ul className="mt-1 flex flex-col gap-0.5 text-muted">
                {result.skipped.map((s) => (
                  <li key={s.title}>
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

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível importar. Tente de novo.'
}
