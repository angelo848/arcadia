import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GameCard } from './GameCard'
import type { GameView } from '../lib/library'

const game = {
  id: 1, title: 'Celeste', platform: 'PC', platformId: 1, status: 'playing', favorite: false,
  totalPlaySeconds: 600, playCount: 1, firstPlayedAt: null, lastPlayedAt: null, path: 'C:\c.exe',
  genres: [], year: null,
} as unknown as GameView

const handlers = { onSelect: vi.fn(), onLaunch: vi.fn() }

describe('GameCard in the Big Box (BIG-004)', () => {
  it('BIG-004/jogo pausado mostra Pausado no Big Box', () => {
    render(<GameCard game={game} live="paused" selected={false} tabbable={false} {...handlers} />)
    expect(screen.getByText('Pausado')).toBeInTheDocument()
    // The card has no launch buttons any more (UIX-014); launching is a double click or the panel.
    expect(screen.queryByRole('button', { name: 'Jogar' })).toBeNull()
  })

  it('BIG-004/jogo rodando mostra Em execução', () => {
    render(<GameCard game={game} live="running" selected={false} tabbable={false} {...handlers} />)
    expect(screen.getByText('Em execução')).toBeInTheDocument()
  })
})
