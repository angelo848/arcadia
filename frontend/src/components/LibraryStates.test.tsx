import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LibraryStates } from './LibraryStates'

type StatesProps = Parameters<typeof LibraryStates>[0]

function states(over: Partial<StatesProps> = {}): ReturnType<typeof render> {
  const props: StatesProps = {
    loaded: true,
    total: 0,
    filtered: false,
    ...over,
  }
  return render(<LibraryStates {...props} />)
}

describe('LibraryStates', () => {
  it('UI-044/sem resposta ainda mostra Conectando ao agente…', () => {
    states({ loaded: false, total: undefined, filtered: false })

    expect(screen.getByText('Conectando ao agente…')).toBeInTheDocument()
    // Still loading is not an empty library.
    expect(screen.queryByText('Nenhum jogo ainda')).not.toBeInTheDocument()
  })

  it("UI-044/total 0 sem filtro mostra 'Nenhum jogo ainda' e a dica de Importar", () => {
    states({ loaded: true, total: 0, filtered: false })

    expect(screen.getByText('Nenhum jogo ainda')).toBeInTheDocument()
    expect(screen.queryByText('Nenhum jogo com estes filtros.')).not.toBeInTheDocument()
    expect(screen.getByText('Use Importar para adicionar jogos.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Adicionar jogo' })).not.toBeInTheDocument()
  })

  it('UI-044/total > 0 mostra N jogos e nenhum contador de plataformas', () => {
    states({ loaded: true, total: 3, filtered: false })

    expect(screen.getByText('3 jogos')).toBeInTheDocument()
    expect(screen.queryByText(/plataforma/i)).not.toBeInTheDocument()
    expect(screen.queryByText('Nenhum jogo ainda')).not.toBeInTheDocument()
  })

  it("UI-035/total 0 com filtro mostra 'Nenhum jogo com estes filtros.' e o botão 'Limpar filtros'", () => {
    states({ loaded: true, total: 0, filtered: true })

    expect(screen.getByText('Nenhum jogo com estes filtros.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Limpar filtros' })).toBeInTheDocument()
    // With a filter active the library-empty message does not apply (UI-035).
    expect(screen.queryByText('Nenhum jogo ainda')).not.toBeInTheDocument()
  })

  it('UI-044/contagem usa plural fixo também para 1 jogo', () => {
    states({ loaded: true, total: 1, filtered: false })

    expect(screen.getByText('1 jogos')).toBeInTheDocument()
  })
})
