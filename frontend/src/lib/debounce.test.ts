import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createDebouncer } from './debounce'

describe('createDebouncer', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('UI-033/a consulta só é aplicada 300 ms depois da última tecla', () => {
    const apply = vi.fn()
    const d = createDebouncer<string>(300, apply)

    d.call('h')
    vi.advanceTimersByTime(299)
    expect(apply).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(apply).toHaveBeenCalledTimes(1)
    expect(apply).toHaveBeenLastCalledWith('h')

    // Nova rodada: a contagem recomeça a partir da última tecla.
    d.call('ha')
    vi.advanceTimersByTime(299)
    expect(apply).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(1)
    expect(apply).toHaveBeenCalledTimes(2)
    expect(apply).toHaveBeenLastCalledWith('ha')
  })

  it('UI-033/uma tecla por vez não dispara consulta intermediária', () => {
    const apply = vi.fn()
    const d = createDebouncer<string>(300, apply)

    // Teclas a cada 100 ms: cada uma reinicia a espera, então nada é aplicado enquanto se digita.
    d.call('h')
    vi.advanceTimersByTime(100)
    d.call('ha')
    vi.advanceTimersByTime(100)
    d.call('hab')
    vi.advanceTimersByTime(100)
    d.call('habe')
    expect(apply).not.toHaveBeenCalled()

    vi.advanceTimersByTime(299)
    expect(apply).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)

    // Só o valor final é aplicado, uma única vez.
    expect(apply).toHaveBeenCalledTimes(1)
    expect(apply).toHaveBeenCalledWith('habe')
    expect(apply.mock.calls.map((c) => c[0])).not.toContain('h')
    expect(apply.mock.calls.map((c) => c[0])).not.toContain('ha')
    expect(apply.mock.calls.map((c) => c[0])).not.toContain('hab')
  })

  it('UI-033/cancel remove o temporizador pendente (sem aplicar após desmontar)', () => {
    const apply = vi.fn()
    const d = createDebouncer<string>(300, apply)

    d.call('x')
    expect(vi.getTimerCount()).toBe(1)
    d.cancel()
    expect(vi.getTimerCount()).toBe(0)

    vi.advanceTimersByTime(1000)
    expect(apply).not.toHaveBeenCalled()

    // Cancelar sem nada pendente não lança erro.
    expect(() => d.cancel()).not.toThrow()
  })
})
