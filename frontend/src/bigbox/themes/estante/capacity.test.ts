import { describe, expect, it } from 'vitest'
import { BIGBOX_THEMES } from '../registry'
import { ESTANTE_COLUMNS, estanteRows } from './capacity'

// temas-v2 §6.2: C = 5, g = 24; w = (0,9 x width - 4 g) / 5; h = 1,5 w + 56; A = (0,9 - 0,16 - 0,07) x height;
// R = floor((A + g) / (h + g)), limitado a 1..5; tamanho desconhecido (width <= 0 ou height <= 0) dá 2.
describe('estanteRows', () => {
  it('ETN-008/1280×720 e 1920×1080 dão 1 fileira inteira', () => {
    expect(estanteRows({ width: 1280, height: 720 })).toBe(1)
    expect(estanteRows({ width: 1920, height: 1080 })).toBe(1)
  })

  it('ETN-008/3840×2160 e 1920×1200 (16:10) também dão 1', () => {
    expect(estanteRows({ width: 3840, height: 2160 })).toBe(1)
    expect(estanteRows({ width: 1920, height: 1200 })).toBe(1)
  })

  it('ETN-008/tamanho desconhecido (0×0, largura ou altura 0) dá 2', () => {
    expect(estanteRows({ width: 0, height: 0 })).toBe(2)
    expect(estanteRows({ width: 0, height: 1080 })).toBe(2)
    expect(estanteRows({ width: 1920, height: 0 })).toBe(2)
  })

  it('ETN-008/a fórmula passa de 1 quando a janela é bem mais alta que larga (vetores 2 e 3)', () => {
    expect(estanteRows({ width: 1000, height: 1400 })).toBe(2)
    expect(estanteRows({ width: 1000, height: 1500 })).toBe(3)
  })

  it('ETN-008/limitada a 1..5', () => {
    expect(estanteRows({ width: 1920, height: 100 })).toBe(1)
    expect(estanteRows({ width: 500, height: 10000 })).toBe(5)
    expect(estanteRows({ width: 800, height: 2000 })).toBe(5)
  })

  it('ETN-008/a grade tem 5 colunas e a capacidade do tema é 5 × R', () => {
    expect(ESTANTE_COLUMNS).toBe(5)
    const t = BIGBOX_THEMES.find((x) => x.id === 'estante')!
    expect(t.columns).toBe(5)
    for (const vp of [
      { width: 1280, height: 720 }, { width: 1920, height: 1080 }, { width: 3840, height: 2160 }, { width: 1920, height: 1200 },
      { width: 0, height: 0 }, { width: 1000, height: 1500 },
    ]) {
      expect(t.listCapacity(vp), JSON.stringify(vp)).toBe(5 * estanteRows(vp))
      expect(t.gridRows?.(vp), JSON.stringify(vp)).toBe(estanteRows(vp))
    }
    expect(t.listCapacity({ width: 1920, height: 1080 })).toBe(5)
    expect(t.listCapacity({ width: 0, height: 0 })).toBe(10)
  })
})
