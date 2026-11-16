import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { providerKeyStatus } from './metadata'

// Video queue calls and status of the ScreenScraper lane [VID-004..VID-007, VID-021..VID-024, SSC-003].

export type VideoSkip = { gameId: number; reason: 'has-video' | 'platform-unmapped' | string }
export type VideoQueueResult = { queued: number; skipped: VideoSkip[] }
export type VideoQueueMissingResult = { queued: number; skipped: number }
export type VideoLane = 'active' | 'paused-quota' | 'paused-credentials' | 'no-credentials' | string
export type VideoStatusView = {
  pending: number
  running: number
  failed: number
  lane: VideoLane
  pausedUntil: string
  requestsToday: number
  maxRequestsPerDay: number
}

type VideoApp = {
  VideoQueue?: (gameIds: number[]) => Promise<VideoQueueResult>
  VideoQueueMissing?: (platformId: number) => Promise<VideoQueueMissingResult>
  VideoCancelPending?: () => Promise<number>
  VideoStatus?: () => Promise<VideoStatusView>
}

type WailsWindow = {
  go?: { main?: { App?: VideoApp } }
  runtime?: { EventsOn?: (name: string, cb: (...data: unknown[]) => void) => (() => void) | void }
}

async function call<T>(name: keyof VideoApp, args: unknown[]): Promise<T> {
  const app = (window as unknown as WailsWindow).go?.main?.App
  const fn = app?.[name] as ((...a: unknown[]) => Promise<T>) | undefined
  if (!fn) {
    throw new Error(`${name} indisponível`)
  }
  return fn(...args)
}

export function videoQueue(gameIds: number[]): Promise<VideoQueueResult> {
  return call<VideoQueueResult>('VideoQueue', [gameIds])
}

// platformId 0 means every platform.
export function videoQueueMissing(platformId: number): Promise<VideoQueueMissingResult> {
  return call<VideoQueueMissingResult>('VideoQueueMissing', [platformId])
}

export function videoCancelPending(): Promise<number> {
  return call<number>('VideoCancelPending', [])
}

export function videoStatus(): Promise<VideoStatusView> {
  return call<VideoStatusView>('VideoStatus', [])
}

const skipLabels: Record<string, string> = {
  'has-video': 'Este jogo já tem vídeo ou busca em andamento',
  'platform-unmapped': 'A plataforma não tem vídeo no ScreenScraper',
}

// queueResultLabel words the result of queueing a single game.
export function queueResultLabel(r: VideoQueueResult): string {
  if (r.queued > 0) {
    return 'Vídeo na fila'
  }
  const reason = r.skipped[0]?.reason
  return (reason !== undefined && skipLabels[reason]) || 'Nenhum vídeo foi para a fila'
}

export function queuedLabel(n: number): string {
  return n === 1 ? '1 vídeo na fila' : `${n} vídeos na fila`
}

function localTime(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}`
}

// videoStatusLine summarizes the lane state in one line.
export function videoStatusLine(s: VideoStatusView): string {
  const parts = [`${s.pending} pendentes`, `${s.failed} sem vídeo`]
  if (s.lane === 'paused-quota' && s.pausedUntil !== '') {
    parts.push(`cota esgotada até ${localTime(s.pausedUntil)}`)
  }
  if (s.lane === 'paused-credentials') {
    parts.push('credenciais recusadas')
  }
  if (s.maxRequestsPerDay > 0 || s.requestsToday > 0) {
    parts.push(`${s.requestsToday} de ${s.maxRequestsPerDay} requisições hoje`)
  }
  return parts.join(' · ')
}

export const videoStatusKey = ['video', 'status'] as const
export const videoCredentialsKey = ['video', 'credentials'] as const

// useVideoStatus rereads the status every 5 s while open and on each media:changed.
export function useVideoStatus(open: boolean) {
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: videoStatusKey,
    queryFn: videoStatus,
    enabled: open,
    refetchInterval: open ? 5000 : false,
    refetchOnWindowFocus: false,
    staleTime: 0,
    gcTime: 0,
  })

  useEffect(() => {
    if (!open) {
      return undefined
    }
    const on = (window as unknown as WailsWindow).runtime?.EventsOn
    if (!on) {
      return undefined
    }
    const off = on('media:changed', () => {
      void qc.invalidateQueries({ queryKey: videoStatusKey })
    })
    return () => {
      if (typeof off === 'function') {
        off()
      }
    }
  }, [open, qc])

  return q
}

// useVideoCredentials: ready is true only with both credential groups registered; null while loading.
export function useVideoCredentials(): { ready: boolean | null } {
  const q = useQuery({
    queryKey: videoCredentialsKey,
    queryFn: async () => {
      const [u, d] = await Promise.all([providerKeyStatus('screenscraper'), providerKeyStatus('screenscraperdev')])
      return u.configured && d.configured
    },
    refetchOnWindowFocus: false,
  })
  return { ready: q.data ?? null }
}
