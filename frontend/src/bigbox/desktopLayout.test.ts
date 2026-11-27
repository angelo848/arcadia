// T05: TDK-005, TDK-007, TDK-034 (contrato bigbox-tema-desktop §3 e §9).
import { describe, expect, it } from 'vitest'
import * as gridNav from '../lib/gridNav'
import * as layout from './desktopLayout'

const { desktopGrid } = layout

describe('desktopLayout (fase 17)', () => {
  it.each([
    [1920, 1080, 'grid', 6, 2],
    [1366, 768, 'grid', 4, 1],
    [1280, 720, 'grid', 3, 1],
    [3840, 2160, 'grid', 17, 6],
    [1920, 1080, 'list', 1, 13],
    [1280, 720, 'list', 1, 7],
    [0, 0, 'grid', 4, 2],
    [0, 0, 'list', 1, 8],
  ] as const)('TDK-007/desktopGrid %i x %i %s = %i x %i', (width, height, view, columns, rows) => {
    expect(desktopGrid({ width, height }, view)).toEqual({ columns, rows })
  })

  it('TDK-007/largura ou altura ≤ 0 usa o padrão', () => {
    for (const vp of [{ width: 0, height: 900 }, { width: 1600, height: 0 }, { width: -5, height: -5 }]) {
      expect(desktopGrid(vp, 'grid')).toEqual({ columns: 4, rows: 2 })
      expect(desktopGrid(vp, 'list')).toEqual({ columns: 1, rows: 8 })
    }
  })

  it('TDK-007/rows nunca é menor que 1', () => {
    for (const view of ['grid', 'list'] as const) {
      for (const height of [1, 100, 201, 250]) {
        expect(desktopGrid({ width: 1000, height }, view).rows).toBeGreaterThanOrEqual(1)
      }
    }
    expect(desktopGrid({ width: 100, height: 100 }, 'grid').columns).toBeGreaterThanOrEqual(1)
  })

  it('TDK-034/os valores de lib/gridNav são reexportados com a mesma referência', () => {
    expect(layout.columnsFor).toBe(gridNav.columnsFor)
    expect(layout.usableWidth).toBe(gridNav.usableWidth)
    expect(layout.GRID_GAP).toBe(gridNav.GRID_GAP)
    expect(layout.GRID_INSET).toBe(gridNav.GRID_INSET)
    expect(layout.CARD_MIN_WIDTH).toBe(gridNav.CARD_MIN_WIDTH)
    expect(layout.CARD_TITLE_HEIGHT).toBe(gridNav.CARD_TITLE_HEIGHT)
    expect(layout.SCROLLBAR_SIZE).toBe(gridNav.SCROLLBAR_SIZE)
    expect(layout.LIST_ROW_HEIGHT).toBe(gridNav.LIST_ROW_HEIGHT)
  })

  it('TDK-007/constantes do cromo (65, 24, 16, 32, 12, 48, 12) e DESKTOP_MAX_NODES = 200', () => {
    expect([
      layout.HEADER_PX, layout.CENTER_PAD_X, layout.CENTER_PAD_Y, layout.TOOLBAR_PX, layout.TOOLBAR_GAP, layout.FOOTER_PX,
      layout.FOOTER_GAP,
    ]).toEqual([65, 24, 16, 32, 12, 48, 12])
    expect(layout.DESKTOP_MAX_NODES).toBe(200)
  })

  it('TDK-005/coverPlaceholderUrl é uma string não vazia', () => {
    expect(typeof layout.coverPlaceholderUrl).toBe('string')
    expect(layout.coverPlaceholderUrl.length).toBeGreaterThan(0)
  })
})
