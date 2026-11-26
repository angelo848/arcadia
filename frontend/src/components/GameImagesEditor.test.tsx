import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { GameEditForm } from './GameEditForm'
import { GameImagesEditor } from './GameImagesEditor'
import { playTimeParts, playTimeSeconds } from '../lib/manualEdit'
import type { GameDetailView } from '../lib/library'

type Item = { id: number; kind: string; provider: string; path: string; thumbPath: string; url: string; thumbUrl: string }

function item(id: number, kind: string): Item {
  return { id, kind, provider: 'user', path: '', thumbPath: '', url: `/media/1/${id}.png`, thumbUrl: `/media/1/t${id}.jpg` }
}

function stubApp(media: Item[]) {
  const app = {
    MediaList: vi.fn(() => Promise.resolve({ media })),
    PickGameImage: vi.fn(() => Promise.resolve('D:\\Imagens\\nova.png')),
    MediaAdd: vi.fn(() => Promise.resolve()),
    MediaRemove: vi.fn(() => Promise.resolve()),
  }
  ;(window as unknown as { go: unknown }).go = { main: { App: app } }
  return app
}

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('GameImagesEditor', () => {
  it('EDM-004/mostra capa, screenshots e fundo com as imagens do jogo e um Remover por imagem', async () => {
    stubApp([item(1, 'cover'), item(2, 'screenshot'), item(3, 'screenshot'), item(4, 'video')])
    render(<GameImagesEditor gameId={1} />)

    const shots = screen.getByRole('group', { name: 'Screenshots de gameplay' })
    await waitFor(() => expect(within(shots).getAllByRole('button', { name: /Remover/ })).toHaveLength(2))
    const cover = screen.getByRole('group', { name: 'Capa' })
    expect(within(cover).getByRole('button', { name: 'Trocar...' })).toBeInTheDocument()
    expect(within(screen.getByRole('group', { name: 'Fundo' })).getByRole('button', { name: 'Adicionar...' })).toBeEnabled()
  })

  it('EDM-001/Adicionar abre o seletor e envia o arquivo com o tipo; cancelar o seletor não envia', async () => {
    const app = stubApp([])
    render(<GameImagesEditor gameId={7} />)
    const add = within(screen.getByRole('group', { name: 'Fundo' })).getByRole('button', { name: 'Adicionar...' })

    fireEvent.click(add)
    await waitFor(() => expect(app.MediaAdd).toHaveBeenCalledWith(7, 'background', 'D:\\Imagens\\nova.png'))
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledTimes(2))

    app.PickGameImage.mockResolvedValueOnce('')
    await waitFor(() => expect(add).toBeEnabled())
    fireEvent.click(add)
    await waitFor(() => expect(app.PickGameImage).toHaveBeenCalledTimes(2))
    expect(app.MediaAdd).toHaveBeenCalledTimes(1)
  })

  it('EDM-003/Remover envia o id da imagem; erro do agente aparece como alerta', async () => {
    const app = stubApp([item(5, 'screenshot')])
    render(<GameImagesEditor gameId={1} />)
    const remove = await screen.findByRole('button', { name: 'Remover Screenshots de gameplay 1' })

    fireEvent.click(remove)
    await waitFor(() => expect(app.MediaRemove).toHaveBeenCalledWith(5))

    app.MediaRemove.mockRejectedValueOnce('O agente não respondeu.')
    await waitFor(() => expect(remove).toBeEnabled())
    fireEvent.click(remove)
    expect(await screen.findByRole('alert')).toHaveTextContent('O agente não respondeu.')
  })

  it('EDM-002/tipo no limite desativa Adicionar', async () => {
    stubApp([item(1, 'background'), item(2, 'background')])
    render(<GameImagesEditor gameId={1} />)
    const group = screen.getByRole('group', { name: 'Fundo' })
    await waitFor(() => expect(within(group).getByRole('button', { name: 'Adicionar...' })).toBeDisabled())
  })
})

describe('tempo jogado', () => {
  const game: GameDetailView = {
    id: 10,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'PC',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    status: 'playing',
    favorite: false,
    year: 2017,
    totalPlaySeconds: 3930,
    playCount: 4,
    firstPlayedAt: '',
    lastPlayedAt: '',
    description: '',
    completedAt: null,
    developers: [],
    publishers: [],
    genres: [],
    playModes: ['single'],
    launchType: 'exe',
    launchArgs: '',
    sessionPlaySeconds: 0, // alterado em 2026-11-25 (fase 16)
    emulatorId: null,
  }

  it('EDM-005/horas e minutos viram segundos; valores inválidos dão null', () => {
    expect(playTimeParts(3930)).toEqual({ hours: '1', minutes: '5' })
    expect(playTimeSeconds('2', '30')).toBe(9000)
    expect(playTimeSeconds('', '')).toBe(0)
    expect(playTimeSeconds('1', '60')).toBeNull()
    expect(playTimeSeconds('abc', '0')).toBeNull()
    expect(playTimeSeconds('-1', '0')).toBeNull()
  })

  it('EDM-005/o formulário mostra o total, envia totalPlaySeconds só quando muda e bloqueia valor inválido', () => {
    const onSave = vi.fn()
    render(<GameEditForm game={game} onSave={onSave} onCancel={vi.fn()} pending={false} error={null} />)
    const hours = screen.getByLabelText('Tempo jogado (horas)')
    const minutes = screen.getByLabelText('Minutos')
    const save = screen.getByRole('button', { name: 'Salvar' })
    expect(hours).toHaveValue('1')
    expect(minutes).toHaveValue('5')

    fireEvent.click(save)
    expect(onSave).toHaveBeenLastCalledWith({})

    fireEvent.change(minutes, { target: { value: '75' } })
    expect(save).toBeDisabled()

    fireEvent.change(hours, { target: { value: '12' } })
    fireEvent.change(minutes, { target: { value: '30' } })
    fireEvent.click(save)
    expect(onSave).toHaveBeenLastCalledWith({ totalPlaySeconds: 45000 })
  })
})
