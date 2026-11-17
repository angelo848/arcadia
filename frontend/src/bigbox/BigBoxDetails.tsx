// Details screen of the Big Box [BBX-016, BBX-032..BBX-037, BBX-041, BBX-043, BBX-044]. Read-only view; actions go through callbacks.
import { useEffect, useRef } from 'react'
import type { CSSProperties } from 'react'
import { formatPlayTime } from '../lib/games'
import { playModeLabels } from '../lib/library'
import type { GameDetailView } from '../lib/library'
import type { MediaItemView } from '../lib/metadata'
import type { DetailsFocus } from './nav'
import { Panel } from './Panel'
import { playWithSound } from './playMedia'
import { sessionsLabel, shortDate } from './viewModel'

export type BigBoxDetailsProps = {
  game: GameDetailView
  media: MediaItemView[]
  session: 'running' | 'paused' | null
  detailsFocus: DetailsFocus
  gallery: number
  sound: boolean
  scrollSignal: number
  onPlay: () => void
  onPlayUntracked: () => void
}

const SCROLL_STEP = 120

export function galleryOf(media: MediaItemView[]): MediaItemView[] {
  return [...media.filter((m) => m.kind === 'video'), ...media.filter((m) => m.kind === 'screenshot')]
}

const focusRing: CSSProperties = { outline: '3px solid var(--arc-bb-focus)', outlineOffset: 2 }
const noMargin: CSSProperties = { margin: 0 }

function Video({ url, sound }: { url: string; sound: boolean }) {
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

export function BigBoxDetails(p: BigBoxDetailsProps) {
  const { game } = p
  const items = galleryOf(p.media)
  const selected = Math.min(Math.max(p.gallery, 0), Math.max(items.length - 1, 0))
  const current = items[selected]
  const descRef = useRef<HTMLDivElement>(null)
  const thumbsRef = useRef<HTMLDivElement>(null)
  const lastSignal = useRef(p.scrollSignal)

  useEffect(() => {
    if (p.scrollSignal === lastSignal.current) return
    lastSignal.current = p.scrollSignal
    const el = descRef.current
    if (el) el.scrollBy?.({ top: Math.sign(p.scrollSignal) * SCROLL_STEP, behavior: 'smooth' })
  }, [p.scrollSignal])

  // The selected thumbnail is kept in view [BBX-034]; jsdom has no scrollIntoView.
  useEffect(() => {
    const el = thumbsRef.current?.querySelectorAll<HTMLElement>('[data-gallery-item]')[selected]
    el?.scrollIntoView?.({ inline: 'nearest', block: 'nearest' })
  }, [selected])

  const last = shortDate(game.lastPlayedAt || null)
  const platformLine = [game.platform, game.year ? String(game.year) : '', game.genres.join(', ')].filter((s) => s !== '').join(' · ')
  const lines = [
    platformLine,
    game.developers.length > 0 ? `Desenvolvedores: ${game.developers.join(', ')}` : '',
    game.publishers.length > 0 ? `Editoras: ${game.publishers.join(', ')}` : '',
    game.playModes.length > 0 ? game.playModes.map((m) => playModeLabels[m]).join(', ') : '',
    [formatPlayTime(game.totalPlaySeconds), last ? `Última vez: ${last}` : '', sessionsLabel(game.playCount)].filter((s) => s !== '').join(' · ') +
      (p.session ? (p.session === 'paused' ? ' · Pausado' : ' · Jogando') : ''),
  ].filter((s) => s !== '')
  const btn = (focused: boolean): CSSProperties => ({
    padding: '0.6em 1.2em',
    background: focused ? 'var(--arc-bb-accent)' : 'var(--arc-bb-surface)',
    color: focused ? 'var(--arc-bb-on-accent)' : 'var(--arc-bb-text)',
    border: 'none',
    borderRadius: 6,
    ...(focused ? focusRing : {}),
  })
  const hasGallery = items.length > 0

  return (
    <section
      data-bb-details=""
      style={{
        display: 'grid',
        gridTemplateColumns: hasGallery ? '40% minmax(0, 1fr)' : 'minmax(0, 60%)',
        gap: '1em',
        height: '100%',
        minHeight: 0,
        overflow: 'hidden',
        color: 'var(--arc-bb-text)',
      }}
    >
      <Panel
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: '0.6em',
          minHeight: 0,
          ...(hasGallery ? {} : { maxWidth: '60%' }),
        }}
      >
        <h1 style={{ ...noMargin, fontSize: 'clamp(2rem, 3.2vw, 3.2rem)', lineHeight: 1.1 }}>{game.title}</h1>
        <div
          data-bb-facts=""
          style={{ display: 'flex', flexDirection: 'column', rowGap: '0.2em', lineHeight: 1.3, color: 'var(--arc-bb-text-muted)' }}
        >
          {lines.map((t) => (
            <p key={t} style={t === platformLine ? { ...noMargin, fontSize: '1.4em', color: 'var(--arc-bb-text)' } : noMargin}>
              {t}
            </p>
          ))}
        </div>
        <div style={{ display: 'flex', gap: '0.75em' }}>
          <button type="button" data-focused={p.detailsFocus === 'play' ? 'true' : 'false'} style={btn(p.detailsFocus === 'play')} onClick={p.onPlay}>
            Jogar
          </button>
          <button
            type="button"
            data-focused={p.detailsFocus === 'playUntracked' ? 'true' : 'false'}
            style={btn(p.detailsFocus === 'playUntracked')}
            onClick={p.onPlayUntracked}
          >
            Jogar sem contar tempo
          </button>
        </div>
        <div
          ref={descRef}
          role="region"
          aria-label="Descrição"
          tabIndex={-1}
          style={{ overflowY: 'auto', flex: 1, minHeight: 0, color: 'var(--arc-bb-text)' }}
        >
          {game.description}
        </div>
      </Panel>
      {hasGallery && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75em', minHeight: 0, minWidth: 0 }}>
          <figure data-bb-viewer="" style={{ ...noMargin, position: 'relative', flex: 1, minHeight: 0, overflow: 'hidden' }}>
            {current.kind === 'video' ? (
              <Video url={current.url} sound={p.sound} />
            ) : (
              <img src={current.url} alt="" className="h-full w-full object-contain" />
            )}
            <Panel as="figcaption" className="absolute bottom-2 right-2 py-1">
              <span>{`${selected + 1} / ${items.length}`}</span>
            </Panel>
          </figure>
          <div
            ref={thumbsRef}
            data-bb-thumbs=""
            data-focused={p.detailsFocus === 'gallery' ? 'true' : 'false'}
            style={{
              display: 'flex',
              gap: '0.5em',
              height: '10vh',
              flexShrink: 0,
              overflowX: 'auto',
              ...(p.detailsFocus === 'gallery' ? focusRing : {}),
            }}
          >
            {items.map((m, i) => (
              <div
                key={m.id}
                data-gallery-item=""
                data-kind={m.kind}
                aria-current={i === selected ? 'true' : undefined}
                style={{
                  height: '100%',
                  aspectRatio: '16 / 9',
                  flexShrink: 0,
                  background: 'var(--arc-bb-surface)',
                  border: i === selected ? '2px solid var(--arc-bb-text-muted)' : '2px solid transparent',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  boxSizing: 'border-box',
                }}
              >
                {m.kind === 'video' ? (
                  <span>Vídeo</span>
                ) : (
                  <img src={m.thumbUrl || m.url} alt="" className="h-full w-full object-contain" />
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  )
}
