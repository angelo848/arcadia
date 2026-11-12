import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SearchBox } from './SearchBox'
import type { Timers } from '../lib/debounce'

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

function input(): HTMLInputElement {
  return screen.getByRole('searchbox') as HTMLInputElement
}

describe('SearchBox', () => {
  it('UI-033/SearchBox envia a consulta 300 ms depois da última tecla e apagar o texto remove o filtro', () => {
    const onChange = vi.fn<(text: string) => void>()
    render(<SearchBox text="" onChange={onChange} />)

    fireEvent.change(input(), { target: { value: 'h' } })
    vi.advanceTimersByTime(100)
    fireEvent.change(input(), { target: { value: 'ha' } })
    vi.advanceTimersByTime(100)
    fireEvent.change(input(), { target: { value: 'hab' } })

    // Nothing is sent while the user keeps typing, and not before 300 ms after the last key.
    vi.advanceTimersByTime(299)
    expect(onChange).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenLastCalledWith('hab')

    // Erasing the text sends the empty string, which removes the text filter (FLT-007).
    fireEvent.change(input(), { target: { value: '' } })
    vi.advanceTimersByTime(299)
    expect(onChange).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(1)
    expect(onChange).toHaveBeenCalledTimes(2)
    expect(onChange).toHaveBeenLastCalledWith('')
  })

  it('UI-033/a caixa mostra o texto digitado na hora, antes da espera', () => {
    const onChange = vi.fn<(text: string) => void>()
    render(<SearchBox text="" onChange={onChange} />)

    fireEvent.change(input(), { target: { value: 'zel' } })

    expect(input().value).toBe('zel')
    expect(onChange).not.toHaveBeenCalled()
  })

  it('UI-033/usa o relógio injetado com 300 ms', () => {
    const onChange = vi.fn<(text: string) => void>()
    const timers: Timers = {
      setTimeout: vi.fn((f: () => void, ms: number) => setTimeout(f, ms)),
      clearTimeout: vi.fn((h: unknown) => clearTimeout(h as ReturnType<typeof setTimeout>)),
    }
    render(<SearchBox text="" onChange={onChange} timers={timers} />)

    fireEvent.change(input(), { target: { value: 'x' } })

    expect(timers.setTimeout).toHaveBeenLastCalledWith(expect.any(Function), 300)
  })

  it('UI-033/desmontar cancela a consulta pendente', () => {
    const onChange = vi.fn<(text: string) => void>()
    const { unmount } = render(<SearchBox text="" onChange={onChange} />)

    fireEvent.change(input(), { target: { value: 'zelda' } })
    unmount()
    vi.advanceTimersByTime(1000)

    expect(onChange).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })
})
