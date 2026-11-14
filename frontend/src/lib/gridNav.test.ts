import { describe, expect, it } from 'vitest'
import { CARD_MIN_WIDTH, CARD_TITLE_HEIGHT, GRID_GAP, columnsFor, isNavKey, nextIndex } from './gridNav'

// 3 columns, 8 loaded: rows are [0 1 2] [3 4 5] [6 7] (the last one is shorter).
const COLS = 3
const LOADED = 8

describe('nextIndex', () => {
  it('UIX-019/direita e esquerda vão ao vizinho; na primeira e na última coluna da linha ficam', () => {
    expect(nextIndex(4, 'ArrowRight', COLS, LOADED)).toBe(5)
    expect(nextIndex(4, 'ArrowLeft', COLS, LOADED)).toBe(3)
    // Last column of a row and first column of a row stay.
    expect(nextIndex(5, 'ArrowRight', COLS, LOADED)).toBe(5)
    expect(nextIndex(3, 'ArrowLeft', COLS, LOADED)).toBe(3)
    expect(nextIndex(0, 'ArrowLeft', COLS, LOADED)).toBe(0)
    // Last card of the short row.
    expect(nextIndex(7, 'ArrowRight', COLS, LOADED)).toBe(7)
  })

  it('UIX-019/baixo vai à mesma coluna; para uma linha mais curta vai ao último cartão dela; na última linha fica', () => {
    expect(nextIndex(1, 'ArrowDown', COLS, LOADED)).toBe(4)
    expect(nextIndex(4, 'ArrowDown', COLS, LOADED)).toBe(7)
    // Column 2 does not exist in the short last row: goes to its last card.
    expect(nextIndex(5, 'ArrowDown', COLS, LOADED)).toBe(7)
    // Last row stays.
    expect(nextIndex(6, 'ArrowDown', COLS, LOADED)).toBe(6)
    expect(nextIndex(7, 'ArrowDown', COLS, LOADED)).toBe(7)
  })

  it('UIX-019/cima vai à mesma coluna; na primeira linha fica', () => {
    expect(nextIndex(7, 'ArrowUp', COLS, LOADED)).toBe(4)
    expect(nextIndex(4, 'ArrowUp', COLS, LOADED)).toBe(1)
    expect(nextIndex(0, 'ArrowUp', COLS, LOADED)).toBe(0)
    expect(nextIndex(2, 'ArrowUp', COLS, LOADED)).toBe(2)
  })

  it('UIX-019/Home vai a 0 e End a loaded - 1', () => {
    expect(nextIndex(5, 'Home', COLS, LOADED)).toBe(0)
    expect(nextIndex(2, 'End', COLS, LOADED)).toBe(LOADED - 1)
  })

  it('UIX-019/isNavKey aceita só as seis teclas', () => {
    for (const k of ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End']) {
      expect(isNavKey(k), k).toBe(true)
    }
    for (const k of ['Enter', ' ', 'Tab', 'PageDown', 'a', 'arrowleft', '']) {
      expect(isNavKey(k), k).toBe(false)
    }
  })

  it('UIX-020/com cols = 1 cima e baixo movem uma linha e esquerda e direita ficam', () => {
    expect(nextIndex(2, 'ArrowDown', 1, 5)).toBe(3)
    expect(nextIndex(2, 'ArrowUp', 1, 5)).toBe(1)
    expect(nextIndex(4, 'ArrowDown', 1, 5)).toBe(4)
    expect(nextIndex(0, 'ArrowUp', 1, 5)).toBe(0)
    expect(nextIndex(2, 'ArrowLeft', 1, 5)).toBe(2)
    expect(nextIndex(2, 'ArrowRight', 1, 5)).toBe(2)
  })
})

describe('columnsFor', () => {
  it('UIX-005/columnsFor(484) dá 2 colunas e columnsFor(1132) dá pelo menos 5', () => {
    expect(columnsFor(484).cols).toBe(2)
    expect(columnsFor(1132).cols).toBeGreaterThanOrEqual(5)
  })

  it('UIX-005/columnsFor de largura menor que um cartão dá 1 coluna', () => {
    expect(columnsFor(CARD_MIN_WIDTH - 1).cols).toBe(1)
    expect(columnsFor(0).cols).toBe(1)
  })

  it('UIX-014/rowHeight = cardWidth × 3/2 + CARD_TITLE_HEIGHT + GRID_GAP', () => {
    for (const w of [300, 484, 800, 1132]) {
      const { cardWidth, rowHeight } = columnsFor(w)
      expect(rowHeight).toBeCloseTo((cardWidth * 3) / 2 + CARD_TITLE_HEIGHT + GRID_GAP, 5)
    }
  })
})
