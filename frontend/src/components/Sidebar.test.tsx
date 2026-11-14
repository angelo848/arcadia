import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { Sidebar } from './Sidebar'
import type { LibraryQuery } from '../lib/filters'
import { defaultQuery, withPlatforms } from '../lib/filters'
import type { CatalogView } from '../lib/library'
import type { Timers } from '../lib/debounce'

const catalog: CatalogView = {
  platforms: [
    { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 4 },
    { id: 2, name: 'SNES', category: 'console', emulatorId: null, seedKey: 'snes', iconUrl: '', gameCount: 1 },
  ],
  genres: [{ id: 5, name: 'Ação' }],
  companies: [{ id: 7, name: 'Capcom' }],
  playModes: [],
}

beforeEach(() => {
  localStorage.clear()
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

function setup(over: { query?: LibraryQuery; search?: string; timers?: Timers } = {}) {
  const onSearchChange = vi.fn<(text: string) => void>()
  const onChange = vi.fn<(q: LibraryQuery) => void>()
  const utils = render(
    <Sidebar
      search={over.search ?? ''}
      onSearchChange={onSearchChange}
      query={over.query ?? defaultQuery}
      catalog={catalog}
      onChange={onChange}
      timers={over.timers}
    />,
  )
  return { ...utils, onSearchChange, onChange }
}

function filtersButton(): HTMLElement {
  return screen.getByRole('button', { name: /^Filtros/ })
}

describe('Sidebar', () => {
  it('UIX-008/o primeiro controle da coluna é a busca; espera de 300 ms com o relógio injetável; apagar remove text', () => {
    const timers: Timers = {
      setTimeout: vi.fn((f: () => void, ms: number) => setTimeout(f, ms)),
      clearTimeout: vi.fn((h: unknown) => clearTimeout(h as ReturnType<typeof setTimeout>)),
    }
    const { container, onSearchChange } = setup({ timers })

    const first = container.querySelector<HTMLElement>('input, button, select, textarea, a[href], [tabindex]')
    expect(first).toBe(screen.getByRole('searchbox'))

    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'hades' } })
    expect(timers.setTimeout).toHaveBeenLastCalledWith(expect.any(Function), 300)
    vi.advanceTimersByTime(299)
    expect(onSearchChange).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(onSearchChange).toHaveBeenCalledTimes(1)
    expect(onSearchChange).toHaveBeenLastCalledWith('hades')

    // Erasing sends '', which removes the text member upstream.
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '' } })
    vi.advanceTimersByTime(300)
    expect(onSearchChange).toHaveBeenCalledTimes(2)
    expect(onSearchChange).toHaveBeenLastCalledWith('')
  })

  it('UIX-010/clicar numa plataforma chama onChange com platformIds=[P] e mantém as outras facetas', () => {
    const q: LibraryQuery = { filter: { version: 1, genreIds: [5], favorite: true, text: 'z' }, sort: 'year' }
    const { onChange, unmount } = setup({ query: q })

    fireEvent.click(screen.getByRole('button', { name: 'SNES, 1 jogos' }))

    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith(withPlatforms(q, [2]))
    expect(onChange.mock.calls[0][0]).toStrictEqual({
      filter: { version: 1, genreIds: [5], favorite: true, text: 'z', platformIds: [2] },
      sort: 'year',
    })
    unmount()

    // "Todos" removes platformIds and keeps the other facets.
    const again = setup({ query: { filter: { version: 1, platformIds: [2], genreIds: [5] }, sort: 'title' } })
    fireEvent.click(screen.getByRole('button', { name: /^Todos/ }))
    expect(again.onChange).toHaveBeenCalledWith({ filter: { version: 1, genreIds: [5] }, sort: 'title' })
  })

  it('UIX-011/cada controle gera o mesmo FilterSpec de antes; não há controle "Plataforma"', () => {
    const { onChange } = setup()

    fireEvent.click(filtersButton())
    expect(screen.queryByLabelText(/^plataforma$/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/gênero/i), { target: { value: '5' } })
    expect(onChange).toHaveBeenLastCalledWith({ filter: { version: 1, genreIds: [5] }, sort: 'title' })

    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'playing' } })
    expect(onChange).toHaveBeenLastCalledWith({ filter: { version: 1, statuses: ['playing'] }, sort: 'title' })

    fireEvent.click(screen.getByRole('button', { name: 'Limpar filtros' }))
    expect(onChange).toHaveBeenLastCalledWith({ filter: { version: 1 }, sort: 'title' })
  })

  it('UIX-012/a seção começa recolhida e aria-expanded alterna', () => {
    setup()

    expect(filtersButton()).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(filtersButton())
    expect(filtersButton()).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(filtersButton())
    expect(filtersButton()).toHaveAttribute('aria-expanded', 'false')
  })

  it('UIX-012/recolhida com 2 facetas ativas mostra "Filtros (2)"; com só texto e plataforma mostra "Filtros"', () => {
    const { unmount } = setup({ query: { filter: { version: 1, genreIds: [5], favorite: true }, sort: 'title' } })
    expect(filtersButton()).toHaveTextContent('Filtros (2)')
    unmount()

    setup({ query: { filter: { version: 1, text: 'z', platformIds: [1] }, sort: 'title' }, search: 'z' })
    expect(filtersButton()).toHaveTextContent(/^Filtros$/)
  })

  it('UIX-012/recolher não chama onChange; a escolha é gravada e lida de volta', () => {
    const { onChange, unmount } = setup()

    fireEvent.click(filtersButton())
    fireEvent.click(filtersButton())
    fireEvent.click(filtersButton())
    expect(onChange).not.toHaveBeenCalled()
    unmount()

    setup()
    expect(filtersButton()).toHaveAttribute('aria-expanded', 'true')
  })

  it('UIX-043/Enter e Espaço no cabeçalho abrem e fecham a seção', () => {
    setup()
    const h = filtersButton()

    // The header is a native, focusable button: the browser turns Enter and Space into click.
    expect(h.tagName).toBe('BUTTON')
    h.focus()
    expect(h).toHaveFocus()
    fireEvent.click(h)
    expect(h).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(h)
    expect(h).toHaveAttribute('aria-expanded', 'false')
  })
})
