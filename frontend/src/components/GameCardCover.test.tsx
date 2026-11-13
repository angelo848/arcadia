import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GameCard } from './GameCard'
import type { GameView } from '../lib/library'

const game: GameView = {
  id: 1, title: 'Celeste', platform: 'PC', platformId: 1, status: 'not_played', favorite: false,
  totalPlaySeconds: 0, playCount: 0, firstPlayedAt: null, lastPlayedAt: null, path: 'C:\c.exe',
  genres: [], year: null,
} as unknown as GameView

const handlers = { onOpen: vi.fn(), onPlay: vi.fn(), onPlayUntracked: vi.fn() }

describe('GameCard cover (MID-006)', () => {
  it('MID-006/com capa mostra a imagem da capa', () => {
    render(<GameCard game={game} live={null} busy={null} coverUrl="/media/1/c.jpg" {...handlers} />)
    expect(document.querySelector('img')).toHaveAttribute('src', '/media/1/c.jpg')
  })

  it('MID-006/sem capa não mostra imagem', () => {
    render(<GameCard game={game} live={null} busy={null} {...handlers} />)
    expect(document.querySelector('img')).toBeNull()
    expect(screen.getByText('Celeste')).toBeInTheDocument()
  })
})
