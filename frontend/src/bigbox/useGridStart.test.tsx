import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useGridStart } from './useGridStart'
import type { GridStartInput } from './useGridStart'

// useGridStart keeps the first visible row of a grid theme [BBX-049, BBX-063]; the contract formulas are in
// docs/specs/phase-17-tema-desktop/contracts/bigbox-tema-desktop.md §4 and §5.

const base: GridStartInput = { mode: 'edges', focus: 0, total: 40, columns: 2, rows: 3, tabKey: 'all' } // 20 rows
const row = (r: number, c = 2) => r * c

function setup(first: Partial<GridStartInput> = {}) {
  return renderHook((p: GridStartInput) => useGridStart(p), { initialProps: { ...base, ...first } })
}

describe('useGridStart', () => {
  it('BBX-063/edges: andar dentro das fileiras inteiras visíveis não rola; passar da última rola uma fileira; passar da primeira também', () => {
    const h = setup()
    expect(h.result.current.startRow).toBe(0)
    for (const r of [1, 2]) {
      h.rerender({ ...base, focus: row(r) })
      expect(h.result.current.startRow, `fileira ${r}`).toBe(0)
    }
    h.rerender({ ...base, focus: row(3) }) // one past the last whole row
    expect(h.result.current.startRow).toBe(1)
    h.rerender({ ...base, focus: row(2) }) // inside [1, 3]
    expect(h.result.current.startRow).toBe(1)
    h.rerender({ ...base, focus: row(1) })
    expect(h.result.current.startRow).toBe(1)
    h.rerender({ ...base, focus: row(0) }) // above the first whole row
    expect(h.result.current.startRow).toBe(0)
  })

  it('BBX-063/troca de tabKey centraliza (centeredRow)', () => {
    const h = setup()
    h.rerender({ ...base, focus: row(6), tabKey: 'p:1' })
    expect(h.result.current.startRow).toBe(5) // 6 − ⌊(3 − 1) / 2⌋
    h.rerender({ ...base, focus: row(19), tabKey: 'p:2' })
    expect(h.result.current.startRow).toBe(17) // clamped to maxStart = 20 − 3
  })

  it('BBX-063/na entrada (primeiro cálculo) a fileira do foco fica no meio', () => {
    const h = setup({ focus: row(6) })
    expect(h.result.current.startRow).toBe(5)
  })

  it('BBX-063/recenter() centraliza no próximo cálculo e só nele', () => {
    const h = setup()
    h.rerender({ ...base, focus: row(2) })
    expect(h.result.current.startRow).toBe(0)

    act(() => h.result.current.recenter())
    h.rerender({ ...base, focus: row(2) })
    expect(h.result.current.startRow).toBe(1) // centeredRow(2, 3, 20)

    h.rerender({ ...base, focus: row(3) }) // back to the edges rule: still whole rows [1, 3]
    expect(h.result.current.startRow).toBe(1)
  })

  it('BBX-063/resize que muda R\' reaplica edgeScrollRow com a fileira guardada', () => {
    const h = setup({ focus: row(2) })
    expect(h.result.current.startRow).toBe(0)
    h.rerender({ ...base, focus: row(2), rows: 2 }) // only rows 0 and 1 are whole now
    expect(h.result.current.startRow).toBe(1)
    h.rerender({ ...base, focus: row(2), rows: 5 })
    expect(h.result.current.startRow).toBe(1) // the stored row is kept while the focus is inside
  })

  it('BBX-063/o limite de baixo é maxStart: não passa de ⌈T/C⌉ − R\'', () => {
    const h = setup({ total: 23, columns: 5, rows: 1, focus: 0 }) // 5 rows
    h.rerender({ ...base, total: 23, columns: 5, rows: 1, focus: 22 })
    expect(h.result.current.startRow).toBe(4)
  })

  it('BBX-049/focusTop devolve clamp(⌊f/C⌋, 0, max(⌈T/C⌉-R\', 0))', () => {
    const f = (focus: number, rows: number) => ({ mode: 'focusTop' as const, focus, total: 23, columns: 5, rows, tabKey: 'all' })
    const h = renderHook((p: GridStartInput) => useGridStart(p), { initialProps: f(0, 1) })
    expect(h.result.current.startRow).toBe(0)
    h.rerender(f(7, 1))
    expect(h.result.current.startRow).toBe(1)
    h.rerender(f(22, 1))
    expect(h.result.current.startRow).toBe(4)
    h.rerender(f(22, 2))
    expect(h.result.current.startRow).toBe(3) // max(5 − 2, 0)
    h.rerender(f(12, 2))
    expect(h.result.current.startRow).toBe(2)
    h.rerender({ ...f(0, 9), total: 0 })
    expect(h.result.current.startRow).toBe(0)
  })
})
