import { describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { GameCard } from './GameCard'
import type { GameView } from '../lib/library'

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

function handlers() {
  return { onOpen: vi.fn(), onPlay: vi.fn(), onPlayUntracked: vi.fn() }
}

function precedes(a: Node, b: Node): boolean {
  return (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
}

// cardOf returns the smallest ancestor of the title that holds the card's buttons,
// without assuming a particular markup.
function cardOf(title: string): HTMLElement {
  let el: HTMLElement | null = screen.getByText(title)
  while (el && within(el).queryAllByRole('button').length === 0) {
    el = el.parentElement
  }
  if (!el) {
    throw new Error(`no card with buttons for ${title}`)
  }
  return el
}

describe('GameCard', () => {
  it('UI-029/card mostra título, plataforma e tempo total no formato de UI-005', () => {
    render(<GameCard game={game()} live={null} busy={null} {...handlers()} />)

    expect(screen.getByText('Celeste')).toBeInTheDocument()
    expect(screen.getByText('Windows')).toBeInTheDocument()
    // 3900 s = 1 h 05 min, the same format as the list [UI-005].
    expect(screen.getByText('1 h 05 min')).toBeInTheDocument()

    cleanup()
    render(<GameCard game={game({ id: 8, title: 'Hades', platform: 'Linux', totalPlaySeconds: 0 })} live={null} busy={null} {...handlers()} />)
    expect(screen.getByText('Hades')).toBeInTheDocument()
    expect(screen.getByText('Linux')).toBeInTheDocument()
    expect(screen.getByText('Nunca jogado')).toBeInTheDocument()
  })

  it('UI-006/card com sessão running ou grace mostra \'Em execução\'; paused mostra \'Pausado\'', () => {
    const { rerender } = render(<GameCard game={game()} live="running" busy={null} {...handlers()} />)
    expect(screen.getByText('Em execução')).toBeInTheDocument()
    expect(screen.queryByText('Pausado')).toBeNull()

    rerender(<GameCard game={game()} live="paused" busy={null} {...handlers()} />)
    expect(screen.getByText('Pausado')).toBeInTheDocument()
    expect(screen.queryByText('Em execução')).toBeNull()
  })

  it('UI-043/marcação some quando a sessão vira ended ou discarded', () => {
    // The parent passes live = null for ended and discarded sessions [UI-006].
    const { rerender } = render(<GameCard game={game()} live="running" busy={null} {...handlers()} />)
    expect(screen.getByText('Em execução')).toBeInTheDocument()

    rerender(<GameCard game={game()} live={null} busy={null} {...handlers()} />)
    expect(screen.queryByText('Em execução')).toBeNull()
    expect(screen.queryByText('Pausado')).toBeNull()
  })

  it('UI-031/card tem os botões \'Jogar\' e \'Sem contar tempo\'', () => {
    render(<GameCard game={game()} live={null} busy={null} {...handlers()} />)

    const play = screen.getByRole('button', { name: 'Jogar' })
    const untracked = screen.getByRole('button', { name: 'Sem contar tempo' })
    expect(play).toBeEnabled()
    expect(untracked).toBeEnabled()
    expect(precedes(play, untracked)).toBe(true)
  })

  it('UI-031/pendência de um jogo desabilita os dois botões e mostra \'Iniciando…\' no acionado', () => {
    const cases = [
      { busy: 'tracked' as const, other: 'Sem contar tempo' },
      { busy: 'untracked' as const, other: 'Jogar' },
    ]
    for (const { busy, other } of cases) {
      const { unmount } = render(<GameCard game={game()} live={null} busy={busy} {...handlers()} />)

      const triggered = screen.getByRole('button', { name: 'Iniciando…' })
      const otherBtn = screen.getByRole('button', { name: other })
      expect(triggered, `${busy}: triggered`).toBeDisabled()
      expect(otherBtn, `${busy}: other`).toBeDisabled()
      // The triggered button keeps its position: Jogar first, Sem contar tempo second.
      const sameOrder = busy === 'tracked' ? precedes(triggered, otherBtn) : precedes(otherBtn, triggered)
      expect(sameOrder, `${busy}: order`).toBe(true)
      unmount()
    }
  })

  it('UI-019/UI-021/botões do card seguem o bloqueio de pendência da fase 4', () => {
    const h = handlers()
    const { rerender } = render(<GameCard game={game({ id: 7 })} live={null} busy={null} {...h} />)

    fireEvent.click(screen.getByRole('button', { name: 'Jogar' }))
    expect(h.onPlay).toHaveBeenCalledTimes(1)
    expect(h.onPlay).toHaveBeenCalledWith(7)
    fireEvent.click(screen.getByRole('button', { name: 'Sem contar tempo' }))
    expect(h.onPlayUntracked).toHaveBeenCalledTimes(1)
    expect(h.onPlayUntracked).toHaveBeenCalledWith(7)

    // While a launch of this game is pending, neither button triggers anything.
    rerender(<GameCard game={game({ id: 7 })} live={null} busy="tracked" {...h} />)
    const buttons = within(cardOf('Celeste')).getAllByRole('button')
    expect(buttons).toHaveLength(2)
    for (const btn of buttons) {
      expect(btn).toBeDisabled()
      fireEvent.click(btn)
    }
    expect(h.onPlay).toHaveBeenCalledTimes(1)
    expect(h.onPlayUntracked).toHaveBeenCalledTimes(1)
  })
})
