import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { createElement } from 'react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  queueResultLabel,
  queuedLabel,
  useVideoCredentials,
  useVideoStatus,
  videoCancelPending,
  videoQueue,
  videoQueueMissing,
  videoStatus,
  videoStatusLine,
} from './video'
import type { VideoStatusView } from './video'

// Bindings Wails: window.go.main.App.<Nome> (T28). Events: window.runtime.EventsOn.
type Handler = (...d: unknown[]) => void
let handlers: Map<string, Set<Handler>>

function install(app: Record<string, unknown>) {
  handlers = new Map()
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
  ;(window as unknown as { runtime?: unknown }).runtime = {
    EventsOn: (name: string, cb: Handler) => {
      if (!handlers.has(name)) handlers.set(name, new Set())
      handlers.get(name)!.add(cb)
      return () => handlers.get(name)!.delete(cb)
    },
  }
}

function status(over: Partial<VideoStatusView> = {}): VideoStatusView {
  return { pending: 0, running: 0, failed: 0, lane: 'active', pausedUntil: '', requestsToday: 0, maxRequestsPerDay: 0, ...over }
}

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client: qc }, children)
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'setTimeout', 'clearTimeout'] })
})

afterEach(() => {
  vi.useRealTimers()
  delete (window as unknown as { go?: unknown }).go
  delete (window as unknown as { runtime?: unknown }).runtime
})

describe('lib/video bindings', () => {
  it('VID-024/cada função chama o binding certo e falha sem binding', async () => {
    const app = {
      VideoQueue: vi.fn(() => Promise.resolve({ queued: 1, skipped: [] })),
      VideoQueueMissing: vi.fn(() => Promise.resolve({ queued: 3, skipped: 2 })),
      VideoCancelPending: vi.fn(() => Promise.resolve(4)),
      VideoStatus: vi.fn(() => Promise.resolve(status({ pending: 2 }))),
    }
    install(app)
    expect(await videoQueue([7, 8])).toEqual({ queued: 1, skipped: [] })
    expect(app.VideoQueue).toHaveBeenCalledWith([7, 8])
    expect(await videoQueueMissing(0)).toEqual({ queued: 3, skipped: 2 })
    expect(app.VideoQueueMissing).toHaveBeenCalledWith(0)
    expect(await videoCancelPending()).toBe(4)
    expect(app.VideoCancelPending).toHaveBeenCalledTimes(1)
    expect((await videoStatus()).pending).toBe(2)
    expect(app.VideoStatus).toHaveBeenCalledTimes(1)

    install({})
    await expect(videoQueue([1])).rejects.toThrow(Error)
    await expect(videoQueueMissing(0)).rejects.toThrow(Error)
    await expect(videoCancelPending()).rejects.toThrow(Error)
    await expect(videoStatus()).rejects.toThrow(Error)
  })
})

describe('lib/video rótulos', () => {
  it('VID-024/queueResultLabel: queued 1 → "Vídeo na fila"; has-video; platform-unmapped', () => {
    expect(queueResultLabel({ queued: 1, skipped: [] })).toBe('Vídeo na fila')
    expect(queueResultLabel({ queued: 0, skipped: [{ gameId: 1, reason: 'has-video' }] })).toBe(
      'Este jogo já tem vídeo ou busca em andamento',
    )
    expect(queueResultLabel({ queued: 0, skipped: [{ gameId: 1, reason: 'platform-unmapped' }] })).toBe(
      'A plataforma não tem vídeo no ScreenScraper',
    )
  })

  it('SSC-003/queuedLabel singular e plural', () => {
    // Premissa: singular natural em pt-BR ("1 vídeo na fila"); a spec fixa só "N vídeos na fila".
    expect(queuedLabel(1)).toBe('1 vídeo na fila')
    expect(queuedLabel(3)).toBe('3 vídeos na fila')
    expect(queuedLabel(0)).toBe('0 vídeos na fila')
  })

  it('SSC-003/videoStatusLine: só pendentes e sem vídeo; cota; credenciais; requisições do dia; no-credentials', () => {
    const base = videoStatusLine(status({ pending: 5, failed: 2 }))
    expect(base).toContain('5 pendentes')
    expect(base).toContain('2 sem vídeo')
    expect(base).not.toContain('cota')
    expect(base).not.toContain('credenciais')
    expect(base).not.toContain('requisições')

    const quota = videoStatusLine(status({ lane: 'paused-quota', pausedUntil: new Date(2026, 10, 15, 9, 5).toISOString() }))
    expect(quota).toContain('cota esgotada até 09:05')

    expect(videoStatusLine(status({ lane: 'paused-credentials' }))).toContain('credenciais recusadas')
    expect(videoStatusLine(status({ requestsToday: 12, maxRequestsPerDay: 20000 }))).toContain('12 de 20000 requisições hoje')
    expect(videoStatusLine(status({ lane: 'no-credentials' }))).not.toContain('cota esgotada')
  })
})

describe('lib/video hooks', () => {
  it('SSC-003/useVideoStatus relê a cada 5 s com a janela aberta, para ao fechar e relê a cada media:changed', async () => {
    const VideoStatus = vi.fn(() => Promise.resolve(status()))
    install({ VideoStatus })
    const { rerender } = renderHook(({ open }) => useVideoStatus(open), {
      wrapper: wrapper(),
      initialProps: { open: true },
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    const first = VideoStatus.mock.calls.length
    expect(first).toBeGreaterThanOrEqual(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000)
    })
    expect(VideoStatus.mock.calls.length).toBe(first + 1)

    const before = VideoStatus.mock.calls.length
    await act(async () => {
      handlers.get('media:changed')?.forEach((h) => h({ gameId: 1 }))
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(VideoStatus.mock.calls.length).toBeGreaterThan(before)

    rerender({ open: false })
    const closed = VideoStatus.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(20000)
    })
    expect(VideoStatus.mock.calls.length).toBe(closed)
  })

  it('VID-024/useVideoCredentials é true só com as duas (null enquanto carrega)', async () => {
    const cases: Array<[boolean, boolean, boolean]> = [
      [true, true, true],
      [true, false, false],
      [false, true, false],
      [false, false, false],
    ]
    for (const [a, b, want] of cases) {
      install({
        ProviderKeyStatus: vi.fn((p: string) =>
          Promise.resolve({ provider: p, configured: p === 'screenscraper' ? a : b }),
        ),
      })
      const { result, unmount } = renderHook(() => useVideoCredentials(), { wrapper: wrapper() })
      expect(result.current.ready).toBeNull()
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      expect(result.current.ready).toBe(want)
      unmount()
    }
  })

  it('CON-024/os hooks só chamam leituras e Video*', async () => {
    const app = {
      ProviderKeyStatus: vi.fn((p: string) => Promise.resolve({ provider: p, configured: true })),
      ProviderSetKey: vi.fn(),
      ProviderDeleteKey: vi.fn(),
      LaunchGame: vi.fn(),
      LaunchGameUntracked: vi.fn(),
      VideoStatus: vi.fn(() => Promise.resolve(status())),
      VideoQueue: vi.fn(),
      VideoQueueMissing: vi.fn(),
      VideoCancelPending: vi.fn(),
    }
    install(app)
    renderHook(() => ({ c: useVideoCredentials(), s: useVideoStatus(true) }), { wrapper: wrapper() })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000)
    })
    for (const name of ['ProviderSetKey', 'ProviderDeleteKey', 'LaunchGame', 'LaunchGameUntracked', 'VideoQueue', 'VideoQueueMissing', 'VideoCancelPending'] as const) {
      expect(app[name], name).not.toHaveBeenCalled()
    }
  })
})
