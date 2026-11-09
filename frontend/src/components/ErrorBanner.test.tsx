import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ErrorBanner } from './ErrorBanner'

describe('ErrorBanner', () => {
  it('UI-008 mostra a mensagem e Fechar chama onClose', () => {
    const onClose = vi.fn()
    const msg = 'Já há um jogo em andamento: Hollow Knight. Feche-o antes de iniciar outro.'
    render(<ErrorBanner message={msg} onClose={onClose} />)

    expect(screen.getByText(msg)).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Fechar' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
