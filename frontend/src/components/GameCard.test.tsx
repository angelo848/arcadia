import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { GameCard, liveOf } from './GameCard'
import { coverPlaceholderUrl } from './CoverImage'
import type { SessionView } from '../lib/games'
import type { GameView } from '../lib/library'

type CardProps = Parameters<typeof GameCard>[0]

function game(over: Partial<GameView> = {}): GameView {
  return {
    id: 7,
    platformId: 1,
    title: 'Celeste',
    platform: 'Windows',
    path: 'C:\\Games\\Celeste\\celeste.exe',
    status: 'not_played',
    favorite: false,
    year: 2018,
    totalPlaySeconds: 3900,
    playCount: 2,
    firstPlayedAt: '2026-11-01T10:00:00.000Z',
    lastPlayedAt: '2026-11-09T10:00:00.000Z',
    ...over,
  }
}

function props(over: Partial<CardProps> = {}): CardProps {
  return {
    game: game(),
    live: null,
    selected: false,
    tabbable: false,
    onSelect: vi.fn(),
    onLaunch: vi.fn(),
    ...over,
  }
}

function session(over: Partial<SessionView> = {}): SessionView {
  return {
    sessionId: 1,
    gameId: 7,
    title: 'Celeste',
    state: 'running',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '',
    activeSeconds: 0,
    activeMs: 0,
    asOf: '2026-11-09T10:00:00.000Z',
    ...over,
  }
}

describe('GameCard', () => {
  it('UIX-014/cartão com capa 2:3 e título (title com o texto completo); sem plataforma, tempo nem botões', () => {
    const long = 'Uma aventura com um título muito comprido que não cabe em duas linhas do cartão da grade'
    const { container } = render(<GameCard {...props({ game: game({ title: long }) })} />)

    expect(screen.getByText(long)).toBeInTheDocument()
    // The full title is in a title attribute, whichever element carries it.
    expect(container.querySelector(`[title="${long}"]`)).not.toBeNull()
    // The cover area keeps the 2:3 ratio.
    expect(container.querySelector('[class*="aspect-[3/4]"]')).not.toBeNull()
    expect(container.querySelector('img')).not.toBeNull()

    expect(screen.queryByText('Windows')).toBeNull()
    expect(screen.queryByText('1 h 05 min')).toBeNull()
    expect(screen.queryByText(/min|Nunca jogado/)).toBeNull()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('UIX-014/selo "Em execução" em running e grace, "Pausado" em paused, nenhum sem sessão do jogo', () => {
    // The grid passes live = liveOf(current, game.id): running and grace share the mark.
    expect(liveOf(session({ state: 'running' }), 7)).toBe('running')
    expect(liveOf(session({ state: 'grace' }), 7)).toBe('running')
    expect(liveOf(session({ state: 'paused' }), 7)).toBe('paused')
    expect(liveOf(session({ state: 'ended' }), 7)).toBeNull()
    expect(liveOf(session({ state: 'discarded' }), 7)).toBeNull()
    expect(liveOf(session({ gameId: 8 }), 7)).toBeNull()
    expect(liveOf(null, 7)).toBeNull()

    const { rerender } = render(<GameCard {...props({ live: 'running' })} />)
    expect(screen.getByText('Em execução')).toBeInTheDocument()
    expect(screen.queryByText('Pausado')).toBeNull()

    rerender(<GameCard {...props({ live: 'paused' })} />)
    expect(screen.getByText('Pausado')).toBeInTheDocument()
    expect(screen.queryByText('Em execução')).toBeNull()

    rerender(<GameCard {...props({ live: null })} />)
    expect(screen.queryByText('Em execução')).toBeNull()
    expect(screen.queryByText('Pausado')).toBeNull()
  })

  it('UIX-015/cartão sem capa mostra o placeholder; com capa, a miniatura', () => {
    const { container, rerender } = render(<GameCard {...props()} />)
    expect(container.querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)

    rerender(<GameCard {...props({ image: { url: '/media/7/thumb.jpg', fit: 'contain' } })} />)
    expect(container.querySelector('img')).toHaveAttribute('src', '/media/7/thumb.jpg')
  })

  it('AJU-015/cartão com image de capa mostra a miniatura com contain; com substituta, cover; sem image, placeholder', () => {
    const { container, rerender } = render(<GameCard {...props({ image: { url: '/media/7/c.jpg', fit: 'contain' } })} />)
    expect(container.querySelector('img')).toHaveAttribute('data-fit', 'contain')

    rerender(<GameCard {...props({ image: { url: '/media/7/t.jpg', fit: 'cover' } })} />)
    expect(container.querySelector('img')).toHaveAttribute('src', '/media/7/t.jpg')
    expect(container.querySelector('img')).toHaveAttribute('data-fit', 'cover')

    rerender(<GameCard {...props()} />)
    expect(container.querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)
    expect(container.querySelector('img')).not.toHaveAttribute('data-fit')
  })

  it('AJU-015/a área continua aspect-[3/4] nos três casos', () => {
    const imgs = [{ url: '/a.jpg', fit: 'contain' as const }, { url: '/b.jpg', fit: 'cover' as const }, undefined]
    for (const image of imgs) {
      const { container, unmount } = render(<GameCard {...props({ image })} />)
      const box = container.querySelector('img')?.parentElement as HTMLElement
      expect(box.className).toContain('aspect-[3/4]')
      unmount()
    }
  })

  it('UIX-042/cartão é role option com o título como nome e aria-selected conforme selected', () => {
    const { rerender } = render(<GameCard {...props({ selected: false })} />)
    const option = screen.getByRole('option', { name: 'Celeste' })
    expect(option).toHaveAttribute('aria-selected', 'false')
    expect(option).toHaveAttribute('data-game-id', '7')

    rerender(<GameCard {...props({ selected: true })} />)
    expect(screen.getByRole('option', { name: 'Celeste' })).toHaveAttribute('aria-selected', 'true')
  })

  it('UIX-019/tabbable define o tabindex do cartão: 0 no focável, -1 nos outros', () => {
    const { rerender } = render(<GameCard {...props({ tabbable: true })} />)
    expect(screen.getByRole('option')).toHaveAttribute('tabindex', '0')

    rerender(<GameCard {...props({ tabbable: false })} />)
    expect(screen.getByRole('option')).toHaveAttribute('tabindex', '-1')
  })

  it('UIX-017/um clique chama onSelect(id) e não lança', () => {
    const p = props({ game: game({ id: 7 }) })
    render(<GameCard {...p} />)

    fireEvent.click(screen.getByRole('option'))
    expect(p.onSelect).toHaveBeenCalledTimes(1)
    expect(p.onSelect).toHaveBeenCalledWith(7)
    expect(p.onLaunch).not.toHaveBeenCalled()
  })

  it('UIX-018/duplo clique chama onLaunch(id)', () => {
    const p = props({ game: game({ id: 7 }) })
    render(<GameCard {...p} />)

    fireEvent.doubleClick(screen.getByRole('option'))
    expect(p.onLaunch).toHaveBeenCalledTimes(1)
    expect(p.onLaunch).toHaveBeenCalledWith(7)
  })
})
