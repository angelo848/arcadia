import { describe, expect, it } from 'vitest'
import { BIGBOX_THEMES, resolveTheme } from './registry'

describe('registro de temas do Big Box', () => {
  it('BBX-023/o registro tem faixa, mosaico e roda; só faixa disponível; mosaico e roda sem componente', () => {
    expect(BIGBOX_THEMES.map((t) => t.id)).toEqual(['faixa', 'mosaico', 'roda'])
    const by = Object.fromEntries(BIGBOX_THEMES.map((t) => [t.id, t]))
    expect(by.faixa.available).toBe(true)
    expect(by.faixa.Component).toBeTypeOf('function')
    for (const id of ['mosaico', 'roda']) {
      expect(by[id].available, id).toBe(false)
      expect(by[id].Component, id).toBeUndefined()
      expect(by[id].name, id).not.toBe('')
    }
  })

  it('BBX-023/valor ausente, desconhecido ou de tema indisponível vira faixa', () => {
    for (const v of [undefined, null, '', 'xyz', 'mosaico', 'roda']) {
      expect(resolveTheme(v as string | null | undefined).id, String(v)).toBe('faixa')
    }
    expect(resolveTheme('faixa').id).toBe('faixa')
  })

  it('BBX-024/cada tema declara o eixo (faixa horizontal)', () => {
    for (const t of BIGBOX_THEMES) expect(['horizontal', 'vertical']).toContain(t.axis)
    expect(BIGBOX_THEMES.find((t) => t.id === 'faixa')?.axis).toBe('horizontal')
  })

  it('BBX-024/todo tema do registro tem listCapacity; mosaico e roda devolvem 12', () => {
    for (const t of BIGBOX_THEMES) expect(t.listCapacity, t.id).toBeTypeOf('function')
    for (const id of ['mosaico', 'roda']) {
      const t = BIGBOX_THEMES.find((x) => x.id === id)!
      expect(t.listCapacity({ width: 1920, height: 1080 }), id).toBe(12)
      expect(t.listCapacity({ width: 0, height: 0 }), id).toBe(12)
    }
    const f = BIGBOX_THEMES.find((x) => x.id === 'faixa')!
    expect(f.listCapacity({ width: 0, height: 0 })).toBe(12)
  })

  it('BBX-024/resolveTheme devolve o tema de teste vertical quando registrado (tema falso)', () => {
    const fake = BIGBOX_THEMES.find((t) => t.id === 'mosaico')!
    const saved = { ...fake }
    fake.available = true
    fake.axis = 'vertical'
    fake.Component = () => null as never
    try {
      const got = resolveTheme('mosaico')
      expect(got.id).toBe('mosaico')
      expect(got.axis).toBe('vertical')
    } finally {
      Object.assign(fake, saved)
      if (!('Component' in saved)) delete fake.Component
    }
  })
})
