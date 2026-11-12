import { useEffect, useState } from 'react'
import type { SessionView } from './games'

// SessionAnchor is the last applied session value plus the UI monotonic instant
// at which it was received [UI-014].
export type SessionAnchor = {
  activeMs: number
  state: 'running' | 'paused' | 'grace'
  receivedAt: number
}

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

// anchorFrom keeps paused and grace frozen at activeMs; only running advances [UI-014, UI-022].
export function anchorFrom(s: SessionView, now: () => number = monotonicNow): SessionAnchor {
  return {
    activeMs: s.activeMs,
    state: s.state === 'grace' ? 'grace' : s.state === 'paused' ? 'paused' : 'running',
    receivedAt: now(),
  }
}

// elapsedMs adds the monotonic time since the anchor only in running; paused and
// grace show activeMs unchanged [UI-015, UI-022, UI-023].
function elapsedMs(anchor: SessionAnchor, now: () => number): number {
  if (anchor.state !== 'running') {
    return anchor.activeMs
  }
  return anchor.activeMs + Math.max(0, now() - anchor.receivedAt)
}

// useElapsedSeconds returns the live active seconds for the anchor. In running
// it re-renders at each second boundary; in paused and grace, without anchor or
// after unmount no timer is armed. It never calls a binding
// [UI-014, UI-015, UI-017, UI-022, UI-025].
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
