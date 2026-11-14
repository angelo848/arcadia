import { useId, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { libraryKeys } from '../lib/library'
import { Modal } from './Modal'
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
  'rounded border border-line bg-raised px-3 py-1.5 text-fg focus:border-accent'

const providers: ProviderId[] = ['igdb', 'steamgriddb', 'steamstore']

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível buscar agora. Tente de novo.'
}

// MetadataDialog searches a provider for the game and applies one candidate [MID-002, MID-003].
// A candidate at or above the threshold is marked as the likely match; the user still picks it.
// Replacing the fields the user filled is off by default (MID-003).
export function MetadataDialog({ gameId, onClose }: { gameId: number; onClose: () => void }): JSX.Element {
  const qc = useQueryClient()
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
    <Modal
      size="medium"
      title="Buscar metadados e mídia"
      onClose={onClose}
      footer={
        <button type="button" onClick={onClose} className="rounded px-3 py-1.5 text-sm text-muted hover:bg-hover">
          Fechar
        </button>
      }
    >

        <div className="flex flex-wrap items-end gap-3">
          <label htmlFor={providerId} className="flex flex-col gap-1 text-sm text-muted">
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
            className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
          >
            Buscar
          </button>
          <label className="flex items-center gap-2 text-sm text-muted">
            <input
              type="checkbox"
              checked={replace}
              onChange={(e) => setReplace(e.target.checked)}
              className="h-4 w-4 accent-[var(--arc-accent)]"
            />
            Substituir os campos já preenchidos
          </label>
        </div>

        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
        {message !== null && (
          <p role="status" className="text-sm text-accent">
            {message}
          </p>
        )}

        {candidates !== null && candidates.length === 0 && (
          <p className="text-sm text-muted">Nenhum resultado neste provedor.</p>
        )}

        {candidates !== null && candidates.length > 0 && (
          <ul className="flex flex-col gap-2">
            {candidates.map((c) => (
              <li
                key={c.providerId}
                className="flex items-center justify-between gap-3 rounded border border-line p-2"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm text-fg">
                    {c.title}
                    {c.year > 0 && <span className="text-muted"> ({c.year})</span>}
                  </p>
                  <p className="truncate text-xs text-muted">
                    {c.platform || 'Plataforma desconhecida'} · correspondência {scoreLabel(c.score)}
                    {c.score >= acceptedScore ? ' · provável' : ''}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void apply(c)}
                  className="shrink-0 rounded bg-raised px-3 py-1 text-sm text-fg hover:bg-hover disabled:text-muted"
                >
                  Aplicar
                </button>
              </li>
            ))}
          </ul>
        )}
    </Modal>
  )
}
