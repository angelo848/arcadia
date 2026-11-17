import { useId, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { libraryGamesKey, libraryKeys, useGame, type GameDetailView } from '../lib/library'
import { Modal } from './Modal'
import { MetadataOptions } from './MetadataOptions'
import {
  applyResultLabel,
  mediaKey,
  metadataApply,
  metadataSearch,
  type MetadataCandidateView,
  type MetadataSearchView,
} from '../lib/metadata'

const inputClass =
  'rounded border border-line bg-raised px-3 py-1.5 text-fg focus:border-accent'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível buscar agora. Tente de novo.'
}

// MetadataDialog searches every provider at once for the game and applies one option [MET-040..044].
// Title, platform and year come from the game cache; the search name is editable and starts with the title.
export function MetadataDialog({ gameId, onClose }: { gameId: number; onClose: () => void }): JSX.Element {
  const game = useGame(gameId)

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
      {game.data ? (
        <MetadataForm key={gameId} gameId={gameId} game={game.data} onApplied={onClose} />
      ) : (
        <p className="text-sm text-muted">{game.isError ? messageOf(game.error) : 'Carregando jogo…'}</p>
      )}
    </Modal>
  )
}

// onApplied runs after the chosen option was applied: the window closes.
type FormProps = { gameId: number; game: GameDetailView; onApplied: () => void }

// MetadataForm holds the search state; it mounts once the game is known, so the name starts with its title.
function MetadataForm({ gameId, game, onApplied }: FormProps): JSX.Element {
  const qc = useQueryClient()
  const nameId = useId()
  const replaceId = useId()
  const [name, setName] = useState(game.title)
  const [result, setResult] = useState<MetadataSearchView | null>(null)
  const [selected, setSelected] = useState<number | null>(null)
  const [replace, setReplace] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  async function search() {
    setBusy(true)
    setError(null)
    setMessage(null)
    try {
      const res = await metadataSearch(name, game.platform, game.year ?? 0)
      setResult(res)
      setSelected(null)
    } catch (e) {
      setResult(null)
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  async function apply(c: MetadataCandidateView) {
    setBusy(true)
    setError(null)
    setMessage(null)
    try {
      const res = await metadataApply(gameId, c.refs, replace)
      setMessage(applyResultLabel(res.queued, res.skippedByLimit))
      await qc.invalidateQueries({ queryKey: libraryKeys.game(gameId) })
      await qc.invalidateQueries({ queryKey: libraryGamesKey })
      await qc.invalidateQueries({ queryKey: mediaKey(gameId) })
      onApplied()
    } catch (e) {
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end gap-3">
        <label htmlFor={nameId} className="flex min-w-0 flex-1 flex-col gap-1 text-sm text-muted">
          Nome para busca
          <input
            id={nameId}
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={`${inputClass} w-full`}
          />
        </label>
        <button
          type="button"
          disabled={busy}
          onClick={() => void search()}
          className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
        >
          Obter dados
        </button>
      </div>

      <label htmlFor={replaceId} className="flex items-center gap-2 text-sm text-muted">
        <input
          id={replaceId}
          type="checkbox"
          checked={replace}
          onChange={(e) => setReplace(e.target.checked)}
          className="h-4 w-4 accent-[var(--arc-accent)]"
        />
        Substituir os campos já preenchidos
      </label>

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

      {result !== null && (
        <MetadataOptions
          candidates={result.candidates}
          confidence={result.confidence}
          warnings={result.warnings}
          selected={selected}
          onSelect={setSelected}
          renderAction={(c) => (
            <button
              type="button"
              disabled={busy}
              onClick={() => void apply(c)}
              className="shrink-0 rounded bg-raised px-3 py-1 text-sm text-fg hover:bg-hover disabled:text-muted"
            >
              Aplicar
            </button>
          )}
        />
      )}
    </div>
  )
}
