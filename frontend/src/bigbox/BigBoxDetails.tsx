// Details screen of the Big Box [BBX-016]. Read-only view; actions go through callbacks.
import { useEffect, useRef } from 'react'
import type { CSSProperties } from 'react'
import { formatPlayTime } from '../lib/games'
import { playModeLabels } from '../lib/library'
import type { GameDetailView } from '../lib/library'
import type { MediaItemView } from '../lib/metadata'
import { sessionsLabel, shortDate } from './viewModel'

export type BigBoxDetailsProps = {
  game: GameDetailView
  media: MediaItemView[]
  session: 'running' | 'paused' | null
  detailsFocus: 'play' | 'playUntracked' | 'description'
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

function Video({ url, sound }: { url: string; sound: boolean }) {
  const ref = useRef<HTMLVideoElement>(null)
  useEffect(() => {
    const v = ref.current
    if (!v) return undefined
    v.muted = !sound
    void v.play()?.catch(() => {})
    return () => {
      v.pause()
      v.removeAttribute('src')
    }
  }, [url, sound])
  return <video ref={ref} src={url} muted={!sound} loop playsInline style={{ width: '100%', height: '100%', objectFit: 'contain' }} />
}

export function BigBoxDetails(p: BigBoxDetailsProps) {
  const { game } = p
  const items = galleryOf(p.media)
  const descRef = useRef<HTMLDivElement>(null)
  const lastSignal = useRef(p.scrollSignal)

  useEffect(() => {
    if (p.scrollSignal === lastSignal.current) return
    lastSignal.current = p.scrollSignal
    const el = descRef.current
    if (el) el.scrollBy?.({ top: Math.sign(p.scrollSignal) * SCROLL_STEP, behavior: 'smooth' })
  }, [p.scrollSignal])

  const last = shortDate(game.lastPlayedAt || null)
  const facts: string[] = [game.platform, game.year ? String(game.year) : '', game.genres.join(', ')].filter((s) => s !== '')
  const people = [
    game.developers.length > 0 ? `Desenvolvedores: ${game.developers.join(', ')}` : '',
    game.publishers.length > 0 ? `Editoras: ${game.publishers.join(', ')}` : '',
    game.playModes.length > 0 ? game.playModes.map((m) => playModeLabels[m]).join(', ') : '',
  ].filter((s) => s !== '')
  const btn = (focused: boolean): CSSProperties => ({
    padding: '0.6em 1.2em',
    background: focused ? 'var(--arc-bb-accent)' : 'var(--arc-bb-surface)',
    color: focused ? 'var(--arc-bb-on-accent)' : 'var(--arc-bb-text)',
    border: 'none',
    borderRadius: 6,
    ...(focused ? focusRing : {}),
  })

  return (
    <section style={{ color: 'var(--arc-bb-text)', display: 'flex', flexDirection: 'column', gap: '1em', height: '100%' }}>
      <h1 style={{ margin: 0 }}>{game.title}</h1>
      <p style={{ margin: 0, color: 'var(--arc-bb-text-muted)' }}>{facts.join(' · ')}</p>
      {people.map((t) => (
        <p key={t} style={{ margin: 0, color: 'var(--arc-bb-text-muted)' }}>{t}</p>
      ))}
      <p style={{ margin: 0, color: 'var(--arc-bb-text-muted)' }}>
        {[formatPlayTime(game.totalPlaySeconds), last ? `Última vez: ${last}` : '', sessionsLabel(game.playCount)].filter((s) => s !== '').join(' · ')}
        {p.session ? (p.session === 'paused' ? ' · Pausado' : ' · Jogando') : ''}
      </p>
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
        data-focused={p.detailsFocus === 'description' ? 'true' : 'false'}
        style={{ overflowY: 'auto', flex: 1, minHeight: 0, background: 'var(--arc-bb-scrim)', padding: '0.75em', ...(p.detailsFocus === 'description' ? focusRing : {}) }}
      >
        {game.description}
      </div>
      {items.length > 0 && (
        <div style={{ display: 'flex', gap: '0.5em' }}>
          {items.map((m, i) => (
            <div
              key={m.id}
              data-gallery-item
              data-kind={m.kind}
              aria-current={i === p.gallery ? 'true' : undefined}
              style={{ width: '12em', aspectRatio: '16 / 9', background: 'var(--arc-bb-surface)', ...(i === p.gallery ? focusRing : {}) }}
            >
              {m.kind === 'video' ? (
                i === p.gallery ? <Video url={m.url} sound={p.sound} /> : null
              ) : (
                <img src={m.thumbUrl || m.url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
