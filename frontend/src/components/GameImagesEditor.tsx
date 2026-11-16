import { useCallback, useEffect, useState, type JSX } from 'react'
import { addGameImage, manualImageKinds, pickGameImage, removeGameImage, type ManualImageKind } from '../lib/manualEdit'
import { fetchMedia, type MediaItemView } from '../lib/metadata'

// GameImagesEditor shows the cover, the screenshots and the backgrounds of a game and lets the user add one from
// a file or remove one (EDM-001..EDM-004). Each action is written at once; it does not wait for "Salvar". The
// list is read here, on opening and after each action; the other screens follow the media:changed event.
export function GameImagesEditor({ gameId }: { gameId: number }): JSX.Element {
  const [items, setItems] = useState<MediaItemView[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setItems(await fetchMedia(gameId))
    } catch {
      setItems([])
    }
  }, [gameId])

  useEffect(() => {
    void load()
  }, [load])

  async function run(action: () => Promise<void>) {
    setBusy(true)
    setError(null)
    try {
      await action()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Não foi possível alterar a imagem.')
    } finally {
      setBusy(false)
    }
  }

  function add(kind: ManualImageKind) {
    void run(async () => {
      const path = await pickGameImage()
      if (path !== '') {
        await addGameImage(gameId, kind, path)
      }
    })
  }

  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-1 text-sm font-medium text-muted">Imagens</legend>
      <span className="text-xs text-muted">Adicionar e remover imagens vale na hora, sem precisar salvar.</span>
      {manualImageKinds.map(({ kind, label, limit }) => {
        const mine = items.filter((m) => m.kind === kind)
        const full = kind !== 'cover' && mine.length >= limit
        return (
          <div key={kind} role="group" aria-label={label} className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm text-fg">
                {label} <span className="text-xs text-muted">({mine.length} de {limit})</span>
              </span>
              <button
                type="button"
                disabled={busy || full}
                onClick={() => add(kind)}
                className="shrink-0 rounded bg-raised px-3 py-1 text-sm text-fg hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
              >
                {kind === 'cover' && mine.length > 0 ? 'Trocar...' : 'Adicionar...'}
              </button>
            </div>
            {mine.length > 0 && (
              <ul className="flex flex-wrap gap-2">
                {mine.map((m, i) => (
                  <li key={m.id} className="flex flex-col items-center gap-1">
                    <img
                      src={m.thumbUrl || m.url}
                      alt=""
                      className="h-20 w-28 rounded border border-line bg-raised object-contain"
                      draggable={false}
                    />
                    <button
                      type="button"
                      disabled={busy}
                      aria-label={`Remover ${label} ${i + 1}`}
                      onClick={() => void run(() => removeGameImage(m.id))}
                      className="rounded px-2 py-0.5 text-xs text-danger hover:bg-hover disabled:opacity-50"
                    >
                      Remover
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )
      })}
      {error !== null && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
    </fieldset>
  )
}
