import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GameCard } from './GameCard'
import type { GameView } from '../lib/library'

const game = {
  id: 1, title: 'Celeste', platform: 'PC', platformId: 1, status: 'playing', favorite: false,
  totalPlaySeconds: 600, playCount: 1, firstPlayedAt: null, lastPlayedAt: null, path: 'C:\c.exe',
  genres: [], year: null,
} as unknown as GameView

const handlers = { onOpen: vi.fn(), onPlay: vi.fn(), onPlayUntracked: vi.fn() }

describe('GameCard in the Big Box (BIG-004)', () => {
  it('BIG-004/jogo pausado mostra Pausado no Big Box e a pausa não muda o botão Jogar', () => {
    render(<GameCard game={game} live="paused" busy={null} {...handlers} />)
    expect(screen.getByText('Pausado')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Jogar' })).toBeEnabled()
  })

  it('BIG-004/jogo rodando mostra Em execução', () => {
    render(<GameCard game={game} live="running" busy={null} {...handlers} />)
    expect(screen.getByText('Em execução')).toBeInTheDocument()
  })
})
