import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { EmptyResults } from './EmptyResults'

describe('EmptyResults', () => {
  it('UI-035/mostra a mensagem Nenhum jogo com estes filtros. e o botão Limpar filtros', () => {
    render(<EmptyResults onClear={() => {}} />)

    expect(screen.getByText('Nenhum jogo com estes filtros.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Limpar filtros' })).toBeInTheDocument()
  })

  it('UI-035/clicar em Limpar filtros chama onClear uma vez', () => {
    const onClear = vi.fn()
    render(<EmptyResults onClear={onClear} />)

    fireEvent.click(screen.getByRole('button', { name: 'Limpar filtros' }))

    expect(onClear).toHaveBeenCalledTimes(1)
  })
})
