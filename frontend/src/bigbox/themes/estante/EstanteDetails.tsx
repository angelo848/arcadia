import { useEffect, useRef } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { galleryOf } from '../../BigBoxDetails'
import type { BigBoxDetailsProps } from '../../BigBoxDetails'
import { Panel } from '../../Panel'
import { playWithSound } from '../../playMedia'
import { relativeDays, shortDate } from '../../viewModel'
import { formatPlayTime, playModeLabels, TimeToBeatList } from '../../widgets'

const SCROLL_STEP = 120
const focusRing: CSSProperties = { outline: '3px solid var(--arc-bb-focus)', outlineOffset: 2 }

function PlayIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" width="1.1em" height="1.1em" fill="currentColor">
      <path d="M8 5v14l11-7z" />
    </svg>
  )
}

// Clip of the carousel: plays muted unless sound is on and drops its source when it leaves the viewer [ETN-011].
function Clip({ url, sound }: { url: string; sound: boolean }) {
  const ref = useRef<HTMLVideoElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el) return undefined
    const stopRetry = playWithSound(el, sound)
    return () => {
      stopRetry()
      el.pause()
      el.removeAttribute('src')
    }
  }, [url, sound])
  return <video ref={ref} src={url} muted={!sound} loop playsInline className="h-full w-full object-contain" />
}

function Row({ label, value }: { label: string; value: string }) {
  if (value === '') return null
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-bb-text-muted">{label}</span>
      <span className="text-right font-semibold text-bb-text">{value}</span>
    </div>
  )
}

function Block({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <Panel className="py-3">
      <div data-testid={id} className="flex flex-col gap-2">
        <h2 className="text-lg font-bold uppercase tracking-wider text-bb-heading">{title}</h2>
        {children}
      </div>
    </Panel>
  )
}

// Details screen of the Estante: story and carousel on the left, statistics, time to beat and about on the right [ETN-010..ETN-012].
export function EstanteDetails(p: BigBoxDetailsProps) {
  const { game } = p
  const items = galleryOf(p.media)
  const selected = Math.min(Math.max(p.gallery, 0), Math.max(items.length - 1, 0))
  const current = items[selected]
  const descRef = useRef<HTMLDivElement>(null)
  const lastSignal = useRef(p.scrollSignal)

  useEffect(() => {
    if (p.scrollSignal === lastSignal.current) return
    lastSignal.current = p.scrollSignal
    descRef.current?.scrollBy?.({ top: Math.sign(p.scrollSignal) * SCROLL_STEP, behavior: 'smooth' })
  }, [p.scrollSignal])

  const played = game.totalPlaySeconds > 0
  const last = shortDate(game.lastPlayedAt || null)
  const playedText = played
    ? `Jogado por ${formatPlayTime(game.totalPlaySeconds)}${game.lastPlayedAt ? ` · Jogado recentemente ${relativeDays(game.lastPlayedAt, new Date())}` : ''}`
    : 'Nunca jogado'
  const btn = (focused: boolean): CSSProperties | undefined => (focused ? focusRing : undefined)

  return (
    <section
      data-bb-details=""
      style={{
        display: 'grid',
        gridTemplateColumns: 'minmax(0, 62fr) minmax(0, 38fr)',
        gap: '1.5rem',
        height: '100%',
        minHeight: 0,
        overflow: 'hidden',
        color: 'var(--arc-bb-text)',
      }}
    >
      <div data-testid="est-d-left" className="flex min-h-0 min-w-0 flex-col gap-3">
        <Panel className="shrink-0 py-3">
          <h1 className="line-clamp-2 text-[length:clamp(2rem,3.4vw,3.6rem)] font-extrabold leading-[1.05] text-bb-heading">{game.title}</h1>
        </Panel>
        <Panel className="flex min-h-0 flex-1 flex-col" style={{ flexBasis: 0 }}>
          <div
            ref={descRef}
            role="region"
            aria-label="Descrição"
            tabIndex={-1}
            className="leading-relaxed text-bb-text"
            style={{ overflowY: 'auto', flex: 1, minHeight: 0 }}
          >
            {game.description}
          </div>
        </Panel>
        <div className="flex shrink-0 items-center gap-3">
          <button
            type="button"
            data-focused={p.detailsFocus === 'play' ? 'true' : 'false'}
            style={btn(p.detailsFocus === 'play')}
            className="inline-flex items-center gap-2 rounded-lg bg-bb-accent px-6 py-2.5 text-lg font-bold text-bb-on-accent"
            onClick={p.onPlay}
          >
            <PlayIcon />
            Iniciar jogo
          </button>
          <button
            type="button"
            data-focused={p.detailsFocus === 'playUntracked' ? 'true' : 'false'}
            style={btn(p.detailsFocus === 'playUntracked')}
            className="rounded-lg bg-bb-surface px-6 py-2.5 text-lg font-semibold text-bb-text"
            onClick={p.onPlayUntracked}
          >
            Jogar sem contar tempo
          </button>
        </div>
        <Panel className="shrink-0">
          <p data-testid="est-d-played" className="text-bb-text-muted">
            {playedText}
          </p>
        </Panel>
        {current && (
          <div className="flex min-h-0 flex-col items-center gap-2" style={{ flex: '1.2 1 0' }}>
            <div
              data-testid="est-d-viewer"
              data-focused={p.detailsFocus === 'gallery' ? 'true' : 'false'}
              className="relative min-h-0 w-full flex-1 overflow-hidden rounded-lg bg-bb-surface"
              style={p.detailsFocus === 'gallery' ? focusRing : undefined}
            >
              {current.kind === 'video' ? (
                <Clip url={current.url} sound={p.sound} />
              ) : (
                <img src={current.url} alt="" className="h-full w-full object-contain" />
              )}
            </div>
            <div data-testid="est-d-dots" className="flex shrink-0 items-center gap-2 py-1">
              {items.map((m, i) => (
                <span
                  key={m.id}
                  data-testid="est-d-dot"
                  aria-current={i === selected ? 'true' : undefined}
                  className={`inline-block h-3 w-3 rounded-full border-2 ${
                    i === selected ? 'border-bb-text bg-bb-text' : 'border-bb-text-muted'
                  }`}
                />
              ))}
            </div>
          </div>
        )}
      </div>
      <div data-testid="est-d-right" className="flex min-h-0 min-w-0 flex-col gap-3 overflow-hidden">
        <Block id="est-d-stats" title="Estatísticas do jogo">
          <Row label="Tempo jogado" value={formatPlayTime(game.totalPlaySeconds)} />
          <Row label="Sessões" value={String(game.playCount)} />
          <Row label="Última vez" value={last ?? 'Nunca jogado'} />
        </Block>
        <Block id="est-d-ttb" title="Tempo para zerar">
          <TimeToBeatList rows={p.timeToBeat} source={p.timeToBeatSource} />
        </Block>
        <Block id="est-d-about" title="Sobre">
          <Row label="Plataforma" value={game.platform} />
          <Row label="Gêneros" value={game.genres.join(', ')} />
          <Row label="Modos de jogo" value={game.playModes.map((m) => playModeLabels[m]).join(', ')} />
          <Row label="Desenvolvedor" value={game.developers.join(', ')} />
          <Row label="Publicado por" value={game.publishers.join(', ')} />
          <Row label="Ano de lançamento" value={game.year ? String(game.year) : ''} />
        </Block>
      </div>
    </section>
  )
}
