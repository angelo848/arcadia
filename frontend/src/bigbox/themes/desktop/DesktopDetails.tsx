// Right column of the Desktop theme, active (Detalhes) and inactive (Navegar) [TDK-008..TDK-011, TDK-018, TDK-022, TDK-030, TDK-037].
// The only buttons are "Jogar" and "Jogar sem contar tempo"; the viewer and its thumbnails are not buttons.
import { createContext, useContext, useEffect, useRef } from 'react'
import type { JSX, ReactNode, RefObject } from 'react'
import { galleryOf } from '../../BigBoxDetails'
import type { BigBoxDetailsProps } from '../../BigBoxDetails'
import { FocusVideo } from '../../FocusVideo'
import { coverPlaceholderUrl } from '../../desktopLayout'
import { playWithSound } from '../../playMedia'
import { TimeToBeatList } from '../../widgets'

// The Desktop provides its aside here so the details draw inside it without a second landmark.
export const AsideScrollContext = createContext<RefObject<HTMLElement | null> | null>(null)

const SCROLL_STEP = 120
const EMPTY = '—'

// GalleryPlayer plays the selected video item of the viewer (Detalhes only) [TDK-010].
function GalleryPlayer({ url, sound }: { url: string; sound: boolean }): JSX.Element {
  const ref = useRef<HTMLVideoElement>(null)
  useEffect(() => {
    const v = ref.current
    if (!v) return undefined
    const stopRetry = playWithSound(v, sound)
    return () => {
      stopRetry()
      v.pause()
      v.removeAttribute('src')
    }
  }, [url, sound])
  return <video ref={ref} src={url} muted={!sound} loop playsInline className="h-full w-full object-contain" />
}

function Field({ label, children }: { label: string; children: ReactNode }): JSX.Element {
  return (
    <div className="flex flex-col">
      <dt className="text-xs text-bb-text-muted">{label}</dt>
      <dd className="m-0 text-bb-text">{children}</dd>
    </div>
  )
}

const BUTTON = 'w-full rounded-md px-4 py-2 font-semibold'
const FOCUS_RING = ' outline-2 outline-offset-2 outline-bb-focus'

