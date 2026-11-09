import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { AddGameForm } from './AddGameForm'
import type { PickedExe } from '../lib/games'

const picked: PickedExe = { path: 'C:\\Games\\HK\\hollow_knight.exe', title: 'hollow_knight' }

function renderForm(over: { pending?: boolean } = {}) {
  const onSave = vi.fn()
  const onCancel = vi.fn()
  render(
    <AddGameForm picked={picked} pending={over.pending ?? false} onSave={onSave} onCancel={onCancel} />,
  )
  return { onSave, onCancel }
}

function titleInput(): HTMLInputElement {
  return screen.getByLabelText('Título') as HTMLInputElement
}

describe('AddGameForm', () => {
  it('UI-007 caminho e Título preenchidos com maxLength 200', () => {
    const { onSave } = renderForm()

    // The path is read-only text; it may be plain text or a read-only input.
    const pathEl = screen.queryByText(picked.path) ?? screen.queryByDisplayValue(picked.path)
    expect(pathEl).not.toBeNull()
    if (pathEl instanceof HTMLInputElement) {
      expect(pathEl.readOnly).toBe(true)
    }

    const input = titleInput()
    expect(input).toHaveValue('hollow_knight')
    expect(input).toHaveAttribute('maxlength', '200')

    // Editable: the edited value is the one saved.
    fireEvent.change(input, { target: { value: 'Hollow Knight' } })
    expect(input).toHaveValue('Hollow Knight')

    const save = screen.getByRole('button', { name: 'Salvar' })
    expect(save).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeEnabled()
    fireEvent.click(save)
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith('Hollow Knight')
  })

  it('UI-007 Salvar desabilitado com título só de espaços', () => {
    const { onSave } = renderForm()
    const save = screen.getByRole('button', { name: 'Salvar' })

    for (const value of ['', '   ', '\t \n']) {
      fireEvent.change(titleInput(), { target: { value } })
      expect(save, JSON.stringify(value)).toBeDisabled()
    }
    fireEvent.click(save)
    expect(onSave).not.toHaveBeenCalled()

    fireEvent.change(titleInput(), { target: { value: '  X  ' } })
    expect(save).toBeEnabled()
  })

  it('UI-007 Salvar desabilitado durante o envio', () => {
    const { onSave } = renderForm({ pending: true })
    const save = screen.getByRole('button', { name: 'Salvar' })
    expect(save).toBeDisabled()
    fireEvent.click(save)
    expect(onSave).not.toHaveBeenCalled()
  })

  it('UI-007 Cancelar chama onCancel', () => {
    const { onSave, onCancel } = renderForm()
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onSave).not.toHaveBeenCalled()
  })
})
