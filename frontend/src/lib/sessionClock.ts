import { useEffect, useState } from 'react'
import type { SessionView } from './games'

// SessionAnchor is the last applied session value plus the UI monotonic instant
// at which it was received [UI-014].
export type SessionAnchor = { activeMs: number; state: 'running' | 'grace'; receivedAt: number }

// monotonicNow is the default clock: the UI's own monotonic clock. Wall-clock
// time (Date, startedAt, asOf) is never used for the live value.
function monotonicNow(): number {
  return performance.now()
}

// formatElapsed renders seconds as H:MM:SS [UI-013].
export function formatElapsed(seconds: number): string {
  const total = seconds > 0 ? Math.floor(seconds) : 0
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

export function anchorFrom(s: SessionView, now: () => number = monotonicNow): SessionAnchor {
  return {
    activeMs: s.activeMs,
    state: s.state === 'grace' ? 'grace' : 'running',
    receivedAt: now(),
  }
}

function elapsedMs(anchor: SessionAnchor, now: () => number): number {
  if (anchor.state !== 'running') {
    return anchor.activeMs
  }
  return anchor.activeMs + Math.max(0, now() - anchor.receivedAt)
}

// useElapsedSeconds returns the live active seconds for the anchor. In running
// it re-renders at each second boundary; in grace, without anchor or after
// unmount no timer is armed. It never calls a binding [UI-014, UI-015, UI-017].
export function useElapsedSeconds(
  anchor: SessionAnchor | null,
  now: () => number = monotonicNow,
): number {
  const [, setTick] = useState(0)

  useEffect(() => {
    if (!anchor || anchor.state !== 'running') {
      return undefined
    }
    let id: ReturnType<typeof setTimeout> | undefined
    const arm = () => {
      const rem = elapsedMs(anchor, now) % 1000
      id = setTimeout(() => {
        setTick((t) => t + 1)
        arm()
      }, 1000 - rem)
    }
    arm()
    return () => {
      if (id !== undefined) {
        clearTimeout(id)
      }
    }
  }, [anchor, now])

  if (!anchor) {
    return 0
  }
  return Math.floor(elapsedMs(anchor, now) / 1000)
}
