import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GameCard } from './GameCard'
import type { GameView } from '../lib/library'

const game = {
  id: 7, title: 'Celeste', platform: 'PC', platformId: 1, status: 'playing', favorite: false,
  totalPlaySeconds: 600, playCount: 1, firstPlayedAt: null, lastPlayedAt: null, path: 'C:\c.exe',
  genres: [], year: null,
} as unknown as GameView

const handlers = { onSelect: vi.fn(), onLaunch: vi.fn() }

describe('GameCard no Big Box', () => {
  it('UIX-044/o cartão é um item do Big Box e um cartão da grade, com o id do jogo', () => {
    render(<GameCard game={game} live={null} selected={false} tabbable={false} {...handlers} />)
    const card = screen.getByRole('option')
    expect(card).toHaveAttribute('data-bigbox-item')
    expect(card).toHaveAttribute('data-bigbox-card')
    expect(card).toHaveAttribute('data-game-id', '7')
    expect(document.querySelectorAll('[data-bigbox-item]')).toHaveLength(1)
  })

  it('UIX-044/o cartão não tem botões nem item "Jogar" próprio', () => {
    render(<GameCard game={game} live={null} selected={false} tabbable={false} {...handlers} />)
    expect(screen.queryByRole('button')).toBeNull()
    expect(document.querySelector('[data-bigbox-play]')).toBeNull()
  })

  it('UIX-044/jogo pausado mostra Pausado e jogo rodando mostra Em execução', () => {
    const { rerender } = render(<GameCard game={game} live="paused" selected={false} tabbable={false} {...handlers} />)
    expect(screen.getByText('Pausado')).toBeInTheDocument()
    rerender(<GameCard game={game} live="running" selected={false} tabbable={false} {...handlers} />)
    expect(screen.getByText('Em execução')).toBeInTheDocument()
  })
})
