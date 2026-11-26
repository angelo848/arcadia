import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SessionDeleteDialog } from './SessionDeleteDialog'
import type { SessionItemView } from '../lib/library'
import { formatLocalDateTime } from '../lib/formatDate'

// Phase 16 (edicao-sessoes), SEL-010..SEL-012: the confirmation of "Excluir" in the sessions list.
// The dialog is controlled: it never calls the agent; the parent (SessionsList) does on onConfirm.

function item(over: Partial<SessionItemView> = {}): SessionItemView {
  return {
    sessionId: 5,
    gameId: 10,
    state: 'ended',
    startedAt: '2026-11-12T20:00:00.000Z',
    endedAt: '2026-11-12T21:00:00.000Z',
    activeSeconds: 3600,
    endReason: 'exited',
    source: 'tracked',
    editedAt: '',
    ...over,
  }
}

function renderDialog(over: Partial<Parameters<typeof SessionDeleteDialog>[0]> = {}) {
  const props = {
    session: item(),
    pending: false,
    error: null as string | null,
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  }
  render(<SessionDeleteDialog {...props} />)
  return props
}

describe('SessionDeleteDialog', () => {
  it('SEL-012/texto da sessão contada com início e tempo jogado', () => {
    renderDialog()
    const start = formatLocalDateTime('2026-11-12T20:00:00.000Z')
    expect(
      screen.getByText(
        `Excluir a sessão de «${start}» («1 h 00 min»)? O tempo dela sai do tempo total do jogo. Esta ação não pode ser desfeita.`,
      ),
    ).toBeInTheDocument()
  })

  it('SEL-012/para a descartada o texto não fala do tempo total', () => {
    renderDialog({ session: item({ state: 'discarded', endReason: null, activeSeconds: 600 }) })
    const start = formatLocalDateTime('2026-11-12T20:00:00.000Z')
    expect(
      screen.getByText(`Excluir a sessão descartada de «${start}»? Esta ação não pode ser desfeita.`),
    ).toBeInTheDocument()
  })

  it('SEL-012/botões "Excluir" e "Cancelar" e o foco inicial em "Cancelar"', () => {
    renderDialog()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Excluir' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancelar' })).toHaveFocus()
  })

  it('SEL-012/Cancelar e Esc chamam onCancel e nunca onConfirm', () => {
    const p = renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(p.onCancel).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(p.onCancel).toHaveBeenCalledTimes(2)
    expect(p.onConfirm).not.toHaveBeenCalled()
  })

  it('SEL-012/Excluir chama onConfirm uma vez por clique', () => {
    const p = renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Excluir' }))
    expect(p.onConfirm).toHaveBeenCalledTimes(1)
    expect(p.onCancel).not.toHaveBeenCalled()
  })

  it('SEL-011/durante a exclusão o botão fica desativado com "Excluindo…" e o clique não chama de novo', () => {
    const p = renderDialog({ pending: true })
    const btn = screen.getByRole('button', { name: 'Excluindo…' })
    expect(btn).toBeDisabled()
    fireEvent.click(btn)
    expect(p.onConfirm).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: 'Excluir' })).toBeNull()
  })

  it('SEL-010/erro do agente aparece em role="alert" e o diálogo continua aberto', () => {
    renderDialog({ error: 'Não é possível alterar uma sessão em andamento.' })
    expect(screen.getByRole('alert')).toHaveTextContent('Não é possível alterar uma sessão em andamento.')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('SEL-010/sem erro não há role="alert"', () => {
    renderDialog()
    expect(screen.queryByRole('alert')).toBeNull()
  })
})
