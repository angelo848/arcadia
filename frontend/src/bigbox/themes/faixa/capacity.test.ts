import { describe, expect, it } from 'vitest'
import { faixaCapacity } from './capacity'

describe('faixaCapacity', () => {
  it('BBX-024/1920×1080 e 1280×720 dão o valor da fórmula de bigbox-faixa §3.1 (ver lacuna dos recuos laterais)', () => {
    // V = max(1, floor((W − 0,25c + g) / (c + g))), W = 0,9 × width, c = 0,18 × height × 2/3, g = 16.
    // Sem recuos: 1920×1080 -> 11; 1280×720 -> 11. Os recuos da faixa (não numerados na spec) podem tirar 1.
    const a = faixaCapacity({ width: 1920, height: 1080 })
    const b = faixaCapacity({ width: 1280, height: 720 })
    expect(a).toBeGreaterThanOrEqual(10)
    expect(a).toBeLessThanOrEqual(11)
    expect(b).toBeGreaterThanOrEqual(10)
    expect(b).toBeLessThanOrEqual(11)
  })

  it('BBX-024/2560×1440 (a tela do usuário) dá 12', () => {
    expect(faixaCapacity({ width: 2560, height: 1440 })).toBe(12)
  })

  it('BBX-024/0×0 (sem tamanho conhecido) dá 12', () => {
    expect(faixaCapacity({ width: 0, height: 0 })).toBe(12)
  })

  it('BBX-024/janela estreita dá 1', () => {
    expect(faixaCapacity({ width: 200, height: 1080 })).toBe(1)
    expect(faixaCapacity({ width: 1, height: 1 })).toBe(1)
  })

  it('BBX-024/nunca passa de 14', () => {
    expect(faixaCapacity({ width: 7680, height: 1080 })).toBe(14)
    expect(faixaCapacity({ width: 30000, height: 400 })).toBe(14)
  })
})
