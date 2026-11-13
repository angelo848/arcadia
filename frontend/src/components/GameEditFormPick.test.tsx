import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { GameEditForm } from './GameEditForm'
import type { GameDetailView } from '../lib/library'

const pickMock = vi.fn()

vi.mock('../lib/games', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/games')>()
  return { ...actual, pickGameExecutable: () => pickMock() }
})

function game(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: 10, platformId: 1, title: 'Celeste', platform: 'PC', path: 'C:\Games\Celeste\Celeste.exe',
    status: 'not_played', favorite: false, year: 2018, totalPlaySeconds: 0, playCount: 0,
    firstPlayedAt: '', lastPlayedAt: '', description: '', completedAt: null, developers: [], publishers: [],
    genres: [], playModes: ['single'], launchType: 'exe', launchArgs: '', emulatorId: null, ...over,
  }
}

function renderForm(g: GameDetailView) {
  return render(
    <GameEditForm game={g} onSave={vi.fn()} onCancel={vi.fn()} pending={false} error={null} />,
  )
}

describe('GameEditForm executable picker (EDT-010)', () => {
  beforeEach(() => pickMock.mockReset())

  it('EDT-010/tipo executável mostra Procurar e o caminho escolhido vai para o campo', async () => {
    pickMock.mockResolvedValue('D:\Jogos\Celeste\Celeste.exe')
    renderForm(game())
    fireEvent.click(screen.getByRole('button', { name: 'Procurar...' }))

    await waitFor(() =>
      expect(screen.getByLabelText('Executável (caminho completo)')).toHaveValue('D:\Jogos\Celeste\Celeste.exe'),
    )
  })

  it('EDT-010/cancelar o seletor não muda o caminho', async () => {
    pickMock.mockResolvedValue('')
    renderForm(game())
    fireEvent.click(screen.getByRole('button', { name: 'Procurar...' }))

    await waitFor(() => expect(pickMock).toHaveBeenCalledTimes(1))
    expect(screen.getByLabelText('Executável (caminho completo)')).toHaveValue('C:\Games\Celeste\Celeste.exe')
  })

  it('EDT-010/tipo URI não oferece seletor de arquivos', () => {
    renderForm(game({ launchType: 'uri', path: 'steam://rungameid/504230' }))
    expect(screen.queryByRole('button', { name: 'Procurar...' })).toBeNull()
  })
})

describe('GameEditForm emulator target (ROM)', () => {
  beforeEach(() => pickMock.mockReset())

  it('EDT-010/tipo emulador mostra o seletor de ROM com qualquer arquivo', async () => {
    pickMock.mockResolvedValue('D:\Roms\SNES\Zelda.sfc')
    renderForm(game({ launchType: 'emulator', path: 'D:\Roms\SNES\old.sfc' }))
    expect(screen.getByLabelText('Arquivo do jogo (ROM, caminho completo)')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Procurar...' }))

    await waitFor(() =>
      expect(screen.getByLabelText('Arquivo do jogo (ROM, caminho completo)')).toHaveValue('D:\Roms\SNES\Zelda.sfc'),
    )
  })
})
