import { describe, expect, it } from 'vitest'
import { BIGBOX_THEMES, resolveTheme } from './registry'

describe('registro de temas do Big Box', () => {
  // alterado em 2026-11-22 (fase 15): mosaico e estante entram nas T34/T36; aqui só faixa (disponível) e roda (indisponível).
  it('BBX-023/o registro tem faixa e roda; só faixa disponível; roda sem componente', () => {
    const ids = BIGBOX_THEMES.map((t) => t.id)
    expect(ids).toContain('faixa')
    expect(ids).toContain('roda')
    const by = Object.fromEntries(BIGBOX_THEMES.map((t) => [t.id, t]))
    expect(by.faixa.available).toBe(true)
    expect(by.faixa.Component).toBeTypeOf('function')
    expect(by.roda.available).toBe(false)
    expect(by.roda.Component).toBeUndefined()
    expect(by.roda.name).not.toBe('')
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
