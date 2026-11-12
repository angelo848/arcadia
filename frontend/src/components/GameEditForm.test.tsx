import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { GameEditForm } from './GameEditForm'
import type { GameDetailView } from '../lib/library'

type Props = Parameters<typeof GameEditForm>[0]

function game(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: 10,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'PC',
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
    emulatorId: null,
    ...over,
  }
}

function renderForm(over: Partial<Props> = {}) {
  const props: Props = {
    game: game(),
    onSave: vi.fn(),
    onCancel: vi.fn(),
    pending: false,
    error: null,
    ...over,
  }
  const utils = render(<GameEditForm {...props} />)
  return { ...utils, props }
}

function field(label: string): HTMLInputElement {
  return screen.getByLabelText(label) as HTMLInputElement
}

function saveButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Salvar' })
}

describe('GameEditForm', () => {
  it('UI-039/formulário mostra os valores atuais do jogo', () => {
    renderForm({ game: game({ favorite: true, completedAt: null }) })

    expect(field('Título')).toHaveValue('Hollow Knight')
    expect(field('Descrição')).toHaveValue('Um metroidvania difícil.')
    expect(field('Ano')).toHaveValue('2017')
    expect(field('Concluído em')).toHaveValue('')
    expect(field('Status')).toHaveValue('playing')
    expect(field('Favorito')).toBeChecked()
  })

  it('EDT-004/modos de jogo são editáveis: marcar e desmarcar altera playModes no patch', () => {
    const { props } = renderForm()

    expect(screen.getByRole('checkbox', { name: 'Um jogador' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Cooperativo online' })).not.toBeChecked()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Cooperativo online' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Um jogador' }))
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledWith({ playModes: ['coop_online'] })
  })

  it('UI-039/Salvar envia só o patch e fica desabilitado durante o envio', () => {
    const { props, rerender } = renderForm()

    fireEvent.change(field('Título'), { target: { value: 'Hollow Knight GOTY' } })
    fireEvent.change(field('Ano'), { target: { value: '2018' } })
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledTimes(1)
    expect(props.onSave).toHaveBeenCalledWith({ title: 'Hollow Knight GOTY', year: 2018 })

    // While the request is pending, Salvar is disabled and a click sends nothing.
    rerender(<GameEditForm {...props} pending={true} />)
    expect(saveButton()).toBeDisabled()
    fireEvent.click(saveButton())
    expect(props.onSave).toHaveBeenCalledTimes(1)
  })

  it('UI-039/erro mostra a mensagem e mantém o formulário com os valores digitados', () => {
    const { props, rerender } = renderForm()

    fireEvent.change(field('Título'), { target: { value: 'Novo título' } })
    fireEvent.click(saveButton())
    expect(props.onSave).toHaveBeenCalledTimes(1)

    const message = 'Não foi possível salvar. Tente de novo.'
    rerender(<GameEditForm {...props} pending={false} error={message} />)

    expect(screen.getByText(message)).toBeInTheDocument()
    // The form stays open with what was typed, and Salvar is usable again.
    expect(field('Título')).toHaveValue('Novo título')
    expect(saveButton()).toBeEnabled()
    expect(props.onCancel).not.toHaveBeenCalled()
  })

  it('UI-039/Cancelar descarta as alterações sem chamar onSave', () => {
    const { props } = renderForm()

    fireEvent.change(field('Título'), { target: { value: 'Outro título' } })
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))

    expect(props.onCancel).toHaveBeenCalledTimes(1)
    expect(props.onSave).not.toHaveBeenCalled()
  })

  it('UI-040/com erro de validação, Salvar fica desabilitado e nada é enviado', () => {
    const cases: Array<[string, Record<string, string>]> = [
      ['título em branco', { Título: '   ' }],
      ['ano fora da faixa', { Ano: '1899' }],
      ['data fora de AAAA-MM-DD', { 'Concluído em': '12/11/2026' }],
      ['descrição acima de 10 000', { Descrição: 'a'.repeat(10001) }],
    ]

    for (const [name, values] of cases) {
      const { props, unmount } = renderForm()
      for (const [label, value] of Object.entries(values)) {
        fireEvent.change(field(label), { target: { value } })
      }
      expect(saveButton(), name).toBeDisabled()
      fireEvent.click(saveButton())
      expect(props.onSave, name).not.toHaveBeenCalled()
      unmount()
    }
  })

  it('TAG-005/salvar envia genres como array e só quando mudou', () => {
    const { props } = renderForm({ genreSuggestions: ['Metroidvania', 'Ação'] })

    fireEvent.click(screen.getByRole('button', { name: 'Remover Metroidvania' }))
    const input = screen.getByLabelText('Gêneros')
    fireEvent.change(input, { target: { value: 'ação' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledWith({ genres: ['Ação'] })
  })

  it('TAG-002/gênero novo entra no patch com a grafia digitada', () => {
    const { props } = renderForm({ genreSuggestions: ['Metroidvania'] })

    const input = screen.getByLabelText('Gêneros')
    fireEvent.change(input, { target: { value: 'Terror' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledWith({ genres: ['Metroidvania', 'Terror'] })
  })

  it('EDT-010/editar o executável e os argumentos envia só esses campos', () => {
    const { props } = renderForm()

    fireEvent.change(screen.getByLabelText('Executável (caminho completo)'), {
      target: { value: 'C:\\Jogos\\novo.exe' },
    })
    fireEvent.change(screen.getByLabelText('Argumentos'), { target: { value: '-windowed' } })
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledWith({ launchTarget: 'C:\\Jogos\\novo.exe', launchArgs: '-windowed' })
  })

  it('EDT-010/executável com caminho relativo desabilita Salvar', () => {
    renderForm()

    fireEvent.change(screen.getByLabelText('Executável (caminho completo)'), { target: { value: 'novo.exe' } })

    expect(saveButton()).toBeDisabled()
  })

  it('EMU-002/escolher um emulador para o jogo envia emulatorId no patch', () => {
    const emus = [{ id: 3, name: 'Emu', exePath: 'C:/Emu/e.exe', argsTemplate: '' }]
    const { props } = renderForm({ emulators: emus })

    fireEvent.change(screen.getByLabelText('Emulador deste jogo'), { target: { value: '3' } })
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledWith({ emulatorId: 3 })
  })

  it('EMU-002/voltar para o padrão da plataforma envia emulatorId nulo', () => {
    const emus = [{ id: 3, name: 'Emu', exePath: 'C:/Emu/e.exe', argsTemplate: '' }]
    const { props } = renderForm({ emulators: emus, game: game({ emulatorId: 3 }) })

    fireEvent.change(screen.getByLabelText('Emulador deste jogo'), { target: { value: '' } })
    fireEvent.click(saveButton())

    expect(props.onSave).toHaveBeenCalledWith({ emulatorId: null })
  })
})
