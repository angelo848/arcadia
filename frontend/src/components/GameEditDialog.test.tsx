import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { GameEditDialog } from './GameEditDialog'
import type { GameDetailView } from '../lib/library'

type Props = Parameters<typeof GameEditDialog>[0]

function game(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: 10,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'Windows',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    status: 'playing',
    favorite: false,
    year: 2017,
    totalPlaySeconds: 3900,
    playCount: 4,
    firstPlayedAt: '',
    lastPlayedAt: '',
    description: 'Um metroidvania difícil.',
    completedAt: null,
    developers: ['Team Cherry'],
    publishers: ['Team17'],
    genres: ['Metroidvania'],
    playModes: ['single'],
    launchType: 'exe',
    launchArgs: '',
    sessionPlaySeconds: 0, // alterado em 2026-11-25 (fase 16)
    emulatorId: null,
    ...over,
  }
}

function renderDialog(over: Partial<Props> = {}) {
  const props: Props = {
    game: game(),
    loading: false,
    loadError: null,
    genreSuggestions: ['Metroidvania', 'Ação'],
    emulators: [],
    pending: false,
    error: null,
    onSave: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  }
  const utils = render(<GameEditDialog {...props} />)
  return { ...utils, props }
}

function field(label: string): HTMLInputElement {
  return screen.getByLabelText(label) as HTMLInputElement
}

function saveButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Salvar' })
}

function pressEscape() {
  fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape', code: 'Escape' })
}

describe('GameEditDialog', () => {
  it('UIX-032/abre como modal medium com o título "Editar jogo" e os campos de UI-039', () => {
    renderDialog()

    const dialog = screen.getByRole('dialog', { name: 'Editar jogo' })
    expect(dialog).toHaveAttribute('data-size', 'medium')
    for (const label of [
      'Título',
      'Descrição',
      'Ano',
      'Concluído em',
      'Status',
      'Favorito',
      'Gêneros',
      'Desenvolvedora',
      'Editora',
      'Argumentos',
    ]) {
      expect(within(dialog).getByLabelText(label), label).toBeInTheDocument()
    }
    expect(within(dialog).getByRole('checkbox', { name: 'Um jogador' })).toBeChecked()
    expect(field('Título')).toHaveValue('Hollow Knight')
  })

  it('UIX-032/Salvar e Cancelar ficam no rodapé do modal', () => {
    renderDialog()

    const dialog = screen.getByRole('dialog', { name: 'Editar jogo' })
    const save = within(dialog).getByRole('button', { name: 'Salvar' })
    const cancel = within(dialog).getByRole('button', { name: 'Cancelar' })
    // The footer is a fixed (shrink-0) region that holds the actions and none of the fields.
    const footer = save.closest('.shrink-0')
    expect(footer).not.toBeNull()
    expect(footer).toContainElement(cancel)
    expect(footer).not.toContainElement(field('Título'))
    // And it comes after the body in document order.
    expect(field('Título').compareDocumentPosition(save) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0)
  })

  it('UIX-032/Salvar chama onSave só com o patch alterado', () => {
    const { props } = renderDialog()

    fireEvent.change(field('Título'), { target: { value: 'Hollow Knight GOTY' } })
    fireEvent.change(field('Ano'), { target: { value: '2018' } })
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledTimes(1)
    expect(props.onSave).toHaveBeenCalledWith({ title: 'Hollow Knight GOTY', year: 2018 })
    expect(props.onCancel).not.toHaveBeenCalled()
  })

  it('UIX-032/com error, a mensagem aparece e os valores digitados ficam', () => {
    const { props, rerender } = renderDialog()

    fireEvent.change(field('Título'), { target: { value: 'Novo título' } })
    const message = 'Não foi possível salvar. Tente de novo.'
    rerender(<GameEditDialog {...props} error={message} />)

    expect(screen.getByText(message)).toBeInTheDocument()
    expect(field('Título')).toHaveValue('Novo título')
    expect(saveButton()).toBeEnabled()
    expect(props.onCancel).not.toHaveBeenCalled()
  })

  it('UIX-032/Cancelar e Esc chamam onCancel sem onSave', () => {
    const { props } = renderDialog()

    fireEvent.change(field('Título'), { target: { value: 'Outro título' } })
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(props.onCancel).toHaveBeenCalledTimes(1)

    pressEscape()
    expect(props.onCancel).toHaveBeenCalledTimes(2)
    expect(props.onSave).not.toHaveBeenCalled()
  })

  it('UIX-032/com pending, Esc não chama onCancel e Cancelar fica desativado', () => {
    const { props } = renderDialog({ pending: true })

    pressEscape()
    expect(props.onCancel).not.toHaveBeenCalled()
    const cancel = screen.getByRole('button', { name: 'Cancelar' })
    expect(cancel).toBeDisabled()
    fireEvent.click(cancel)
    expect(props.onCancel).not.toHaveBeenCalled()
  })

  it('UIX-032/com loading mostra "Carregando jogo…" e nenhum campo', () => {
    renderDialog({ game: undefined, loading: true })

    const dialog = screen.getByRole('dialog', { name: 'Editar jogo' })
    expect(within(dialog).getByText('Carregando jogo…')).toBeInTheDocument()
    expect(within(dialog).queryByLabelText('Título')).toBeNull()
    expect(within(dialog).queryByRole('textbox')).toBeNull()
  })

  it('UIX-032/com loadError mostra role=alert e nenhum campo', () => {
    renderDialog({ game: undefined, loadError: 'Falha ao ler o jogo.' })

    const dialog = screen.getByRole('dialog', { name: 'Editar jogo' })
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Falha ao ler o jogo.')
    expect(within(dialog).queryByLabelText('Título')).toBeNull()
    expect(within(dialog).queryByRole('textbox')).toBeNull()
  })

  it('UIX-032/título vazio desativa Salvar (UI-040)', () => {
    const { props } = renderDialog()

    fireEvent.change(field('Título'), { target: { value: '   ' } })
    expect(saveButton()).toBeDisabled()
    fireEvent.click(saveButton())
    expect(props.onSave).not.toHaveBeenCalled()
  })
})
