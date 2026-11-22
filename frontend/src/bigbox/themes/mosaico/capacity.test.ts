import { describe, expect, it } from 'vitest'
import { mosaicoCapacity } from './capacity'

// temas-v2 §5.2: W = 0,9 x width; s = 0,30 x height; g = 16; extra = 0,09 s + 18; V = floor((W - extra + g) / (s + g)), 1..14.
describe('mosaicoCapacity', () => {
  it('MOS-012/1280×720 e 1920×1080 dão 4', () => {
    expect(mosaicoCapacity({ width: 1280, height: 720 })).toBe(4)
    expect(mosaicoCapacity({ width: 1920, height: 1080 })).toBe(4)
  })

  it('MOS-012/3840×2160 dá 5 e 2560×1080 dá 6', () => {
    expect(mosaicoCapacity({ width: 3840, height: 2160 })).toBe(5)
    expect(mosaicoCapacity({ width: 2560, height: 1080 })).toBe(6)
  })

  it('MOS-012/tamanho desconhecido (0×0, largura ou altura 0) dá 12', () => {
    expect(mosaicoCapacity({ width: 0, height: 0 })).toBe(12)
    expect(mosaicoCapacity({ width: 0, height: 1080 })).toBe(12)
    expect(mosaicoCapacity({ width: 1920, height: 0 })).toBe(12)
  })

  it('MOS-012/limitada a 1..14', () => {
    expect(mosaicoCapacity({ width: 100, height: 1000 })).toBe(1)
    expect(mosaicoCapacity({ width: 1, height: 1 })).toBe(1)
    expect(mosaicoCapacity({ width: 30000, height: 400 })).toBe(14)
  })
})
