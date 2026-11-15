// Window and agent events of the Big Box [BBX-018, CON-022, CON-024]. Subscribes only while active.
import { useEffect, useRef } from 'react'
import type { NavAction } from './nav'

type Off = (() => void) | void
type Win = { runtime?: { EventsOn?: (name: string, cb: (...d: unknown[]) => void) => Off } }

export type BigBoxEventsOpts = {
  dispatch: (a: NavAction) => void
  launchedGameId: number | null
  active: boolean
}

// useBigBoxEvents turns window blur/focus, session:changed and game:closedByHotkey into reducer actions.
// The gameClosed action carries indexInTab null; the shell, which knows the loaded list, fills it in.
export function useBigBoxEvents(o: BigBoxEventsOpts): void {
  const ref = useRef(o)
  ref.current = o
  const { active } = o

  useEffect(() => {
    if (!active) return undefined
    const onBlur = () => ref.current.dispatch({ type: 'windowBlur' })
    const onFocus = () => ref.current.dispatch({ type: 'windowFocus' })
    window.addEventListener('blur', onBlur)
    window.addEventListener('focus', onFocus)

    const offs: Array<() => void> = []
    const on = (window as unknown as Win).runtime?.EventsOn
    const sub = (name: string, cb: (d: unknown) => void) => {
      const off = on?.(name, (...d: unknown[]) => cb(d[0]))
      if (typeof off === 'function') offs.push(off)
    }
    sub('session:changed', (d) => {
      const s = d as { gameId?: number; state?: string } | undefined
      if (!s || typeof s !== 'object') return
      const { launchedGameId, dispatch } = ref.current
      if (launchedGameId !== null && s.gameId === launchedGameId && (s.state === 'ended' || s.state === 'discarded')) {
        dispatch({ type: 'sessionEnded', gameId: launchedGameId })
      }
    })
    sub('game:closedByHotkey', (d) => {
      const e = d as { gameId?: number } | undefined
      if (!e || typeof e.gameId !== 'number') return
      ref.current.dispatch({ type: 'gameClosed', gameId: e.gameId, indexInTab: null })
    })

    return () => {
      window.removeEventListener('blur', onBlur)
      window.removeEventListener('focus', onFocus)
      for (const off of offs) off()
    }
  }, [active])
}
