import { useId, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { libraryKeys } from '../lib/library'
import {
  acceptedScore,
  mediaKey,
  providerLabels,
  scoreLabel,
  scrapeApply,
  scrapeSearch,
  type ProviderId,
  type ScrapeCandidateView,
} from '../lib/metadata'

const inputClass =
  'rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-neutral-100 focus:border-emerald-500 focus:outline-none'

const providers: ProviderId[] = ['igdb', 'steamgriddb', 'steamstore']

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível buscar agora. Tente de novo.'
}

// MetadataDialog searches a provider for the game and applies one candidate [MID-002, MID-003].
// A candidate at or above the threshold is marked as the likely match; the user still picks it.
// Replacing the fields the user filled is off by default (MID-003).
export function MetadataDialog({ gameId, onClose }: { gameId: number; onClose: () => void }): JSX.Element {
  const qc = useQueryClient()
  const titleId = useId()
  const providerId = useId()
  const [provider, setProvider] = useState<ProviderId>('igdb')
  const [candidates, setCandidates] = useState<ScrapeCandidateView[] | null>(null)
  const [replace, setReplace] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  async function search() {
    setBusy(true)
    setError(null)
    setMessage(null)
    try {
      setCandidates(await scrapeSearch(gameId, provider))
    } catch (e) {
      setCandidates(null)
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  async function apply(c: ScrapeCandidateView) {
    setBusy(true)
    setError(null)
    try {
      const res = await scrapeApply(gameId, provider, c.providerId, replace)
      setMessage(
        res.queued > 0
          ? `Metadados aplicados. ${res.queued} imagem(ns) na fila de download.`
          : 'Metadados aplicados. Este provedor não tem imagens para este jogo.',
      )
      await qc.invalidateQueries({ queryKey: libraryKeys.game(gameId) })
      await qc.invalidateQueries({ queryKey: ['library', 'games'] })
      await qc.invalidateQueries({ queryKey: mediaKey(gameId) })
    } catch (e) {
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex max-h-full w-full max-w-2xl flex-col gap-4 overflow-y-auto rounded-lg bg-neutral-900 p-5"
      >
        <div className="flex items-center justify-between">
          <h2 id={titleId} className="text-lg font-semibold text-neutral-100">
            Buscar metadados e mídia
          </h2>
          <button type="button" onClick={onClose} className="text-sm text-neutral-300 hover:text-neutral-100">
            Fechar
          </button>
        </div>

        <div className="flex flex-wrap items-end gap-3">
          <label htmlFor={providerId} className="flex flex-col gap-1 text-sm text-neutral-300">
            Provedor
            <select
              id={providerId}
              value={provider}
              onChange={(e) => {
                setProvider(e.target.value as ProviderId)
                setCandidates(null)
              }}
              className={inputClass}
            >
              {providers.map((p) => (
                <option key={p} value={p}>
                  {providerLabels[p]}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            disabled={busy}
            onClick={() => void search()}
            className="rounded bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400"
          >
            Buscar
          </button>
          <label className="flex items-center gap-2 text-sm text-neutral-300">
            <input
              type="checkbox"
              checked={replace}
              onChange={(e) => setReplace(e.target.checked)}
              className="h-4 w-4 accent-emerald-600"
            />
            Substituir os campos já preenchidos
          </label>
        </div>

        {error !== null && (
          <p role="alert" className="text-sm text-red-400">
            {error}
          </p>
        )}
        {message !== null && (
          <p role="status" className="text-sm text-emerald-400">
            {message}
          </p>
        )}

        {candidates !== null && candidates.length === 0 && (
          <p className="text-sm text-neutral-400">Nenhum resultado neste provedor.</p>
        )}

        {candidates !== null && candidates.length > 0 && (
          <ul className="flex flex-col gap-2">
            {candidates.map((c) => (
              <li
                key={c.providerId}
                className="flex items-center justify-between gap-3 rounded border border-neutral-800 p-2"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm text-neutral-100">
                    {c.title}
                    {c.year > 0 && <span className="text-neutral-400"> ({c.year})</span>}
                  </p>
                  <p className="truncate text-xs text-neutral-400">
                    {c.platform || 'Plataforma desconhecida'} · correspondência {scoreLabel(c.score)}
                    {c.score >= acceptedScore ? ' · provável' : ''}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void apply(c)}
                  className="shrink-0 rounded bg-neutral-800 px-3 py-1 text-sm text-neutral-100 hover:bg-neutral-700 disabled:text-neutral-500"
                >
                  Aplicar
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
