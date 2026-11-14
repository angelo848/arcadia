import { useEffect, useState } from 'react'
import type { JSX } from 'react'
import { useRecentGames } from '../lib/library'
import { useCardImages } from '../lib/metadata'
import type { CardImage } from '../lib/metadata'
import { formatLocalDateTime } from '../lib/formatDate'
import { coverPlaceholderUrl } from './CoverImage'

// Thumb is the small cover of a list item, with the placeholder when missing or broken.
function Thumb({ image, title }: { image?: CardImage; title: string }): JSX.Element {
  const [failed, setFailed] = useState(false)
  const url = image?.url
  useEffect(() => setFailed(false), [url])
  const real = !!image && !failed
  return (
    <div className="aspect-[2/3] w-full overflow-hidden rounded bg-raised">
      <img
        src={real ? image.url : coverPlaceholderUrl}
        alt={`Capa de ${title}`}
        loading="lazy"
        data-fit={real ? image.fit : undefined}
        onError={() => setFailed(true)}
        className={`h-full w-full ${real && image.fit === 'cover' ? 'object-cover' : 'object-contain'}`}
      />
    </div>
  )
}

// PanelEmpty is the details panel with no game in focus [UIX-024]: an instruction and the
// last played games. Selecting one only focuses it; it never launches.
export function PanelEmpty({ onSelect }: { onSelect: (id: number) => void }): JSX.Element {
  const recent = useRecentGames()
  // A game that was never played has no last time: it is not a recent game.
  const games = (recent.data ?? []).filter((g) => g.lastPlayedAt !== '')
  const images = useCardImages(games.map((g) => g.id))

  return (
    <div className="flex flex-col gap-4 p-4 text-fg">
      <div>
        <h2 className="text-lg font-semibold">Nenhum jogo selecionado</h2>
        <p className="text-sm text-muted">Selecione um jogo na grade para ver os detalhes.</p>
      </div>
      {games.length > 0 && (
        <section>
          <h3 className="mb-2 text-sm font-semibold text-muted">Jogados recentemente</h3>
          <ul className="flex flex-col gap-2">
            {games.map((g) => (
              <li key={g.id}>
                <button
                  type="button"
                  data-bigbox-item
                  onClick={() => onSelect(g.id)}
                  className="flex w-full items-center gap-3 rounded bg-surface p-2 text-left hover:bg-hover"
                >
                  <span className="w-12 shrink-0">
                    <Thumb image={images[g.id]} title={g.title} />
                  </span>
                  <span className="flex min-w-0 flex-col">
                    <span className="truncate">{g.title}</span>
                    {g.lastPlayedAt && (
                      <span className="text-xs text-muted">{formatLocalDateTime(g.lastPlayedAt)}</span>
                    )}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
