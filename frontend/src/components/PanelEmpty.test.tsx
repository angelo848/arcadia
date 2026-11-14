import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PanelEmpty } from './PanelEmpty'
import { coverPlaceholderUrl } from './CoverImage'
import { formatLocalDateTime } from '../lib/formatDate'
import type { GameView } from '../lib/library'
import type { MediaItemView } from '../lib/metadata'

type Fn = ReturnType<typeof vi.fn>
let app: { ListGames: Fn; MediaList: Fn; LaunchGame: Fn; StartGame: Fn }

function game(id: number, over: Partial<GameView> = {}): GameView {
  return {
    id,
    platformId: 1,
    title: `Jogo ${id}`,
    platform: 'Windows',
    path: 'C:\\g.exe',
    status: 'playing',
    favorite: false,
    year: 2020,
    totalPlaySeconds: 60,
    playCount: 1,
    firstPlayedAt: '2026-01-01T10:00:00Z',
    lastPlayedAt: `2026-02-0${id}T10:30:00Z`,
    ...over,
  } as GameView
}

function install(games: GameView[], media: Record<number, MediaItemView[]> = {}) {
  app = {
    ListGames: vi.fn(() => Promise.resolve({ games, total: games.length, offset: 0 })),
    MediaList: vi.fn((id: number) => Promise.resolve({ media: media[id] ?? [] })),
    LaunchGame: vi.fn(() => Promise.resolve()),
    StartGame: vi.fn(() => Promise.resolve()),
  }
  ;(window as unknown as { go: unknown }).go = { main: { App: app } }
}

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>)
}

beforeEach(() => install([]))
afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('PanelEmpty', () => {
  it('UIX-024/mostra "Nenhum jogo selecionado" e o texto de instrução', () => {
    wrap(<PanelEmpty onSelect={() => {}} />)
    expect(screen.getByText('Nenhum jogo selecionado')).toBeTruthy()
    expect(screen.getByText(/selecion/i, { selector: 'p, div, span' })).toBeTruthy()
  })

  it('UIX-024/com jogos jogados lista até 5 em "Jogados recentemente" com capa ou placeholder, título e última vez', async () => {
    const cover: MediaItemView = {
      id: 1, kind: 'cover', provider: 'p', path: '', thumbPath: '', url: '/m/c1.jpg', thumbUrl: '/t/c1.jpg',
    }
    install([game(1), game(2), game(3), game(4), game(5)], { 1: [cover] })
    const { container } = wrap(<PanelEmpty onSelect={() => {}} />)
    expect(await screen.findByText('Jogados recentemente')).toBeTruthy()
    const items = await screen.findAllByRole('listitem')
    expect(items).toHaveLength(5)
    expect(within(items[0]).getByText('Jogo 1')).toBeTruthy()
    expect(within(items[0]).getByText(formatLocalDateTime('2026-02-01T10:30:00Z'))).toBeTruthy()
    await waitFor(() =>
      expect(within(items[0]).getByRole('img', { hidden: true }).getAttribute('src')).toBe('/t/c1.jpg'),
    )
    expect(within(items[1]).getByRole('img', { hidden: true }).getAttribute('src')).toBe(coverPlaceholderUrl)
    expect(container.querySelectorAll('li img')).toHaveLength(5)
  })

  it('UIX-024/ListGames chamado com played=played, sort lastPlayed e limit 5', async () => {
    install([game(1)])
    wrap(<PanelEmpty onSelect={() => {}} />)
    await screen.findByText('Jogo 1')
    expect(app.ListGames).toHaveBeenCalledWith({
      filter: { version: 1, played: 'played' },
      sort: 'lastPlayed',
      offset: 0,
      limit: 5,
    })
  })

  it('UIX-024/clicar num item chama onSelect(id) e nenhum binding de lançamento', async () => {
    install([game(1), game(2)])
    const onSelect = vi.fn()
    wrap(<PanelEmpty onSelect={onSelect} />)
    fireEvent.click(await screen.findByText('Jogo 2'))
    expect(onSelect).toHaveBeenCalledTimes(1)
    expect(onSelect).toHaveBeenCalledWith(2)
    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(app.StartGame).not.toHaveBeenCalled()
  })

  it('UIX-024/resposta vazia (biblioteca vazia) não mostra a lista', async () => {
    wrap(<PanelEmpty onSelect={() => {}} />)
    await waitFor(() => expect(app.ListGames).toHaveBeenCalled())
    expect(screen.getByText('Nenhum jogo selecionado')).toBeTruthy()
    expect(screen.queryByText('Jogados recentemente')).toBeNull()
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
  })
})
