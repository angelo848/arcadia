import { describe, expect, it } from 'vitest'
import {
  activeFacetCount,
  readBigBoxTheme,
  readFiltersOpen,
  writeBigBoxTheme,
  readLibraryView,
  writeFiltersOpen,
  writeLibraryView,
} from './uiPrefs'
import type { FilterSpec } from './filters'

function memoryStorage() {
  const data = new Map<string, string>()
  return {
    data,
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => {
      data.set(k, v)
    },
  }
}

const throwing = {
  getItem: (): string | null => {
    throw new Error('storage unavailable')
  },
  setItem: (): void => {
    throw new Error('storage unavailable')
  },
}

describe('uiPrefs', () => {
  it('UIX-020/readLibraryView e writeLibraryView usam a chave arcadia.libraryView (preferência antiga continua valendo)', () => {
    const s = memoryStorage()
    expect(readLibraryView(s)).toBe('grid')
    s.data.set('arcadia.libraryView', 'list')
    expect(readLibraryView(s)).toBe('list')

    const w = memoryStorage()
    writeLibraryView('list', w)
    expect(w.data.get('arcadia.libraryView')).toBe('list')
    writeLibraryView('grid', w)
    expect(w.data.get('arcadia.libraryView')).toBe('grid')
    expect(readLibraryView(throwing)).toBe('grid')
    expect(() => writeLibraryView('list', throwing)).not.toThrow()
  })

  it('UIX-012/readFiltersOpen é false sem valor gravado e com o storage indisponível', () => {
    expect(readFiltersOpen(memoryStorage())).toBe(false)
    expect(readFiltersOpen(throwing)).toBe(false)
  })

  it('UIX-012/writeFiltersOpen(true) é lido de volta como true', () => {
    const s = memoryStorage()
    writeFiltersOpen(true, s)
    expect(s.data.has('arcadia.filtersOpen')).toBe(true)
    expect(readFiltersOpen(s)).toBe(true)
    writeFiltersOpen(false, s)
    expect(readFiltersOpen(s)).toBe(false)
    expect(() => writeFiltersOpen(true, throwing)).not.toThrow()
  })

  it('UIX-012/activeFacetCount ignora version, text e platformIds', () => {
    expect(activeFacetCount({ version: 1 })).toBe(0)
    expect(activeFacetCount({ version: 1, text: 'zelda', platformIds: [1, 2] })).toBe(0)
  })

  it('UIX-012/activeFacetCount conta cada faceta presente (gênero, modo, empresas, ano de/até, status, favorito false, jogado, tempo mín/máx)', () => {
    const facets: Array<[string, Partial<FilterSpec>]> = [
      ['gênero', { genreIds: [1] }],
      ['modo', { playMode: 'single' }],
      ['desenvolvedora', { developerIds: [2] }],
      ['editora', { publisherIds: [3] }],
      ['ano de', { yearFrom: 1990 }],
      ['ano até', { yearTo: 2000 }],
      ['status', { statuses: ['playing'] }],
      ['favorito false', { favorite: false }],
      ['favorito true', { favorite: true }],
      ['jogado', { played: 'played' }],
      ['tempo mínimo', { playSecondsMin: 60 }],
      ['tempo máximo', { playSecondsMax: 3600 }],
    ]
    for (const [name, f] of facets) {
      expect(activeFacetCount({ version: 1, ...f }), name).toBe(1)
    }
    expect(
      activeFacetCount({ version: 1, genreIds: [1], statuses: ['playing'], favorite: false, text: 'x', platformIds: [4] }),
    ).toBe(3)
  })
})

// alterado em 2026-11-15 (fase 13); alterado em 2026-11-22 (fase 15): mosaico e estante disponíveis, só roda indisponível
describe('uiPrefs: tema do Big Box', () => {
  it('BBX-023/BBX-046/valor ausente, desconhecido ou de tema indisponível vira faixa', () => {
    const s = memoryStorage()
    expect(readBigBoxTheme(s)).toBe('faixa')
    for (const v of ['', 'xyz', 'roda']) {
      s.data.set('arcadia.bigBoxTheme', v)
      expect(readBigBoxTheme(s), v).toBe('faixa')
    }
    s.data.set('arcadia.bigBoxTheme', 'faixa')
    expect(readBigBoxTheme(s)).toBe('faixa')
  })

  it('BBX-046/mosaico e estante são aceitos (temas disponíveis)', () => {
    const s = memoryStorage()
    for (const v of ['mosaico', 'estante'] as const) {
      s.data.set('arcadia.bigBoxTheme', v)
      expect(readBigBoxTheme(s), v).toBe(v)
    }
    const w = memoryStorage()
    writeBigBoxTheme('estante', w)
    expect(w.data.get('arcadia.bigBoxTheme')).toBe('estante')
  })

  // alterado em 2026-11-27 (fase 17)
  it('BBX-060/readBigBoxTheme lê "desktop" e writeBigBoxTheme o grava; ausente, desconhecido e "roda" viram faixa', () => {
    const s = memoryStorage()
    s.data.set('arcadia.bigBoxTheme', 'desktop')
    expect(readBigBoxTheme(s)).toBe('desktop')
    const w = memoryStorage()
    writeBigBoxTheme('desktop', w)
    expect(w.data.get('arcadia.bigBoxTheme')).toBe('desktop')
    expect(readBigBoxTheme(memoryStorage())).toBe('faixa')
    for (const v of ['', 'xyz', 'roda']) {
      const x = memoryStorage()
      x.data.set('arcadia.bigBoxTheme', v)
      expect(readBigBoxTheme(x), v).toBe('faixa')
    }
  })

  it('BBX-023/a escolha fica em localStorage na chave arcadia.bigBoxTheme', () => {
    const w = memoryStorage()
    writeBigBoxTheme('faixa', w)
    expect(w.data.get('arcadia.bigBoxTheme')).toBe('faixa')
    window.localStorage.removeItem('arcadia.bigBoxTheme')
    writeBigBoxTheme('faixa')
    expect(window.localStorage.getItem('arcadia.bigBoxTheme')).toBe('faixa')
    expect(readBigBoxTheme()).toBe('faixa')
  })

  it('BBX-023/storage indisponível não quebra', () => {
    expect(readBigBoxTheme(throwing)).toBe('faixa')
    expect(() => writeBigBoxTheme('faixa', throwing)).not.toThrow()
  })
})
