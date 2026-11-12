import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { SortSelect } from './SortSelect'
import type { SortKey } from '../lib/filters'

const options: ReadonlyArray<{ label: string; key: SortKey }> = [
  { label: 'Título', key: 'title' },
  { label: 'Última vez jogado', key: 'lastPlayed' },
  { label: 'Tempo jogado', key: 'totalPlayed' },
  { label: 'Ano', key: 'year' },
]

function control(): HTMLElement {
  return screen.getByRole('combobox')
}

describe('SortSelect', () => {
  it('UI-034/escolher ordenação recarrega com o sort correspondente e volta para o início da página', () => {
    // The page reset lives above this component (the query changes); here we check the sort emitted.
    for (const opt of options) {
      const onChange = vi.fn<(s: SortKey) => void>()
      const { unmount } = render(<SortSelect sort="title" onChange={onChange} />)

      fireEvent.change(control(), { target: { value: opt.key } })

      expect(onChange).toHaveBeenCalledTimes(1)
      expect(onChange).toHaveBeenCalledWith(opt.key)
      unmount()
    }
  })

  it('UI-034/as quatro opções aparecem com os rótulos da spec, na ordem da spec', () => {
    render(<SortSelect sort="title" onChange={() => {}} />)

    const names = within(control())
      .getAllByRole('option')
      .map((o) => o.textContent)
    expect(names).toEqual(options.map((o) => o.label))
  })

  it('UI-034/a ordenação atual aparece selecionada', () => {
    render(<SortSelect sort="year" onChange={() => {}} />)

    expect(control()).toHaveValue('year')
  })
})
