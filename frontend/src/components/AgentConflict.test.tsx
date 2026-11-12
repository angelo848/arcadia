import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentConflict } from './AgentConflict'

describe('AgentConflict', () => {
  it('ADM-006/mostra o aviso de agente sem privilégio de administrador e a saída', () => {
    render(<AgentConflict />)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Já há um agente rodando sem privilégio de administrador.')
    expect(alert).toHaveTextContent('Saia dele pela bandeja e abra o Arcadia de novo como administrador.')
    expect(alert).not.toHaveTextContent('Iniciar com o Windows')
  })
})
