import { useState, type JSX, type ReactNode } from 'react'
import type { GameStatus } from '../lib/filters'
import type { SessionView } from '../lib/games'
import { formatPlayTime } from '../lib/games'
import { useCatalog, useGame } from '../lib/library'
import { playModeLabels } from '../lib/library'
import { formatLocalDateTime, formatUtcDate } from '../lib/formatDate'
import { SessionsList } from './SessionsList'
import { MetadataDialog } from './MetadataDialog'
import { useMedia, type MediaItemView } from '../lib/metadata'

// Labels match the library filter (FilterPanel) and the play mode names of the contract.
const statusLabels: Record<GameStatus, string> = {
  not_played: 'Não jogado',
  playing: 'Jogando',
  completed: 'Concluído',
  abandoned: 'Abandonado',
  backlog: 'Na fila',
}

// onEdit and onRemove are called once per click; the parent opens the edit form (UI-039) and the
// removal confirmation (UI-041).
export type GamePageProps = {
  id: number
  current: SessionView | null
  onBack: () => void
  onEdit?: () => void
  onRemove?: () => void
}

// GamePage shows one game with its data, its session marking and its sessions [UI-037, UI-006].
// It shows no media: no img and no video (phase 7). The session marking is above the title;
// the live clock is the global SessionPanel, so the page does not show a second title.
export function GamePage(props: GamePageProps): JSX.Element {
  const { id, current, onBack, onEdit, onRemove } = props
  const q = useGame(id)
  // The catalog gives the platform defaults of the emulators (EMU-002, EMU-003). Opening a page
  // does not refetch it: writes and the Emuladores dialog reload it when it changes.
  const catalog = useCatalog({ staleTime: Infinity })
  // The images of the game and the metadata search dialog (MID-004, MID-006).
  const media = useMedia(id)
  const [searching, setSearching] = useState(false)

  if (q.isError) {
    return (
      <section className="px-6 py-4">
        <button type="button" onClick={onBack} className="text-sm text-neutral-300 hover:text-neutral-100">
          Voltar
        </button>
        <p role="alert" className="mt-4 text-sm text-red-400">
          {q.error?.message}
        </p>
      </section>
    )
  }

  const g = q.data
  if (g === undefined) {
    return <p className="px-6 py-4 text-sm text-neutral-400">Carregando jogo…</p>
  }

  // Only a session of this game marks the page; a live session shows its state [UI-006].
  const sessionHere = current !== null && current.gameId === g.id ? current.state : null
  // A game run by an emulator with no emulator of its own nor a platform default (EMU-003).
  // Until the catalog answers, no notice is shown.
  const platformDefault = catalog.data?.platforms.find((p) => p.id === g.platformId)?.emulatorId ?? null
  const noEmulator = g.launchType === 'emulator' && g.emulatorId === null && platformDefault === null
  const marking =
    sessionHere === 'running' || sessionHere === 'grace' ? (
      <span className="text-sm font-medium text-emerald-400">Em execução</span>
    ) : sessionHere === 'paused' ? (
      <span className="text-sm font-medium text-sky-400">Pausado</span>
    ) : null

  return (
    <article className="flex flex-col gap-4 px-6 py-4">
      <div className="flex items-center gap-4">
        <button type="button" onClick={onBack} className="text-sm text-neutral-300 hover:text-neutral-100">
          Voltar
        </button>
        {marking}
        {sessionHere !== null && current?.tracking === 'process' && (
          <span className="text-sm text-neutral-400">Rastreio por processo: pausa e processos filhos não entram no tempo</span>
        )}
        {noEmulator && <span className="text-sm text-amber-400">Sem emulador configurado: escolha um padrão em Emuladores ou neste jogo.</span>}
        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            onClick={() => setSearching(true)}
            className="rounded px-3 py-1 text-sm font-medium text-neutral-300 hover:bg-neutral-800"
          >
            Buscar mídia
          </button>
          <button
            type="button"
            onClick={() => onEdit?.()}
            className="rounded px-3 py-1 text-sm font-medium text-neutral-300 hover:bg-neutral-800"
          >
            Editar
          </button>
          <button
            type="button"
            onClick={() => onRemove?.()}
            className="rounded px-3 py-1 text-sm font-medium text-red-400 hover:bg-neutral-800"
          >
            Remover
          </button>
        </div>
      </div>

      <header className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold text-neutral-100">{g.title}</h1>
        <p className="text-sm text-neutral-400">{g.platform}</p>
        <div className="flex items-center gap-3 text-sm">
          <span className="text-neutral-200">{statusLabels[g.status]}</span>
          {g.favorite && <span className="font-medium text-amber-400">Favorito</span>}
        </div>
      </header>

      {g.description !== '' && <p className="text-neutral-300">{g.description}</p>}

      <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        <Field label="Gêneros">
          <ListValue items={g.genres} />
        </Field>
        <Field label="Modos de jogo">
          <ListValue items={g.playModes.map((m) => playModeLabels[m])} />
        </Field>
        <Field label="Desenvolvedora">
          <ListValue items={g.developers} />
        </Field>
        <Field label="Editora">
          <ListValue items={g.publishers} />
        </Field>
        <Field label="Ano">
          <span>{g.year ?? '—'}</span>
        </Field>
        <Field label="Tempo total">
          <span>{formatPlayTime(g.totalPlaySeconds)}</span>
        </Field>
        <Field label="Primeira vez jogado">
          <span>{formatLocalDateTime(g.firstPlayedAt)}</span>
        </Field>
        <Field label="Última vez jogado">
          <span>{formatLocalDateTime(g.lastPlayedAt)}</span>
        </Field>
        {g.completedAt ? (
          <Field label="Concluído em">
            <span>{formatUtcDate(g.completedAt)}</span>
          </Field>
        ) : null}
      </dl>

      <MediaSection items={media.data ?? []} />

      <section className="flex flex-col gap-2">
        <h2 className="text-lg font-medium text-neutral-200">Sessões</h2>
        <SessionsList gameId={g.id} current={null} />
      </section>
      {searching && <MetadataDialog gameId={g.id} onClose={() => setSearching(false)} />}
    </article>
  )
}

// MediaSection shows the images of the game: the cover first, then the rest (MID-006). Nothing is
// shown when the game has no image yet.
function MediaSection({ items }: { items: MediaItemView[] }): JSX.Element | null {
  if (items.length === 0) {
    return null
  }
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-lg font-medium text-neutral-200">Imagens</h2>
      <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {items.map((m) => (
          <li key={m.id} className="flex flex-col gap-1">
            <img src={m.url} alt="" loading="lazy" className="w-full rounded object-cover" />
            <span className="text-xs text-neutral-400">{mediaKindLabels[m.kind]}</span>
          </li>
        ))}
      </ul>
    </section>
  )
}

const mediaKindLabels: Record<MediaItemView['kind'], string> = {
  cover: 'Capa',
  screenshot: 'Screenshot',
  title: 'Tela de título',
}

function Field({ label, children }: { label: string; children: ReactNode }): JSX.Element {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs font-medium text-neutral-400">{label}</dt>
      <dd className="text-sm text-neutral-100">{children}</dd>
    </div>
  )
}

function ListValue({ items }: { items: string[] }): JSX.Element {
  if (items.length === 0) {
    return <span>—</span>
  }
  return (
    <ul className="flex flex-col">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  )
}
