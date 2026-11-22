import { useEffect, useRef } from 'react'
import type { JSX } from 'react'
import { playWithSound } from './playMedia'
import { useVideoDelay } from './useVideoDelay'

export type FocusVideoProps = {
  gameId: number
  url: string
  active: boolean
  sound: boolean
  failed: boolean
  onError: (gameId: number) => void
}

// FocusVideo plays the focused game's video inside its cover after the focus rests [BBX-053].
export function FocusVideo(p: FocusVideoProps): JSX.Element | null {
  const { gameId, url, active, sound, failed, onError } = p
  const ready = useVideoDelay({ gameId, videoUrl: url, active, failed })
  const ref = useRef<HTMLVideoElement>(null)
  const soundRef = useRef(sound)
  soundRef.current = sound

  // React only reads `muted` when the element is created, so the sound toggle is applied by hand.
  useEffect(() => {
    const v = ref.current
    if (!v) return undefined
    return playWithSound(v, sound)
  }, [sound, ready, gameId])

  useEffect(() => {
    const v = ref.current
    if (!ready || !v) return undefined
    return () => {
      v.pause()
      v.removeAttribute('src')
    }
  }, [ready, gameId])

  if (!ready) return null
  return (
    <video
      ref={ref}
      key={gameId}
      data-testid="bb-video"
      src={url}
      muted={!sound}
      loop
      playsInline
      autoPlay
      className="absolute inset-0 h-full w-full object-contain"
      onError={() => onError(gameId)}
    />
  )
}
