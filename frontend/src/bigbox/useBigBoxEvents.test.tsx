import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useBigBoxEvents } from './useBigBoxEvents'
import { bringToFront } from '../lib/bigbox'

// Tests of the Wails events of the Big Box (phase 13, T40; BBX-018, CON-022, CON-024).
// Assumed signature: useBigBoxEvents({ dispatch, launchedGameId, active }) in ./useBigBoxEvents and
// bringToFront() exported by lib/bigbox. Assumed payloads: session:changed = SessionView, game:closedByHotkey = { gameId }.

let handlers: Map<string, Set<(...d: unknown[]) => void>>
let bring: ReturnType<typeof vi.fn>

beforeEach(() => {
  handlers = new Map()
  bring = vi.fn(() => Promise.resolve())
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: { BringToFront: bring } } }
  w.runtime = {
    EventsOn: vi.fn((name: string, cb: (...d: unknown[]) => void) => {
      let set = handlers.get(name)
      if (!set) {
        set = new Set()
        handlers.set(name, set)
      }
      set.add(cb)
      return () => {
        set!.delete(cb)
      }
    }),
  }
})

afterEach(() => {
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
})

function emit(name: string, ...data: unknown[]) {
  act(() => {
    for (const cb of [...(handlers.get(name) ?? [])]) cb(...data)
  })
}

const ended = (gameId: number, state = 'ended') => ({ sessionId: 1, gameId, title: 't', state })

function setup(o: { launchedGameId?: number | null; active?: boolean } = {}) {
  const dispatch = vi.fn()
  const r = renderHook((p: { launchedGameId: number | null; active: boolean }) => useBigBoxEvents({ dispatch, ...p }), {
    initialProps: { launchedGameId: 'launchedGameId' in o ? (o.launchedGameId as number | null) : 20, active: o.active ?? true },
  })
  return { dispatch, ...r }
}

describe('useBigBoxEvents', () => {
  it('BBX-018/blur e focus da janela viram windowBlur e windowFocus', () => {
    const { dispatch } = setup()
    act(() => {
      window.dispatchEvent(new Event('blur'))
    })
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    expect(dispatch.mock.calls.map((c) => (c[0] as { type: string }).type)).toEqual(['windowBlur', 'windowFocus'])
  })

  it('BBX-018/session:changed ended ou discarded do jogo lançado vira sessionEnded', () => {
    const { dispatch } = setup({ launchedGameId: 20 })
    emit('session:changed', ended(20))
    emit('session:changed', ended(20, 'discarded'))
    expect(dispatch).toHaveBeenCalledTimes(2)
    expect(dispatch).toHaveBeenNthCalledWith(1, { type: 'sessionEnded', gameId: 20 })
    expect(dispatch).toHaveBeenNthCalledWith(2, { type: 'sessionEnded', gameId: 20 })
  })

  it('BBX-018/sessão de outro jogo, ou ainda em andamento, não vira nada', () => {
    const { dispatch } = setup({ launchedGameId: 20 })
    emit('session:changed', ended(99))
    emit('session:changed', ended(20, 'running'))
    emit('session:changed', ended(20, 'paused'))
    expect(dispatch).not.toHaveBeenCalled()
  })

  it('BBX-018/sem jogo lançado nenhuma sessão vira sessionEnded', () => {
    const { dispatch } = setup({ launchedGameId: null })
    emit('session:changed', ended(20))
    expect(dispatch.mock.calls.filter((c) => (c[0] as { type: string }).type === 'sessionEnded')).toHaveLength(0)
  })

  it('CON-022/game:closedByHotkey vira gameClosed com o gameId do evento', () => {
    const { dispatch } = setup({ launchedGameId: 20 })
    emit('game:closedByHotkey', { gameId: 20 })
    expect(dispatch).toHaveBeenCalledTimes(1)
    expect(dispatch).toHaveBeenCalledWith(expect.objectContaining({ type: 'gameClosed', gameId: 20 }))
  })

  it('CON-022/gameId de outro jogo também vira gameClosed', () => {
    const { dispatch } = setup({ launchedGameId: 20 })
    emit('game:closedByHotkey', { gameId: 99 })
    expect(dispatch).toHaveBeenCalledWith(expect.objectContaining({ type: 'gameClosed', gameId: 99 }))
  })

  it('CON-022/inativo: nada é assinado e nada é despachado', () => {
    const { dispatch } = setup({ active: false })
    expect([...handlers.values()].reduce((n, s) => n + s.size, 0)).toBe(0)
    emit('game:closedByHotkey', { gameId: 20 })
    emit('session:changed', ended(20))
    act(() => {
      window.dispatchEvent(new Event('blur'))
    })
    expect(dispatch).not.toHaveBeenCalled()
  })

  it('CON-022/ao desmontar as assinaturas e os ouvintes da janela são desfeitos', () => {
    const { dispatch, unmount } = setup()
    expect(handlers.get('game:closedByHotkey')?.size).toBe(1)
    expect(handlers.get('session:changed')?.size).toBe(1)
    unmount()
    expect([...handlers.values()].reduce((n, s) => n + s.size, 0)).toBe(0)
    act(() => {
      window.dispatchEvent(new Event('blur'))
    })
    expect(dispatch).not.toHaveBeenCalled()
  })

  it('CON-022/trocar active de true para false desfaz as assinaturas', () => {
    const { rerender } = setup()
    expect(handlers.get('game:closedByHotkey')?.size).toBe(1)
    rerender({ launchedGameId: 20, active: false })
    expect([...handlers.values()].reduce((n, s) => n + s.size, 0)).toBe(0)
  })
})

describe('bringToFront', () => {
  it('CON-022/chama o binding BringToFront', async () => {
    await bringToFront()
    expect(bring).toHaveBeenCalledTimes(1)
  })

  it('CON-022/ignora a falha do binding', async () => {
    bring.mockRejectedValue('falhou')
    await expect(bringToFront()).resolves.toBeUndefined()
  })

  it('CON-022/ignora a falta do binding', async () => {
    ;(window as unknown as { go: { main: { App: object } } }).go.main.App = {}
    await expect(bringToFront()).resolves.toBeUndefined()
  })
})
