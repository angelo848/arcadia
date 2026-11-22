import { describe, expect, it } from 'vitest'
import { BIGBOX_THEMES, resolveTheme } from './registry'

describe('registro de temas do Big Box', () => {
  // alterado em 2026-11-22 (fase 15): estado final de BBX-046 (faixa, mosaico, estante disponíveis; roda indisponível).
  it('BBX-023/BBX-046/o registro lista faixa, mosaico e estante, nesta ordem, disponíveis, e roda indisponível sem componente', () => {
    expect(BIGBOX_THEMES.map((t) => t.id)).toEqual(['faixa', 'mosaico', 'estante', 'roda'])
    expect(BIGBOX_THEMES.map((t) => t.name)).toEqual(['Faixa', 'Mosaico', 'Estante', 'Roda'])
    const by = Object.fromEntries(BIGBOX_THEMES.map((t) => [t.id, t]))
    for (const id of ['faixa', 'mosaico', 'estante']) {
      expect(by[id].available, id).toBe(true)
      expect(by[id].Component, id).toBeTypeOf('function')
    }
    expect(by.roda.available).toBe(false)
    expect(by.roda.Component).toBeUndefined()
  })

  it('BBX-046/BBX-052/BBX-054/BBX-055/valores da Estante: grade de 5 colunas, fundo cover, vídeo no fundo, abas de coleção, por último jogado, Detalhes próprios', () => {
    const e = BIGBOX_THEMES.find((t) => t.id === 'estante')!
    expect(e).toMatchObject({
      available: true, axis: 'grid', columns: 5, backdrop: 'cover', videoSlot: 'background', tabs: 'collections', sort: 'lastPlayed',
    })
    expect(e.gridRows).toBeTypeOf('function')
    expect(e.Details).toBeTypeOf('function')
    expect(e.listCapacity({ width: 1920, height: 1080 })).toBe(5)
  })

  it('BBX-052/o Mosaico usa fundo plain e o Faixa contain', () => {
    expect(BIGBOX_THEMES.find((t) => t.id === 'mosaico')!.backdrop).toBe('plain')
    expect(BIGBOX_THEMES.find((t) => t.id === 'faixa')!.backdrop).toBe('contain')
  })

  it('BBX-047/cada tema declara id, name, available, axis, columns, anchor, listCapacity, backdrop, videoSlot, tabs, sort e timeToBeatInBrowse', () => {
    expect(BIGBOX_THEMES.length).toBeGreaterThan(0)
    for (const t of BIGBOX_THEMES) {
      expect(t.id, t.id).toBeTypeOf('string')
      expect(t.name, t.id).not.toBe('')
      expect(t.available, t.id).toBeTypeOf('boolean')
      expect(['horizontal', 'vertical', 'grid'], t.id).toContain(t.axis)
      expect(t.columns, t.id).toBeGreaterThanOrEqual(1)
      expect(Number.isInteger(t.anchor), t.id).toBe(true)
      expect(t.listCapacity, t.id).toBeTypeOf('function')
      expect(['contain', 'cover', 'plain'], t.id).toContain(t.backdrop)
      expect(['background', 'cover'], t.id).toContain(t.videoSlot)
      expect(['platforms', 'collections'], t.id).toContain(t.tabs)
      expect(['title', 'lastPlayed'], t.id).toContain(t.sort)
      expect(t.timeToBeatInBrowse, t.id).toBeTypeOf('boolean')
      if (t.axis === 'grid') expect(t.gridRows, t.id).toBeTypeOf('function')
    }
  })

  it('BBX-047/valores do Faixa: horizontal, 1 coluna, âncora 2, contain, vídeo no fundo, abas de plataforma, por título, sem tempo para zerar', () => {
    const f = BIGBOX_THEMES.find((t) => t.id === 'faixa')!
    expect(f).toMatchObject({
      available: true, axis: 'horizontal', columns: 1, anchor: 2, backdrop: 'contain', videoSlot: 'background',
      tabs: 'platforms', sort: 'title', timeToBeatInBrowse: false,
    })
    expect(f.Details).toBeUndefined()
  })

  it('BBX-047/valores do Roda: vertical, 1 coluna, âncora 2, indisponível', () => {
    const r = BIGBOX_THEMES.find((t) => t.id === 'roda')!
    expect(r).toMatchObject({
      available: false, axis: 'vertical', columns: 1, anchor: 2, backdrop: 'contain', videoSlot: 'background',
      tabs: 'platforms', sort: 'title', timeToBeatInBrowse: false,
    })
  })

  it('BBX-023/valor ausente, desconhecido ou de tema indisponível vira faixa', () => {
    for (const v of [undefined, null, '', 'xyz', 'roda']) {
      expect(resolveTheme(v as string | null | undefined).id, String(v)).toBe('faixa')
    }
    expect(resolveTheme('faixa').id).toBe('faixa')
    // alterado em 2026-11-22 (fase 15): mosaico e estante estão disponíveis (BBX-046)
    expect(resolveTheme('mosaico').id).toBe('mosaico')
    expect(resolveTheme('estante').id).toBe('estante')
  })

  it('BBX-024/cada tema declara o eixo (faixa horizontal)', () => {
    for (const t of BIGBOX_THEMES) expect(['horizontal', 'vertical', 'grid']).toContain(t.axis)
    expect(BIGBOX_THEMES.find((t) => t.id === 'faixa')?.axis).toBe('horizontal')
  })

  it('BBX-024/todo tema do registro tem listCapacity; roda devolve 12', () => {
    for (const t of BIGBOX_THEMES) expect(t.listCapacity, t.id).toBeTypeOf('function')
    const t = BIGBOX_THEMES.find((x) => x.id === 'roda')!
    expect(t.listCapacity({ width: 1920, height: 1080 })).toBe(12)
    expect(t.listCapacity({ width: 0, height: 0 })).toBe(12)
    const f = BIGBOX_THEMES.find((x) => x.id === 'faixa')!
    expect(f.listCapacity({ width: 0, height: 0 })).toBe(12)
  })

  it('BBX-024/resolveTheme devolve o tema de teste vertical quando registrado (tema falso)', () => {
    const fake = BIGBOX_THEMES.find((t) => t.id === 'roda')!
    const saved = { ...fake }
    fake.available = true
    fake.axis = 'vertical'
    fake.Component = () => null as never
    try {
      const got = resolveTheme('roda')
      expect(got.id).toBe('roda')
      expect(got.axis).toBe('vertical')
    } finally {
      Object.assign(fake, saved)
      if (!('Component' in saved)) delete fake.Component
    }
  })
})
