import { useEffect, useLayoutEffect, useRef, useState, type JSX, type ReactNode } from 'react'
import { queueResultLabel, useVideoCredentials, videoQueue } from '../lib/video'
import type { GameStatus } from '../lib/filters'
import type { SessionView } from '../lib/games'
import { formatPlayTime } from '../lib/games'
import { isNotFound, playModeLabels, useCatalog, useGame } from '../lib/library'
import { formatLocalDateTime, formatUtcDate } from '../lib/formatDate'
import { useMedia } from '../lib/metadata'
import type { PendingLaunch } from '../lib/pendingLaunch'
import type { SelectionEvent } from '../lib/selection'
import { MediaCarousel } from './MediaCarousel'
import { PlatformIcon } from './PlatformIcon'
import { SessionsList } from './SessionsList'

const statusLabels: Record<GameStatus, string> = {
  not_played: 'Não jogado',
  playing: 'Jogando',
  completed: 'Concluído',
  abandoned: 'Abandonado',
  backlog: 'Na fila',
}

export type GameDetailsPanelProps = {
  gameId: number
  current: SessionView | null
  pending: PendingLaunch
  onPlay: (id: number) => void
  onPlayUntracked: (id: number) => void
  onEdit: () => void
  onSearchMetadata: () => void
  onRemove: () => void
  onSelectionEvent: (e: SelectionEvent) => void
}

