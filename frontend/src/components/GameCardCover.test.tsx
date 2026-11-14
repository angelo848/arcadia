import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { GameCard } from './GameCard'
import { coverPlaceholderUrl } from './CoverImage'
import type { GameView } from '../lib/library'

const game: GameView = {
  id: 1, title: 'Celeste', platform: 'PC', platformId: 1, status: 'not_played', favorite: false,
  totalPlaySeconds: 0, playCount: 0, firstPlayedAt: null, lastPlayedAt: null, path: 'C:\c.exe',
  genres: [], year: null,
} as unknown as GameView

const handlers = { onSelect: vi.fn(), onLaunch: vi.fn() }

describe('GameCard cover (MID-006)', () => {
  it('MID-006/com capa mostra a imagem da capa', () => {
    render(<GameCard game={game} live={null} selected={false} tabbable={false} coverUrl="/media/1/c.jpg" {...handlers} />)
    expect(document.querySelector('img')).toHaveAttribute('src', '/media/1/c.jpg')
  })

  it('MID-006/sem capa mostra o placeholder no lugar e o título continua visível', () => {
    render(<GameCard game={game} live={null} selected={false} tabbable={false} {...handlers} />)
    expect(document.querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)
    expect(screen.getByText('Celeste')).toBeInTheDocument()
  })

  it('UIX-015/imagem que falha ao carregar vira o placeholder', () => {
    render(<GameCard game={game} live={null} selected={false} tabbable={false} coverUrl="/media/1/quebrada.jpg" {...handlers} />)
    const img = document.querySelector('img') as HTMLImageElement
    expect(img).toHaveAttribute('src', '/media/1/quebrada.jpg')

    fireEvent.error(img)
    expect(document.querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)
  })
})
