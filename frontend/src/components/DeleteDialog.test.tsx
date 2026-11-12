import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { DeleteDialog } from './DeleteDialog'

type Props = Parameters<typeof DeleteDialog>[0]

// DSK-018: message for DeleteGame with -32002 (session active). Without the title.
const sessionActiveMessage =
  'Não é possível remover: o jogo está em uma sessão ativa. Feche a sessão antes de remover.'

function renderDialog(over: Partial<Props> = {}) {
  const props: Props = {
    title: 'Hollow Knight',
    pending: false,
    error: null,
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  }
  const utils = render(<DeleteDialog {...props} />)
  return { ...utils, props }
}

function removeButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Remover' })
}

function cancelButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Cancelar' })
}

describe('DeleteDialog', () => {
  it('UI-041/mostra o texto exato com o título entre « »', () => {
    renderDialog({ title: 'Hollow Knight' })

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Remover «Hollow Knight»? As sessões e as pausas deste jogo também serão apagadas. Esta ação não pode ser desfeita.',
      ),
    ).toBeInTheDocument()
    expect(removeButton()).toBeEnabled()
    expect(cancelButton()).toBeEnabled()
  })

  it('UI-041/Cancelar, Esc e fechar não chamam onConfirm', () => {
    const { props } = renderDialog()

    // "Cancelar" button.
    fireEvent.click(cancelButton())
    expect(props.onCancel).toHaveBeenCalledTimes(1)

    // Escape key closes the dialog through onCancel.
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(props.onCancel).toHaveBeenCalledTimes(2)

    expect(props.onConfirm).not.toHaveBeenCalled()
  })

  it('UI-041/Remover chama onConfirm uma vez; durante o envio o botão fica desabilitado', () => {
    const { props, rerender } = renderDialog()

    fireEvent.click(removeButton())
    expect(props.onConfirm).toHaveBeenCalledTimes(1)

    // While DeleteGame is pending, Remover is disabled and a click sends nothing.
    rerender(<DeleteDialog {...props} pending={true} />)
    expect(removeButton()).toBeDisabled()
    fireEvent.click(removeButton())
    expect(props.onConfirm).toHaveBeenCalledTimes(1)
    expect(props.onCancel).not.toHaveBeenCalled()
  })

  it('UI-041/erro (-32002, mensagem de DSK-018) aparece no diálogo com aviso, e o diálogo fecha pelo pai', () => {
    const { props, rerender } = renderDialog()

    fireEvent.click(removeButton())
    expect(props.onConfirm).toHaveBeenCalledTimes(1)

    // The parent passes the DSK-018 message back after the -32002 error.
    rerender(<DeleteDialog {...props} pending={false} error={sessionActiveMessage} />)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(sessionActiveMessage)
    // The dialog does not close itself: it stays on screen until the parent removes it.
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(removeButton()).toBeEnabled()
    expect(props.onConfirm).toHaveBeenCalledTimes(1)

    rerender(<></>)
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
