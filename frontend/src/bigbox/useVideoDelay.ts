import { useEffect, useState } from 'react'

export const VIDEO_DELAY_MS = 2000

export type VideoDelayInput = { gameId: number | null; videoUrl: string | null; active: boolean; failed: boolean }

// useVideoDelay is true once the same game has been focused for VIDEO_DELAY_MS with a video that may play [BBX-011, BBX-053].
export function useVideoDelay(o: VideoDelayInput): boolean {
  const { gameId, videoUrl, active, failed } = o
  const canPlay = gameId !== null && !!videoUrl && active && !failed
  const key = `${gameId}|${videoUrl ?? ''}`
  const [readyKey, setReadyKey] = useState<string | null>(null)

  useEffect(() => {
    if (!canPlay) {
      setReadyKey(null)
      return undefined
    }
    const t = setTimeout(() => setReadyKey(key), VIDEO_DELAY_MS)
    return () => clearTimeout(t)
  }, [canPlay, key])

  return canPlay && readyKey === key
}
