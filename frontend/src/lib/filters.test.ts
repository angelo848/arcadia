import { describe, expect, it } from 'vitest'
import { defaultQuery, resetQuery, withPlatforms, withText } from './filters'
import type { FilterSpec, GameStatus, LibraryQuery, SortKey } from './filters'

describe('filters', () => {
  it('UI-032/cada faceta entra no FilterSpec e lista vazia ou nula não aparece', () => {
    const statuses: GameStatus[] = ['playing', 'backlog']
    const filter: FilterSpec = {
      version: 1,
      platformIds: [1, 2],
      genreIds: [5],
      developerIds: [7],
      publisherIds: [9],
      playMode: 'multi',
      yearFrom: 1998,
      yearTo: 2005,
      statuses,
      favorite: true,
      played: 'played',
      playSecondsMin: 3600,
      playSecondsMax: 7200,
    }
    const q: LibraryQuery = { filter, sort: 'title' }

    // Cada faceta preenchida chega ao FilterSpec com o mesmo valor (plataforma, gênero, dev, editora, modo,
    // ano, status, favorito, jogado, faixa de tempo).
    expect(withText(q, '').filter).toStrictEqual(filter)
    expect(withText(q, '').filter.platformIds).toEqual([1, 2])
    expect(withText(q, '').filter.statuses).toEqual(['playing', 'backlog'])
    expect(withText(q, '').filter.playMode).toBe('multi')

    // Sem nenhuma faceta preenchida, o FilterSpec padrão tem só `version` (lista vazia ou nula não aparece).
    expect(Object.keys(defaultQuery.filter)).toEqual(['version'])
    expect(JSON.stringify(defaultQuery.filter)).toBe('{"version":1}')
  })

  it('UI-035/resetQuery volta a {version: 1} e sort lastPlayed', () => {
    const r = resetQuery()
    expect(r).toStrictEqual({ filter: { version: 1 }, sort: 'lastPlayed' })
    expect(r).toStrictEqual(defaultQuery)

    // Devolve uma cópia nova: alterar o resultado não muda o padrão nem a próxima chamada.
    r.filter.platformIds = [1]
    r.filter.text = 'hades'
    r.sort = 'year'
    expect(resetQuery()).toStrictEqual({ filter: { version: 1 }, sort: 'lastPlayed' })
    expect(defaultQuery).toStrictEqual({ filter: { version: 1 }, sort: 'lastPlayed' })
  })

  it('UI-033/withText com texto vazio ou só espaços tira o filtro text', () => {
    const base: LibraryQuery = { filter: { version: 1, text: 'hades' }, sort: 'title' }

    for (const blank of ['', '   ', '\t \n']) {
      const q = withText(base, blank)
      expect(q, `withText(${JSON.stringify(blank)})`).toStrictEqual({ filter: { version: 1 }, sort: 'title' })
      expect(q.filter).not.toHaveProperty('text')
    }

    // Entrada não é alterada.
    expect(base.filter.text).toBe('hades')

    // Texto com conteúdo define o filtro text.
    expect(withText(defaultQuery, 'hades').filter.text).toBe('hades')
  })

  it('UI-034/withText e troca de sort preservam os outros filtros', () => {
    const sorts: SortKey[] = ['title', 'lastPlayed', 'totalPlayed', 'year']
    const q: LibraryQuery = {
      filter: {
        version: 1,
        platformIds: [1, 2],
        genreIds: [5],
        playMode: 'multi',
        yearFrom: 2000,
        yearTo: null,
        statuses: ['playing'],
        favorite: true,
        played: 'played',
        playSecondsMin: 3600,
        playSecondsMax: 7200,
      },
      sort: 'title',
    }
    const others: FilterSpec = { ...q.filter }

    // Busca preserva todos os outros membros e o sort.
    const searched = withText(q, 'hades')
    expect(searched.filter).toStrictEqual({ ...others, text: 'hades' })
    expect(searched.sort).toBe('title')

    // Trocar a ordenação (sem tocar no filtro) mantém o FilterSpec, inclusive o texto da busca.
    for (const s of sorts) {
      const resorted: LibraryQuery = { ...searched, sort: s }
      expect(resorted.sort).toBe(s)
      expect(resorted.filter).toStrictEqual({ ...others, text: 'hades' })
    }

    // Limpar o texto depois da troca de sort mantém os outros filtros.
    const cleared = withText({ ...searched, sort: 'year' }, '  ')
    expect(cleared).toStrictEqual({ filter: others, sort: 'year' })
  })

  it('UIX-010/withPlatforms troca platformIds e mantém text, as outras facetas e sort; undefined remove o membro', () => {
    const q: LibraryQuery = {
      filter: { version: 1, platformIds: [1], text: 'hades', genreIds: [5], favorite: true, yearFrom: 2000 },
      sort: 'year',
    }
    const snapshot = structuredClone(q)

    const swapped = withPlatforms(q, [2])
    expect(swapped).toStrictEqual({
      filter: { version: 1, platformIds: [2], text: 'hades', genreIds: [5], favorite: true, yearFrom: 2000 },
      sort: 'year',
    })
    // The input is not mutated.
    expect(q).toStrictEqual(snapshot)

    const removed = withPlatforms(q, undefined)
    expect(removed.filter).not.toHaveProperty('platformIds')
    expect(removed).toStrictEqual({
      filter: { version: 1, text: 'hades', genreIds: [5], favorite: true, yearFrom: 2000 },
      sort: 'year',
    })

    expect(withPlatforms(defaultQuery, [3])).toStrictEqual({ filter: { version: 1, platformIds: [3] }, sort: 'lastPlayed' })
    expect(withPlatforms(defaultQuery, undefined)).toStrictEqual({ filter: { version: 1 }, sort: 'lastPlayed' })
  })
})
