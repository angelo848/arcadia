import { useEffect, useRef, useState } from 'react'

export const VIDEO_DELAY_MS = 2000
export const VIDEO_FADE_MS = 400
export const ART_FADE_MS = 300

export type BackgroundStageProps = {
  gameId: number | null
  artUrl: string | null
  videoUrl: string | null
  active: boolean
  sound: boolean
  failed: boolean
  onVideoError: (gameId: number) => void
  onVideoVisible: (visible: boolean) => void
}

// BackgroundStage draws the full-screen art with a crossfade and, after the focus rests, a single muted video [BBX-010..BBX-013].
export function BackgroundStage(p: BackgroundStageProps) {
  const { gameId, artUrl, videoUrl, active, sound, failed, onVideoError, onVideoVisible } = p
  const [layers, setLayers] = useState<string[]>(artUrl ? [artUrl] : [])
  const [readyId, setReadyId] = useState<number | null>(null)
  const [shown, setShown] = useState(false)

  const layersRef = useRef(layers)
  layersRef.current = layers

  useEffect(() => {
    if (!artUrl) {
      setLayers([])
      return
    }
    if (layersRef.current[layersRef.current.length - 1] === artUrl) return
    setLayers((prev) => [...prev.slice(-1), artUrl])
    const t = setTimeout(() => setLayers([artUrl]), ART_FADE_MS)
    return () => clearTimeout(t)
  }, [artUrl])

  const canPlay = gameId !== null && !!videoUrl && active && !failed
  useEffect(() => {
    if (!canPlay) {
      setReadyId(null)
      return
    }
    const t = setTimeout(() => setReadyId(gameId), VIDEO_DELAY_MS)
    return () => clearTimeout(t)
  }, [canPlay, gameId, videoUrl])

  const showVideo = canPlay && readyId === gameId

  const visibleRef = useRef(onVideoVisible)
  visibleRef.current = onVideoVisible
  useEffect(() => {
    if (!showVideo) {
      setShown(false)
      return
    }
    visibleRef.current(true)
    setShown(true)
    return () => visibleRef.current(false)
  }, [showVideo, gameId])

  return (
    <div className="absolute inset-0 overflow-hidden bg-bb-bg" aria-hidden="true">
      {layers.length === 0 && (
        <div
          data-testid="bb-gradient"
          className="absolute inset-0"
          style={{ backgroundImage: 'linear-gradient(135deg, var(--arc-bb-bg), var(--arc-bb-bg-2))' }}
        />
      )}
      {[...layers].reverse().map((url, i, arr) => (
        <img
          key={url}
          data-testid="bb-art"
          alt=""
          src={url}
          className="absolute inset-0 h-full w-full object-cover"
          style={{ opacity: i === 0 && arr.length > 1 ? 0 : 1, transition: `opacity ${ART_FADE_MS}ms` }}
        />
      ))}
      {showVideo && (
        <video
          key={`${gameId}`}
          data-testid="bb-video"
          src={videoUrl ?? undefined}
          muted={!sound}
          loop
          playsInline
          autoPlay
          className="absolute inset-0 h-full w-full object-cover"
          style={{ opacity: shown ? 1 : 0, transition: `opacity ${VIDEO_FADE_MS}ms` }}
          onError={() => {
            setReadyId(null)
            if (gameId !== null) onVideoError(gameId)
          }}
        />
      )}
    </div>
  )
}
