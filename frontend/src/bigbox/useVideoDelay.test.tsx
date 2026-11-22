import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useVideoDelay } from './useVideoDelay'

// Assumed: useVideoDelay({ gameId, videoUrl, active, failed }): boolean, with the 2.000 ms of BBX-011 (design §4.3).
type P = { gameId: number | null; videoUrl: string | null; active: boolean; failed: boolean }
const base: P = { gameId: 1, videoUrl: 'http://m/1/v.mp4', active: true, failed: false }
const DELAY = 2000

beforeEach(() => {
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
})

const setup = (p: P = base) => renderHook((o: P) => useVideoDelay(o), { initialProps: p })
const advance = (ms: number) => act(() => { vi.advanceTimersByTime(ms) })

describe('useVideoDelay', () => {
  it('BBX-053/false antes de 2 s e true depois de 2.000 ms com active', () => {
    const { result } = setup()
    expect(result.current).toBe(false)
    advance(DELAY - 1)
    expect(result.current).toBe(false)
    advance(1)
    expect(result.current).toBe(true)
  })

  it('BBX-053/failed mantém false, mesmo depois de muito tempo', () => {
    const { result } = setup({ ...base, failed: true })
    advance(DELAY * 3)
    expect(result.current).toBe(false)
  })

  it('BBX-053/depois de um erro (failed passa a true) volta a false e não tenta de novo enquanto o foco não sair', () => {
    const { result, rerender } = setup()
    advance(DELAY)
    expect(result.current).toBe(true)
    rerender({ ...base, failed: true })
    expect(result.current).toBe(false)
    advance(DELAY * 3)
    expect(result.current).toBe(false)
  })

  it('BBX-053/trocar de jogo reinicia a contagem: volta a false e só fica true 2.000 ms depois', () => {
    const { result, rerender } = setup()
    advance(DELAY)
    expect(result.current).toBe(true)
    rerender({ ...base, gameId: 2, videoUrl: 'http://m/2/v.mp4' })
    expect(result.current).toBe(false)
    advance(DELAY - 1)
    expect(result.current).toBe(false)
    advance(1)
    expect(result.current).toBe(true)
  })

  it('BBX-053/troca de jogo antes de 2 s: o tempo do jogo anterior não conta', () => {
    const { result, rerender } = setup()
    advance(1500)
    rerender({ ...base, gameId: 2, videoUrl: 'http://m/2/v.mp4' })
    advance(1500)
    expect(result.current).toBe(false)
    advance(500)
    expect(result.current).toBe(true)
  })

  it('BBX-053/active false (Detalhes ou suspensão) nunca libera e desliga um true anterior', () => {
    const { result, rerender } = setup({ ...base, active: false })
    advance(DELAY * 3)
    expect(result.current).toBe(false)
    rerender({ ...base, active: true })
    advance(DELAY)
    expect(result.current).toBe(true)
    rerender({ ...base, active: false })
    expect(result.current).toBe(false)
  })

  it('BBX-053/sem vídeo ou sem jogo em foco nunca libera', () => {
    const a = setup({ ...base, videoUrl: null })
    advance(DELAY * 2)
    expect(a.result.current).toBe(false)
    a.unmount()
    const b = setup({ ...base, gameId: null })
    advance(DELAY * 2)
    expect(b.result.current).toBe(false)
  })

  it('BBX-053/desmontar limpa o temporizador (nada pendente)', () => {
    const { unmount } = setup()
    advance(500)
    unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})
