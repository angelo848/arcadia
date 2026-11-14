import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { LibraryToolbar } from './LibraryToolbar'

function toolbar(over: Partial<Parameters<typeof LibraryToolbar>[0]> = {}) {
  const props = {
    total: 3 as number | undefined,
    sort: 'title' as const,
    view: 'grid' as const,
    onSortChange: vi.fn(),
    onViewChange: vi.fn(),
    ...over,
  }
  render(<LibraryToolbar {...props} />)
  return props
}

describe('LibraryToolbar', () => {
  it('UIX-013/mostra "N jogos" com total > 0 e nenhum contador com total 0 ou undefined', () => {
    const a = render(
      <LibraryToolbar total={3} sort="title" view="grid" onSortChange={vi.fn()} onViewChange={vi.fn()} />,
    )
    expect(screen.getByText('3 jogos')).toBeInTheDocument()
    a.unmount()

    for (const total of [0, undefined]) {
      const r = render(
        <LibraryToolbar total={total} sort="title" view="grid" onSortChange={vi.fn()} onViewChange={vi.fn()} />,
      )
      expect(screen.queryByText(/\d+ jogos?/)).toBeNull()
      r.unmount()
    }
  })

  it('UIX-013/ordenação chama onSortChange e a alternância chama onViewChange', () => {
    const props = toolbar()
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'year' } })
    expect(props.onSortChange).toHaveBeenCalledTimes(1)
    expect(props.onSortChange).toHaveBeenCalledWith('year')

    fireEvent.click(screen.getByRole('button', { name: 'Mostrar em lista' }))
    expect(props.onViewChange).toHaveBeenCalledTimes(1)
    expect(props.onViewChange).toHaveBeenCalledWith('list')
  })
})
