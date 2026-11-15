import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { hintsFor } from './hints'
import { Footer } from './Footer'

// Assumed props: { hints: Hint[] } (the model's hints, rendered as text).
describe('Footer', () => {
  it('BBX-015/o rodapé mostra as dicas do modelo em texto', () => {
    const hints = hintsFor('browse', { empty: false, videoVisible: true })
    render(<Footer hints={hints} />)
    for (const h of hints) {
      expect(screen.getByText(h.button)).toBeInTheDocument()
      expect(screen.getByText(h.label)).toBeInTheDocument()
    }
    expect(screen.getByText('Som')).toBeInTheDocument()
  })

  it('BBX-021/aba vazia: só LB/RB e B', () => {
    render(<Footer hints={hintsFor('browse', { empty: true, videoVisible: false })} />)
    expect(screen.getByText('LB/RB')).toBeInTheDocument()
    expect(screen.getByText('B')).toBeInTheDocument()
    expect(screen.queryByText('Jogar')).toBeNull()
    expect(screen.queryByText('Detalhes')).toBeNull()
  })

  it('BBX-015/sem o Y quando não há vídeo', () => {
    render(<Footer hints={hintsFor('browse', { empty: false, videoVisible: false })} />)
    expect(screen.queryByText('Y')).toBeNull()
  })
})
