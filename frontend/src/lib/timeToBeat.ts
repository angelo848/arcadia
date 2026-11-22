import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

// Time to beat of a game: view, formatting and hooks [TTB-013..TTB-017, TTB-039].

export type TimeToBeatCandidate = {
  externalId: string
  title: string
  year: number | null
  platforms: string[]
  mainSeconds: number | null
  mainExtraSeconds: number | null
  completionistSeconds: number | null
  score: number
}

export type TimeToBeatView = {
  gameId: number
  state: 'none' | 'found' | 'doubt' | 'not-found' | string
  source: 'hltb' | 'igdb' | '' | string
  externalId: string
  externalTitle: string
  mainSeconds: number | null
  mainExtraSeconds: number | null
  completionistSeconds: number | null
  fetchedAt: string
  confirmed: boolean
  stale: boolean
  pending: boolean
  unavailable: boolean
  candidates: TimeToBeatCandidate[]
}

export type TimeToBeatRow = { label: string; value: string }

// Wait before asking the agent to query the provider when the cache is empty.
export const TTB_FETCH_DELAY_MS = 1000

export const timeToBeatKey = (gameId: number) => ['timeToBeat', gameId] as const

// formatTimeToBeat words seconds: minutes (steps of 5) under an hour, otherwise hours in half steps.
export function formatTimeToBeat(seconds: number | null | undefined): string {
  if (seconds == null || !(seconds > 0)) {
    return '—'
  }
  if (seconds < 3600) {
    const min = Math.max(5, Math.round(seconds / 300) * 5)
    return min >= 60 ? '1 h' : `${min} min`
  }
  const half = Math.round((seconds / 3600) * 2)
  const whole = Math.floor(half / 2)
  return half % 2 === 1 ? `${whole}½ h` : `${whole} h`
}

export function timeToBeatRows(v: TimeToBeatView | undefined): TimeToBeatRow[] {
  return [
    { label: 'História principal', value: formatTimeToBeat(v?.mainSeconds) },
    { label: 'História + extras', value: formatTimeToBeat(v?.mainExtraSeconds) },
    { label: '100%', value: formatTimeToBeat(v?.completionistSeconds) },
  ]
}

export function timeToBeatSource(v: TimeToBeatView | undefined): string | null {
  if (v?.source === 'hltb') {
    return 'HowLongToBeat'
  }
  if (v?.source === 'igdb') {
    return 'IGDB'
  }
  return null
}

type TtbApp = { TimeToBeatGet?: (gameId: number, fetch: boolean) => Promise<TimeToBeatView | null> }
type WailsWindow = {
  go?: { main?: { App?: TtbApp } }
  runtime?: { EventsOn?: (name: string, cb: (...data: unknown[]) => void) => (() => void) | void }
}

async function getView(gameId: number, fetch: boolean): Promise<TimeToBeatView | undefined> {
  try {
    const fn = (window as unknown as WailsWindow).go?.main?.App?.TimeToBeatGet
    if (!fn) {
      return undefined
    }
    return (await fn(gameId, fetch)) ?? undefined
  } catch {
    return undefined
  }
}

// useTimeToBeat reads the cache and, unless fetchAfterMs is null, asks for one fetch after that delay (0 = now).
export function useTimeToBeat(gameId: number | null, opts: { fetchAfterMs: number | null }) {
  const qc = useQueryClient()
  const { fetchAfterMs } = opts
  const q = useQuery({
    queryKey: timeToBeatKey(gameId ?? 0),
    // react-query rejects undefined data, so the cache holds null and select turns it back into undefined.
    queryFn: async () => (await getView(gameId as number, false)) ?? null,
    select: (v: TimeToBeatView | null): TimeToBeatView | undefined => v ?? undefined,
    enabled: gameId !== null,
    retry: false,
    gcTime: Infinity,
    refetchOnWindowFocus: false,
  })

  useEffect(() => {
    if (gameId === null || fetchAfterMs === null) {
      return undefined
    }
    const run = () => {
      void getView(gameId, true).then((v) => {
        if (v !== undefined) {
          qc.setQueryData(timeToBeatKey(gameId), v)
        }
      })
    }
    if (fetchAfterMs <= 0) {
      run()
      return undefined
    }
    const t = setTimeout(run, fetchAfterMs)
    return () => clearTimeout(t)
  }, [gameId, fetchAfterMs, qc])

  return q
}

// useTimeToBeatEvents invalidates only the game named by each timeToBeat:changed.
export function useTimeToBeatEvents() {
  const qc = useQueryClient()
  useEffect(() => {
    const on = (window as unknown as WailsWindow).runtime?.EventsOn
    if (!on) {
      return undefined
    }
    const off = on('timeToBeat:changed', (...data: unknown[]) => {
      const id = (data[0] as { gameId?: unknown } | undefined)?.gameId
      if (typeof id === 'number') {
        void qc.invalidateQueries({ queryKey: timeToBeatKey(id) })
      }
    })
    return () => {
      if (typeof off === 'function') {
        off()
      }
    }
  }, [qc])
}
