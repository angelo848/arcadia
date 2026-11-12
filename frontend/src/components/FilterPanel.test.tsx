import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { FilterPanel } from './FilterPanel'
import type { FilterSpec, LibraryQuery } from '../lib/filters'
import { defaultQuery } from '../lib/filters'
import type { CatalogView } from '../lib/library'

// Labels of the controls are assumed here (the spec names the facets, not the labels);
// see the ambiguity notes in the T30 report.
const catalog: CatalogView = {
  platforms: [
    { id: 1, name: 'Windows', category: 'PC', emulatorId: null },
    { id: 2, name: 'SNES', category: 'Console', emulatorId: null },
  ],
  // Genres and companies are {id, name} items; the ids are the database ids the
  // FilterSpec filters by (FLT-002, FLT-003), and the option value is that id.
  genres: [
    { id: 5, name: 'Ação' },
    { id: 6, name: 'RPG' },
  ],
  companies: [
    { id: 7, name: 'Capcom' },
    { id: 8, name: 'Konami' },
  ],
  playModes: ['single', 'multi_local', 'multi_online', 'coop_local', 'coop_online'],
}

const facetLabels: ReadonlyArray<RegExp> = [
  /plataforma/i,
  /gênero/i,
  /modo/i,
  /desenvolvedora/i,
  /editora/i,
  /ano de/i,
  /ano até/i,
  /status/i,
  /favorito/i,
  /jogado/i,
  /mínimo/i,
  /máximo/i,
]

describe('FilterPanel', () => {
  it('UI-032/cada faceta tem um controle e a mudança chama onChange com um FilterSpec novo (version 1)', () => {
    for (const label of facetLabels) {
      const { unmount } = render(<FilterPanel query={defaultQuery} catalog={catalog} onChange={() => {}} />)
      expect(screen.getByLabelText(label)).toBeInTheDocument()
      unmount()
    }

    // Each change below must produce a new FilterSpec with version 1 and the sort kept.
    const changes: ReadonlyArray<{ label: RegExp; value: string; want: Partial<FilterSpec> }> = [
      { label: /plataforma/i, value: '2', want: { platformIds: [2] } },
      { label: /gênero/i, value: '5', want: { genreIds: [5] } },
      { label: /desenvolvedora/i, value: '7', want: { developerIds: [7] } },
      { label: /editora/i, value: '8', want: { publisherIds: [8] } },
      { label: /modo/i, value: 'multi', want: { playMode: 'multi' } },
      { label: /status/i, value: 'playing', want: { statuses: ['playing'] } },
      { label: /favorito/i, value: 'true', want: { favorite: true } },
      { label: /jogado/i, value: 'played', want: { played: 'played' } },
      { label: /ano de/i, value: '2000', want: { yearFrom: 2000 } },
      { label: /ano até/i, value: '2005', want: { yearTo: 2005 } },
      { label: /mínimo/i, value: '1', want: { playSecondsMin: 3600 } },
      { label: /máximo/i, value: '2', want: { playSecondsMax: 7200 } },
    ]

    for (const tc of changes) {
      const onChange = vi.fn<(q: LibraryQuery) => void>()
      const start: LibraryQuery = { filter: { version: 1 }, sort: 'year' }
      const { unmount } = render(<FilterPanel query={start} catalog={catalog} onChange={onChange} />)

      fireEvent.change(screen.getByLabelText(tc.label), { target: { value: tc.value } })

      expect(onChange).toHaveBeenCalledTimes(1)
      const next = onChange.mock.calls[0][0]
      expect(next.filter.version).toBe(1)
      expect(next.filter).toEqual({ version: 1, ...tc.want })
      expect(next.filter).not.toBe(start.filter)
      expect(next.sort).toBe('year')
      unmount()
    }
  })

  it('UI-032/listas de opções vêm do catálogo (plataforma, gênero, modo, desenvolvedora, editora)', () => {
    render(<FilterPanel query={defaultQuery} catalog={catalog} onChange={() => {}} />)

    const platform = screen.getByLabelText(/plataforma/i)
    expect(within(platform).getByRole('option', { name: 'Windows' })).toBeInTheDocument()
    expect(within(platform).getByRole('option', { name: 'SNES' })).toBeInTheDocument()

    const genre = screen.getByLabelText(/gênero/i)
    expect(within(genre).getByRole('option', { name: 'Ação' })).toHaveAttribute('value', '5')
    expect(within(genre).getByRole('option', { name: 'RPG' })).toHaveAttribute('value', '6')

    // Developers and publishers both come from the catalog companies list.
    const developer = screen.getByLabelText(/desenvolvedora/i)
    expect(within(developer).getByRole('option', { name: 'Capcom' })).toHaveAttribute('value', '7')
    expect(within(developer).getByRole('option', { name: 'Konami' })).toHaveAttribute('value', '8')

    const publisher = screen.getByLabelText(/editora/i)
    expect(within(publisher).getByRole('option', { name: 'Capcom' })).toHaveAttribute('value', '7')
    expect(within(publisher).getByRole('option', { name: 'Konami' })).toHaveAttribute('value', '8')

    // Play mode group control exists (FLT-003 group values).
    const mode = screen.getByLabelText(/modo/i)
    expect(within(mode).getByRole('option', { name: 'Todos' })).toBeInTheDocument()
  })

  it('UI-032/sem catálogo ainda o painel mostra as facetas sem quebrar', () => {
    render(<FilterPanel query={defaultQuery} catalog={undefined} onChange={() => {}} />)

    for (const label of facetLabels) {
      expect(screen.getByLabelText(label)).toBeInTheDocument()
    }
  })

  it("UI-035/'Limpar filtros' volta o FilterSpec para {version: 1} e a ordenação para title", () => {
    const onChange = vi.fn<(q: LibraryQuery) => void>()
    const active: LibraryQuery = {
      filter: { version: 1, platformIds: [2], favorite: true, text: 'zelda' },
      sort: 'year',
    }
    render(<FilterPanel query={active} catalog={catalog} onChange={onChange} />)

    fireEvent.click(screen.getByRole('button', { name: 'Limpar filtros' }))

    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith({ filter: { version: 1 }, sort: 'title' })
  })
})