export function DesktopDetails(p: BigBoxDetailsProps): JSX.Element {
  const { game, labels } = p
  const active = p.active ?? true
  // Inside the Desktop the scroller is the shell's aside; alone, this component is the aside itself.
  const outer = useContext(AsideScrollContext)
  const ownRef = useRef<HTMLElement>(null)
  const rootRef = outer ?? ownRef
  const thumbsRef = useRef<HTMLDivElement>(null)
  const lastSignal = useRef(p.scrollSignal)

  const items = galleryOf(p.media)
  const n = items.length
  const selected = Math.min(Math.max(p.gallery, 0), Math.max(n - 1, 0))
  const current = items[selected]
  const showGallery = active && n > 0
  const viewerFocused = active && n > 0 && p.detailsFocus === 'gallery'
  const playFocused = active && p.detailsFocus === 'play'
  const untrackedFocused = active && p.detailsFocus === 'playUntracked'

  // Back to the top when the game changes [TDK-008].
  useEffect(() => {
    if (rootRef.current) rootRef.current.scrollTop = 0
  }, [game.id])

  // The right analog stick scrolls the panel only while it is active [TDK-018].
  useEffect(() => {
    if (p.scrollSignal === lastSignal.current) return
    lastSignal.current = p.scrollSignal
    if (active) rootRef.current?.scrollBy?.({ top: Math.sign(p.scrollSignal) * SCROLL_STEP, behavior: 'smooth' })
  }, [p.scrollSignal, active])

  // The focused element is kept in view [TDK-022].
  useEffect(() => {
    if (!active) return
    rootRef.current?.querySelector<HTMLElement>('[data-focused="true"]')?.scrollIntoView?.({ block: 'nearest' })
  }, [active, p.detailsFocus])

  // The selected thumbnail is kept in view.
  useEffect(() => {
    thumbsRef.current?.querySelectorAll<HTMLElement>('[data-gallery-item]')[selected]?.scrollIntoView?.({ inline: 'nearest', block: 'nearest' })
  }, [selected])

  const session = p.session === 'running' ? 'Em execução' : p.session === 'paused' ? 'Pausado' : null
  const preview = p.preview

  const Root = outer ? 'div' : 'aside'
  const rootProps = outer
    ? { className: 'flex flex-col gap-3 p-4 text-bb-text' }
    : {
        ref: ownRef as RefObject<HTMLDivElement>,
        'aria-label': 'Detalhes do jogo',
        className: 'flex h-full min-h-0 flex-col gap-3 overflow-y-auto p-4 text-bb-text',
      }

  return (
    <Root data-bb-details="" {...rootProps}>
      <h1 className="m-0 text-2xl font-bold text-bb-heading">{game.title}</h1>
      <div className="flex items-center gap-2 text-bb-text-muted">
        {p.platformIconUrl ? <img src={p.platformIconUrl} alt="" className="h-5 w-5 shrink-0 object-contain" /> : null}
        <span>{game.platform}</span>
        {session && <span className="rounded bg-bb-chip px-2 text-xs text-bb-on-chip">{session}</span>}
      </div>

      <section
        role="region"
        aria-label="Imagens"
        data-focused={viewerFocused ? 'true' : undefined}
        className={`flex shrink-0 flex-col gap-2${viewerFocused ? FOCUS_RING : ''}`}
      >
        <div className="relative aspect-video w-full overflow-hidden rounded-md bg-bb-placeholder">
          {showGallery ? (
            current.kind === 'video' ? (
              <GalleryPlayer url={current.url} sound={p.sound} />
            ) : (
              <img src={current.url} alt="" className="h-full w-full object-contain" />
            )
          ) : (
            <img src={p.coverUrl ?? coverPlaceholderUrl} alt="" className="h-full w-full object-contain" />
          )}
          {!active && preview && preview.videoUrl && (
            <FocusVideo
              gameId={preview.gameId}
              url={preview.videoUrl}
              active={preview.active}
              sound={preview.sound}
              failed={preview.failed}
              onError={preview.onVideoError}
            />
          )}
          {showGallery && (
            <span className="absolute left-2 top-2 rounded bg-bb-chip px-2 text-xs text-bb-on-chip">
              {current.kind === 'video' ? 'Vídeo' : 'Screenshot'}
            </span>
          )}
          {showGallery && (
            <span aria-live="polite" className="absolute bottom-2 right-2 rounded bg-bb-chip px-2 text-xs text-bb-on-chip">
              {`${selected + 1} de ${n}`}
            </span>
          )}
        </div>
        {showGallery && n >= 2 && (
          <div ref={thumbsRef} className="flex gap-2 overflow-x-auto">
            {items.map((m, i) => (
              <div
                key={m.id}
                data-gallery-item=""
                data-kind={m.kind}
                aria-current={i === selected ? 'true' : undefined}
                className={`flex aspect-video h-12 shrink-0 items-center justify-center overflow-hidden rounded bg-bb-surface text-xs text-bb-text ${
                  i === selected ? 'ring-2 ring-bb-text-muted' : ''
                }`}
              >
                {m.kind === 'video' ? <span>Vídeo</span> : <img src={m.thumbUrl || m.url} alt="" className="h-full w-full object-cover" />}
              </div>
            ))}
          </div>
        )}
      </section>

      <div className="flex flex-col gap-2">
        <button
          type="button"
          data-focused={playFocused ? 'true' : undefined}
          className={`${BUTTON} bg-bb-accent text-bb-on-accent${playFocused ? FOCUS_RING : ''}`}
          onClick={p.onPlay}
        >
          Jogar
        </button>
        <button
          type="button"
          data-focused={untrackedFocused ? 'true' : undefined}
          className={`${BUTTON} bg-bb-surface text-bb-text${untrackedFocused ? FOCUS_RING : ''}`}
          onClick={p.onPlayUntracked}
        >
          Jogar sem contar tempo
        </button>
      </div>

      {labels && (
        <dl className="m-0 flex flex-col gap-2">
          <Field label="Tempo total">{labels.totalTime}</Field>
          <Field label="Primeira vez jogado">{labels.firstPlayed}</Field>
          <Field label="Última vez jogado">{labels.lastPlayed}</Field>
          <Field label="Tempo para zerar">
            {p.timeToBeat.length > 0 ? <TimeToBeatList rows={p.timeToBeat} source={p.timeToBeatSource} /> : EMPTY}
          </Field>
          {labels.completedAt && <Field label="Concluído em">{labels.completedAt}</Field>}
          <Field label="Status">{labels.status}</Field>
          {labels.favorite && <Field label="Marcação">Favorito</Field>}
          <Field label="Gêneros">{labels.genres}</Field>
          <Field label="Modos de jogo">{labels.playModes}</Field>
          <Field label="Desenvolvedora">{labels.developers}</Field>
          <Field label="Editora">{labels.publishers}</Field>
          <Field label="Ano">{labels.year}</Field>
        </dl>
      )}
      <div className="flex flex-col">
        <p className="m-0 text-xs text-bb-text-muted">Descrição</p>
        <p className="m-0 whitespace-pre-wrap text-bb-text">{game.description || EMPTY}</p>
      </div>
    </Root>
  )
}
