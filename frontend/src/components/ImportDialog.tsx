import { useEffect, useId, useRef, useState, type JSX, type KeyboardEvent } from 'react'
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

const focusableSelector = [
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'a[href]',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

const inputClass =
  'rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-neutral-100 focus:border-emerald-500 focus:outline-none'

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
  const dialogRef = useRef<HTMLDivElement>(null)
  const titleId = useId()
  const pathId = useId()
  const platformId = useId()

  // Focus starts in the dialog and goes back to the opener on unmount.
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    dialogRef.current?.querySelector<HTMLElement>(focusableSelector)?.focus()
    return () => previous?.focus()
  }, [])

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
      .map(({ c }) => ({ ...c, platform: c.platform || (source === 'file' ? platform.trim() : c.platform) }))
    setBusy(true)
    setError(null)
    try {
      const res = await applyImport(chosen)
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

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
      return
    }
    if (e.key !== 'Tab') {
      return
    }
    // Keep Tab and Shift+Tab inside the dialog (UI-041).
    const dialog = dialogRef.current
    if (!dialog) {
      return
    }
    const items = Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector))
    if (items.length === 0) {
      return
    }
    const first = items[0]
    const last = items[items.length - 1]
    const active = document.activeElement
    if (e.shiftKey && (active === first || !dialog.contains(active))) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (active === last || !dialog.contains(active))) {
      e.preventDefault()
      first.focus()
    }
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 p-4">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onKeyDown={onKeyDown}
        className="flex max-h-full w-full max-w-2xl flex-col gap-3 overflow-y-auto rounded-lg bg-neutral-900 p-5"
      >
        <h2 id={titleId} className="text-lg font-semibold text-neutral-100">
          Importar jogos
        </h2>

        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-sm text-neutral-300">
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
            <div className="flex min-w-0 flex-1 flex-col gap-1 text-sm text-neutral-300">
              <label htmlFor={pathId}>{source === 'file' ? 'Arquivo' : 'Pasta'}</label>
              <div className="flex gap-2">
                <input id={pathId} type="text" readOnly value={path} className={`${inputClass} min-w-0 flex-1`} />
                <button
                  type="button"
                  onClick={() => void choosePath()}
                  className="shrink-0 rounded bg-neutral-800 px-3 py-1.5 text-sm text-neutral-100 hover:bg-neutral-700"
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
            className="rounded bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400"
          >
            Buscar
          </button>
        </div>

        {warnings.length > 0 && (
          <ul role="status" className="flex flex-col gap-1 text-sm text-amber-300">
            {warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        )}

        {error !== null && (
          <p role="alert" className="text-sm text-red-400">
            {error}
          </p>
        )}

        {candidates !== null && candidates.length === 0 && (
          <p className="text-sm text-neutral-400">Nenhum jogo encontrado nesta fonte.</p>
        )}

        {candidates !== null && candidates.length > 0 && (
          <ul className="flex max-h-72 flex-col gap-1 overflow-y-auto rounded border border-neutral-800 p-2">
            {candidates.map((c, i) => (
              <li key={`${c.source}-${c.launchTarget}-${i}`} className="flex items-center gap-2 text-sm text-neutral-200">
                <input
                  id={`${titleId}-c${i}`}
                  type="checkbox"
                  disabled={c.exists || busy}
                  checked={checked.has(i)}
                  onChange={(e) => toggle(i, e.target.checked)}
                  className="h-4 w-4 accent-emerald-600"
                />
                <label htmlFor={`${titleId}-c${i}`} className="flex min-w-0 flex-1 items-center gap-2">
                  <span className="truncate">{c.title}</span>
                  {c.exists && <span className="shrink-0 text-xs text-neutral-400">já existe</span>}
                </label>
              </li>
            ))}
          </ul>
        )}

        {candidates !== null && source === 'file' && (
          <div className="flex items-center gap-2 text-sm text-neutral-300">
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
          <div role="status" className="text-sm text-neutral-300">
            <p>Importados: {result.applied.length}</p>
            {result.skipped.length > 0 && (
              <ul className="mt-1 flex flex-col gap-0.5 text-neutral-400">
                {result.skipped.map((s) => (
                  <li key={s.title}>
                    {s.title}: {skipReasonLabels[s.reason] ?? s.reason}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded px-4 py-1.5 text-sm font-medium text-neutral-300 hover:bg-neutral-800"
          >
            Fechar
          </button>
          <button
            type="button"
            onClick={() => void apply()}
            disabled={busy || selectedCount === 0}
            className="rounded bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400"
          >
            Importar selecionados
          </button>
        </div>
      </div>
    </div>
  )
}

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível importar. Tente de novo.'
}