// GameDetailsPanel shows the game in focus with its data, media, launch buttons and sessions
// [UIX-025..UIX-030, UIX-035, UIX-036]. The root element is the same for every game so its scroll
// position can be reset when the game changes.
export function GameDetailsPanel(props: GameDetailsPanelProps): JSX.Element {
  const { gameId, current, pending, onPlay, onPlayUntracked, onEdit, onSearchMetadata, onRemove, onSelectionEvent } = props
  const rootRef = useRef<HTMLDivElement>(null)
  const q = useGame(gameId)
  const catalog = useCatalog({ staleTime: Infinity })
  const media = useMedia(gameId)
  const creds = useVideoCredentials()
  const [videoBusy, setVideoBusy] = useState(false)
  const [videoMsg, setVideoMsg] = useState<{ gameId: number; text: string; error: boolean } | null>(null)

  async function searchVideo() {
    setVideoBusy(true)
    setVideoMsg(null)
    try {
      const r = await videoQueue([gameId])
      setVideoMsg({ gameId, text: queueResultLabel(r), error: false })
    } catch (e) {
      setVideoMsg({ gameId, text: e instanceof Error ? e.message : 'Não foi possível buscar o vídeo.', error: true })
    } finally {
      setVideoBusy(false)
    }
  }

  const emitRef = useRef(onSelectionEvent)
  emitRef.current = onSelectionEvent

  useLayoutEffect(() => {
    if (rootRef.current) {
      rootRef.current.scrollTop = 0
    }
  }, [gameId])

  const g = q.data
  const readOk = q.isSuccess && g !== undefined
  const errorMissing = q.isError && isNotFound(q.error)
  const errorMessage = q.isError ? (q.error?.message ?? '') : ''

  useEffect(() => {
    if (readOk) {
      emitRef.current({ type: 'readOk', gameId })
    } else if (errorMissing) {
      emitRef.current({ type: 'readMissing', gameId })
    } else if (q.isError) {
      emitRef.current({ type: 'readError', gameId, message: errorMessage })
    }
  }, [gameId, readOk, errorMissing, q.isError, errorMessage, q.errorUpdatedAt])

  const rootClass = 'h-full overflow-y-auto bg-surface px-6 py-4 text-fg'

  if (q.isError && !errorMissing) {
    return (
      <div ref={rootRef} className={rootClass}>
        <p role="alert" className="text-sm text-danger">
          {errorMessage}
        </p>
        <button
          type="button"
          data-bigbox-item
          onClick={() => {
            emitRef.current({ type: 'retry' })
            void q.refetch()
          }}
          className="mt-3 rounded bg-raised px-3 py-1 text-sm text-fg hover:bg-hover"
        >
          Tentar de novo
        </button>
      </div>
    )
  }

  if (g === undefined) {
    return (
      <div ref={rootRef} className={rootClass}>
        <p className="text-sm text-muted">Carregando jogo…</p>
      </div>
    )
  }

  const platform = catalog.data?.platforms.find((p) => p.id === g.platformId)
  const sessionHere = current !== null && current.gameId === g.id ? current.state : null
  const platformDefault = platform?.emulatorId ?? null
  const noEmulator = g.launchType === 'emulator' && g.emulatorId === null && platformDefault === null
  const busy = pending !== null && pending.id === g.id
  const playLabel = busy && pending.mode === 'tracked' ? 'Iniciando…' : 'Jogar'
  const untrackedLabel = busy && pending.mode === 'untracked' ? 'Iniciando…' : 'Jogar sem contar tempo'
  // A game with only video has no images: the normal window ignores video [VID-023].
  const onlyVideo = media.data !== undefined && media.data.length > 0 && media.data.every((m) => m.kind === 'video')
  const hasVideo = media.data?.some((m) => m.kind === 'video') ?? false
  const secondary ='rounded bg-raised px-3 py-1 text-sm font-medium text-fg hover:bg-hover'

  return (
    <div ref={rootRef} className={rootClass}>
      <article className="flex flex-col gap-4">
        <header className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            {platform && <PlatformIcon platform={platform} size={24} />}
            <h2 className="text-2xl font-semibold text-fg">{g.title}</h2>
          </div>
          <p className="text-sm text-muted">{g.platform}</p>
          <div className="flex flex-wrap items-center gap-3 text-sm">
            {sessionHere === 'running' || sessionHere === 'grace' ? (
              <span className="font-medium text-play">Em execução</span>
            ) : sessionHere === 'paused' ? (
              <span className="font-medium text-warning">Pausado</span>
            ) : null}
            {sessionHere !== null && current?.tracking === 'process' && (
              <span className="text-muted">Rastreio por processo: pausa e processos filhos não entram no tempo</span>
            )}
            {noEmulator && (
              <span className="text-warning">Sem emulador configurado: escolha um padrão em Emuladores ou neste jogo.</span>
            )}
          </div>
        </header>

        {!onlyVideo && <MediaCarousel items={media.data} resetKey={g.id} />}

        <div className="flex flex-col gap-2">
          <button
            type="button"
            data-bigbox-item
            data-bigbox-play
            disabled={busy}
            onClick={() => onPlay(g.id)}
            className="w-full min-h-12 rounded text-lg font-bold bg-play hover:bg-play-hover text-on-accent disabled:opacity-50"
          >
            {playLabel}
          </button>
          <button
            type="button"
            data-bigbox-item
            disabled={busy}
            onClick={() => onPlayUntracked(g.id)}
            className="w-full min-h-10 rounded text-base bg-raised hover:bg-hover border border-play text-fg disabled:opacity-50"
          >
            {untrackedLabel}
          </button>
          <div className="flex flex-wrap items-center gap-2">
          <button type="button" data-bigbox-item onClick={() => onEdit()} className={secondary}>
            Editar
          </button>
          <button type="button" data-bigbox-item onClick={() => onSearchMetadata()} className={secondary}>
            Buscar metadados
          </button>
          {creds.ready === true && !hasVideo && (
            <button type="button" data-bigbox-item disabled={videoBusy} onClick={() => void searchVideo()} className={secondary}>
              Buscar vídeo
            </button>
          )}
          <button type="button" data-bigbox-item onClick={() => onRemove()} className={`${secondary} text-danger`}>
            Remover
          </button>
          </div>
          {videoMsg !== null && videoMsg.gameId === g.id && (
            <p role={videoMsg.error ? 'alert' : 'status'} className={`text-sm ${videoMsg.error ? 'text-danger' : 'text-muted'}`}>
              {videoMsg.text}
            </p>
          )}
        </div>

        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
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
          <Field label="Status">
            <span>{statusLabels[g.status]}</span>
          </Field>
          {g.favorite && (
            <Field label="Marcação">
              <span className="text-warning">Favorito</span>
            </Field>
          )}
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
        </dl>

        <section className="flex flex-col gap-1">
          <h3 className="text-sm font-medium text-muted">Descrição</h3>
          {g.description !== '' ? <p className="whitespace-pre-line text-fg">{g.description}</p> : <p>—</p>}
        </section>

        <section className="flex flex-col gap-2">
          <h3 className="text-lg font-medium text-fg">Sessões</h3>
          <SessionsList gameId={g.id} current={null} />
        </section>
      </article>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }): JSX.Element {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs font-medium text-muted">{label}</dt>
      <dd className="text-sm text-fg">{children}</dd>
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
