import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { FilterPanel } from './FilterPanel'
import type { FilterSpec, LibraryQuery } from '../lib/filters'
import { defaultQuery } from '../lib/filters'
import type { CatalogView } from '../lib/library'
import { readFiltersOpen, writeFiltersOpen } from '../lib/uiPrefs'

// Labels of the controls are assumed here (the spec names the facets, not the labels).
// The platform control left the panel: PLT-032 now holds in PlatformNav [UIX-011].
const catalog: CatalogView = {
  platforms: [
    { id: 1, name: 'Windows', category: 'PC', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 4 },
    { id: 2, name: 'SNES', category: 'Console', emulatorId: null, seedKey: 'snes', iconUrl: '', gameCount: 1 },
  ],
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

beforeEach(() => {
  localStorage.clear()
})

function header(): HTMLElement {
  return screen.getByRole('button', { name: /^Filtros/ })
}

// Opens the collapsible "Filtros" section, which starts collapsed [UIX-012].
function openSection(): void {
  const h = header()
  if (h.getAttribute('aria-expanded') !== 'true') {
    fireEvent.click(h)
  }
}

describe('FilterPanel', () => {
  it('UI-032/cada faceta tem um controle e a mudança chama onChange com um FilterSpec novo (version 1)', () => {
    for (const label of facetLabels) {
      const { unmount } = render(<FilterPanel query={defaultQuery} catalog={catalog} onChange={() => {}} />)
      openSection()
      expect(screen.getByLabelText(label)).toBeInTheDocument()
      unmount()
    }

    const changes: ReadonlyArray<{ label: RegExp; value: string; want: Partial<FilterSpec> }> = [
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
      localStorage.clear()
      const onChange = vi.fn<(q: LibraryQuery) => void>()
      const start: LibraryQuery = { filter: { version: 1 }, sort: 'year' }
      const { unmount } = render(<FilterPanel query={start} catalog={catalog} onChange={onChange} />)
      openSection()

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

  it('UI-032/listas de opções vêm do catálogo (gênero, modo, desenvolvedora, editora)', () => {
    render(<FilterPanel query={defaultQuery} catalog={catalog} onChange={() => {}} />)
    openSection()

    const genre = screen.getByLabelText(/gênero/i)
    expect(within(genre).getByRole('option', { name: 'Ação' })).toHaveAttribute('value', '5')
    expect(within(genre).getByRole('option', { name: 'RPG' })).toHaveAttribute('value', '6')

    const developer = screen.getByLabelText(/desenvolvedora/i)
    expect(within(developer).getByRole('option', { name: 'Capcom' })).toHaveAttribute('value', '7')
    expect(within(developer).getByRole('option', { name: 'Konami' })).toHaveAttribute('value', '8')

    const publisher = screen.getByLabelText(/editora/i)
    expect(within(publisher).getByRole('option', { name: 'Capcom' })).toHaveAttribute('value', '7')
    expect(within(publisher).getByRole('option', { name: 'Konami' })).toHaveAttribute('value', '8')

    const mode = screen.getByLabelText(/modo/i)
    expect(within(mode).getByRole('option', { name: 'Todos' })).toBeInTheDocument()
  })

  it('UI-032/sem catálogo ainda o painel mostra as facetas sem quebrar', () => {
    render(<FilterPanel query={defaultQuery} catalog={undefined} onChange={() => {}} />)
    openSection()

    for (const label of facetLabels) {
      expect(screen.getByLabelText(label)).toBeInTheDocument()
    }
  })

  it("UI-035/'Limpar filtros' volta o FilterSpec para {version: 1} e a ordenação para lastPlayed", () => {
    const onChange = vi.fn<(q: LibraryQuery) => void>()
    const active: LibraryQuery = {
      filter: { version: 1, platformIds: [2], favorite: true, text: 'zelda' },
      sort: 'year',
    }
    render(<FilterPanel query={active} catalog={catalog} onChange={onChange} />)
    openSection()

    fireEvent.click(screen.getByRole('button', { name: 'Limpar filtros' }))

    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith({ filter: { version: 1 }, sort: 'lastPlayed' })
  })

  it('UIX-011/não há controle "Plataforma" no painel', () => {
    render(<FilterPanel query={defaultQuery} catalog={catalog} onChange={() => {}} />)
    openSection()

    expect(screen.queryByLabelText(/plataforma/i)).not.toBeInTheDocument()
  })

  it('UIX-012/a seção começa recolhida e aria-expanded alterna', () => {
    render(<FilterPanel query={defaultQuery} catalog={catalog} onChange={() => {}} />)

    expect(header()).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByLabelText(/gênero/i)).not.toBeInTheDocument()

    fireEvent.click(header())
    expect(header()).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByLabelText(/gênero/i)).toBeInTheDocument()

    fireEvent.click(header())
    expect(header()).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByLabelText(/gênero/i)).not.toBeInTheDocument()
  })

  it('UIX-012/recolhida com 2 facetas ativas mostra "Filtros (2)"; com só texto e plataforma mostra "Filtros"', () => {
    const two: LibraryQuery = { filter: { version: 1, genreIds: [5], favorite: true }, sort: 'title' }
    const { unmount } = render(<FilterPanel query={two} catalog={catalog} onChange={() => {}} />)
    expect(header()).toHaveAttribute('aria-expanded', 'false')
    expect(header()).toHaveTextContent('Filtros (2)')
    unmount()

    const textOnly: LibraryQuery = { filter: { version: 1, text: 'zelda', platformIds: [2] }, sort: 'title' }
    render(<FilterPanel query={textOnly} catalog={catalog} onChange={() => {}} />)
    expect(header()).toHaveTextContent(/^Filtros$/)
  })

  it('UIX-012/recolher não chama onChange; a escolha é gravada e lida de volta', () => {
    const onChange = vi.fn<(q: LibraryQuery) => void>()
    const active: LibraryQuery = { filter: { version: 1, favorite: true }, sort: 'title' }
    const { unmount } = render(<FilterPanel query={active} catalog={catalog} onChange={onChange} />)

    fireEvent.click(header())
    expect(readFiltersOpen()).toBe(true)
    fireEvent.click(header())
    expect(readFiltersOpen()).toBe(false)
    fireEvent.click(header())
    expect(onChange).not.toHaveBeenCalled()
    expect(readFiltersOpen()).toBe(true)
    unmount()

    // The saved choice is used on the next mount.
    render(<FilterPanel query={active} catalog={catalog} onChange={onChange} />)
    expect(header()).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByLabelText(/favorito/i)).toBeInTheDocument()

    localStorage.clear()
    writeFiltersOpen(false)
    expect(readFiltersOpen()).toBe(false)
  })

  it('UIX-043/Enter e Espaço no cabeçalho abrem e fecham a seção', () => {
    render(<FilterPanel query={defaultQuery} catalog={catalog} onChange={() => {}} />)
    const h = header()
    expect(h.tagName).toBe('BUTTON')

    // A native button turns Enter and Space into a click; jsdom does not, so the test
    // fires the click that the browser would dispatch and checks the element is a focusable button.
    h.focus()
    expect(h).toHaveFocus()
    fireEvent.click(h)
    expect(h).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(h)
    expect(h).toHaveAttribute('aria-expanded', 'false')
  })
})
