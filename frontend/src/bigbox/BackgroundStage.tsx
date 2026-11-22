import { useEffect, useRef, useState } from 'react'
import { playWithSound } from './playMedia'
import { VIDEO_DELAY_MS, useVideoDelay } from './useVideoDelay'

export { VIDEO_DELAY_MS }
export const VIDEO_FADE_MS = 400
export const ART_FADE_MS = 300

export type BackgroundStageProps = {
  backdrop: 'contain' | 'cover' | 'plain'
  gameId: number | null
  artUrl: string | null
  videoUrl: string | null
  active: boolean
  sound: boolean
  failed: boolean
  onVideoError: (gameId: number) => void
  onVideoVisible: (visible: boolean) => void
}

// BackgroundStage draws the theme's backdrop with a crossfade and, after the focus rests, a single video [BBX-010..BBX-013, BBX-052, BBX-053].
export function BackgroundStage(p: BackgroundStageProps) {
  const { backdrop, gameId, artUrl, videoUrl, active, sound, failed, onVideoError, onVideoVisible } = p
  const plain = backdrop === 'plain'
  const art = plain ? null : artUrl
  const [layers, setLayers] = useState<string[]>(art ? [art] : [])
  const [shown, setShown] = useState(false)

  const layersRef = useRef(layers)
  layersRef.current = layers

  useEffect(() => {
    if (!art) {
      setLayers([])
      return undefined
    }
    if (layersRef.current[layersRef.current.length - 1] === art) return undefined
    setLayers((prev) => [...prev.slice(-1), art])
    const t = setTimeout(() => setLayers([art]), ART_FADE_MS)
    return () => clearTimeout(t)
  }, [art])

  const showVideo = useVideoDelay({ gameId, videoUrl: plain ? null : videoUrl, active, failed })

  // React only reads `muted` when the element is created, so the sound toggle is applied by hand.
  const videoRef = useRef<HTMLVideoElement>(null)
  useEffect(() => {
    const v = videoRef.current
    if (!v) return undefined
    return playWithSound(v, sound)
  }, [sound, showVideo, gameId])

  const visibleRef = useRef(onVideoVisible)
  visibleRef.current = onVideoVisible
  useEffect(() => {
    if (!showVideo) {
      setShown(false)
      return undefined
    }
    visibleRef.current(true)
    setShown(true)
    return () => visibleRef.current(false)
  }, [showVideo, gameId])

  return (
    <div className="absolute inset-0 overflow-hidden bg-bb-bg" aria-hidden="true">
      {!plain && layers.length === 0 && (
        <div
          data-testid="bb-gradient"
          className="absolute inset-0"
          style={{ backgroundImage: 'linear-gradient(135deg, var(--arc-bb-bg), var(--arc-bb-bg-2))' }}
        />
      )}
      {!plain &&
        [...layers].reverse().map((url, i, arr) => (
          <div
            key={url}
            data-testid="bb-art-layer"
            className="absolute inset-0"
            style={{ opacity: i === 0 && arr.length > 1 ? 0 : 1, transition: `opacity ${ART_FADE_MS}ms` }}
          >
            {backdrop === 'contain' ? (
              <>
                <img
                  data-testid="bb-art-fill"
                  alt=""
                  src={url}
                  className="absolute inset-0 h-full w-full scale-110 object-cover blur-[32px]"
                />
                <div className="absolute inset-0" style={{ background: 'var(--arc-bb-scrim)' }} />
                <img data-testid="bb-art" alt="" src={url} className="absolute inset-0 h-full w-full object-contain" />
              </>
            ) : (
              <>
                <img data-testid="bb-art" alt="" src={url} className="absolute inset-0 h-full w-full object-cover" />
                <div
                  data-testid="bb-cover-gradient"
                  className="absolute inset-0"
                  style={{ backgroundImage: 'linear-gradient(to bottom, transparent, var(--arc-bb-scrim), var(--arc-bb-bg))' }}
                />
              </>
            )}
          </div>
        ))}
      {showVideo && (
        <video
          ref={videoRef}
          key={`${gameId}`}
          data-testid="bb-video"
          src={videoUrl ?? undefined}
          muted={!sound}
          loop
          playsInline
          autoPlay
          className="absolute inset-0 h-full w-full object-contain"
          style={{ opacity: shown ? 1 : 0, transition: `opacity ${VIDEO_FADE_MS}ms` }}
          onError={() => {
            if (gameId !== null) onVideoError(gameId)
          }}
        />
      )}
    </div>
  )
}
