import { useRef, useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { Modal, type ModalSize } from './Modal'

function directChild(dialog: HTMLElement, inner: HTMLElement): HTMLElement {
  let el: HTMLElement = inner
  while (el.parentElement !== null && el.parentElement !== dialog) {
    el = el.parentElement
  }
  return el
}

function renderModal(props: Partial<React.ComponentProps<typeof Modal>> = {}) {
  const onClose = props.onClose ?? vi.fn()
  const utils = render(
    <Modal size="medium" title="Meu modal" onClose={onClose} footer={<button type="button">Salvar</button>} {...props}>
      <button type="button">Primeiro</button>
      <button type="button">Segundo</button>
      <p data-testid="body-text">corpo</p>
    </Modal>,
  )
  return { ...utils, onClose }
}

describe('Modal', () => {
  it.each<ModalSize>(['large', 'medium', 'compact'])(
    'UIX-037/data-size e a classe modal-<size> para large, medium e compact (%s)',
    (size) => {
      renderModal({ size })
      const dialog = screen.getByRole('dialog')
      expect(dialog).toHaveAttribute('data-size', size)
      expect(dialog).toHaveClass(`modal-${size}`)
    },
  )

  it('UIX-038/cabeçalho e rodapé com shrink-0 e corpo com overflow-y auto e flex-1', () => {
    renderModal()
    const dialog = screen.getByRole('dialog')
    const header = directChild(dialog, screen.getByRole('heading', { name: 'Meu modal' }))
    const footer = directChild(dialog, screen.getByRole('button', { name: 'Salvar' }))
    const body = directChild(dialog, screen.getByTestId('body-text'))
    expect(header).toHaveClass('shrink-0')
    expect(footer).toHaveClass('shrink-0')
    expect(body).toHaveClass('overflow-y-auto')
    expect(body).toHaveClass('flex-1')
    expect(header).not.toBe(body)
    expect(footer).not.toBe(body)
  })

  it('UIX-039/role dialog, aria-modal true e nome acessível pelo título', () => {
    renderModal()
    const dialog = screen.getByRole('dialog', { name: 'Meu modal' })
    expect(dialog).toHaveAttribute('aria-modal', 'true')
  })

  it('UIX-039/ao abrir, o foco vai ao primeiro controle; com initialFocus, ao elemento indicado', () => {
    const { unmount } = renderModal()
    expect(screen.getByRole('button', { name: 'Primeiro' })).toHaveFocus()
    unmount()

    function WithInitial() {
      const ref = useRef<HTMLButtonElement>(null)
      return (
        <Modal size="compact" title="Confirmar" onClose={vi.fn()} initialFocus={ref}>
          <button type="button">Remover</button>
          <button type="button" ref={ref}>
            Cancelar
          </button>
        </Modal>
      )
    }
    render(<WithInitial />)
    expect(screen.getByRole('button', { name: 'Cancelar' })).toHaveFocus()
  })

  it('UIX-039/Tab no último volta ao primeiro e Shift+Tab no primeiro vai ao último', () => {
    renderModal()
    const dialog = screen.getByRole('dialog')
    const first = screen.getByRole('button', { name: 'Primeiro' })
    const last = screen.getByRole('button', { name: 'Salvar' })

    last.focus()
    fireEvent.keyDown(last, { key: 'Tab' })
    expect(first).toHaveFocus()

    fireEvent.keyDown(first, { key: 'Tab', shiftKey: true })
    expect(last).toHaveFocus()
    expect(dialog).toContainElement(document.activeElement as HTMLElement)
  })

  it('UIX-039/Esc chama onClose uma vez; com closeDisabled não chama', () => {
    const { onClose, unmount } = renderModal()
    fireEvent.keyDown(screen.getByRole('button', { name: 'Primeiro' }), { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
    unmount()

    const blocked = vi.fn()
    renderModal({ onClose: blocked, closeDisabled: true })
    fireEvent.keyDown(screen.getByRole('button', { name: 'Primeiro' }), { key: 'Escape' })
    expect(blocked).not.toHaveBeenCalled()
  })

  it('UIX-039/clicar no overlay não chama onClose', () => {
    const { onClose } = renderModal()
    const overlay = screen.getByRole('dialog').parentElement as HTMLElement
    fireEvent.mouseDown(overlay)
    fireEvent.click(overlay)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('UIX-039/ao desmontar, o foco volta ao elemento que abriu; se ele saiu do documento, vai ao listbox "Jogos"', () => {
    function Host() {
      const [open, setOpen] = useState(false)
      return (
        <div>
          <div role="listbox" aria-label="Jogos" tabIndex={0} />
          <button type="button" onClick={() => setOpen(true)}>
            Abrir
          </button>
          {open && (
            <Modal size="compact" title="Aberto" onClose={() => setOpen(false)}>
              <button type="button">Dentro</button>
            </Modal>
          )}
        </div>
      )
    }
    const { unmount } = render(<Host />)
    const opener = screen.getByRole('button', { name: 'Abrir' })
    opener.focus()
    fireEvent.click(opener)
    expect(screen.getByRole('button', { name: 'Dentro' })).toHaveFocus()
    fireEvent.keyDown(screen.getByRole('button', { name: 'Dentro' }), { key: 'Escape' })
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(opener).toHaveFocus()
    unmount()

    // The opener leaves the document while the modal is open.
    function Host2() {
      const [state, setState] = useState<'idle' | 'open' | 'closed'>('idle')
      return (
        <div>
          <div role="listbox" aria-label="Jogos" tabIndex={0} />
          {state === 'idle' && (
            <button type="button" onClick={() => setState('open')}>
              Abrir
            </button>
          )}
          {state === 'open' && (
            <Modal size="compact" title="Aberto" onClose={() => setState('closed')}>
              <button type="button">Dentro</button>
            </Modal>
          )}
        </div>
      )
    }
    render(<Host2 />)
    const opener2 = screen.getByRole('button', { name: 'Abrir' })
    opener2.focus()
    fireEvent.click(opener2)
    expect(opener2).not.toBeInTheDocument()
    act(() => {
      fireEvent.keyDown(screen.getByRole('button', { name: 'Dentro' }), { key: 'Escape' })
    })
    expect(screen.getByRole('listbox', { name: 'Jogos' })).toHaveFocus()
  })
})
