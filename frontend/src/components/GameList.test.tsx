import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { GameList } from './GameList'
import type { GameView, SessionView } from '../lib/games'

function game(over: Partial<GameView> = {}): GameView {
  return {
    id: 10,
    title: 'Hollow Knight',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    totalPlaySeconds: 0,
    playCount: 0,
    lastPlayedAt: '',
    ...over,
  }
}

function session(over: Partial<SessionView> = {}): SessionView {
  return {
    sessionId: 1,
    gameId: 10,
    title: 'Hollow Knight',
    state: 'running',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '',
    activeSeconds: 0,
    activeMs: 0,
    asOf: '2026-11-09T10:00:00.000Z',
    ...over,
  }
}

const games: GameView[] = [
  game({ id: 30, title: 'Celeste', totalPlaySeconds: 3900 }),
  game({ id: 10, title: 'Hollow Knight', totalPlaySeconds: 0 }),
  game({ id: 20, title: 'Hades', totalPlaySeconds: 30 }),
]

// rowOf returns the smallest ancestor of the title that contains a button:
// the game's row, without assuming a particular markup (li, tr, div...).
function rowOf(title: string): HTMLElement {
  let el: HTMLElement | null = screen.getByText(title)
  while (el && within(el).queryAllByRole('button').length === 0) {
    el = el.parentElement
  }
  if (!el) {
    throw new Error(`no row with a button for ${title}`)
  }
  expect(within(el).getAllByRole('button'), `one button in the row of ${title}`).toHaveLength(1)
  return el
}

function precedes(a: Node, b: Node): boolean {
  return (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
}

describe('GameList', () => {
  it('UI-004 uma linha por jogo na ordem recebida, com título, total e Jogar', () => {
    const onPlay = vi.fn()
    render(<GameList games={games} current={null} pendingId={null} onPlay={onPlay} />)

    expect(screen.getAllByRole('button', { name: 'Jogar' })).toHaveLength(3)

    const titles = games.map((g) => screen.getByText(g.title))
    expect(precedes(titles[0], titles[1])).toBe(true)
    expect(precedes(titles[1], titles[2])).toBe(true)

    const want: Array<[string, string]> = [
      ['Celeste', '1 h 05 min'],
      ['Hollow Knight', 'Nunca jogado'],
      ['Hades', '< 1 min'],
    ]
    for (const [title, total] of want) {
      const row = rowOf(title)
      expect(row, `total of ${title}`).toHaveTextContent(total)
      const play = within(row).getByRole('button', { name: 'Jogar' })
      expect(play).toBeEnabled()
    }

    // Each Jogar triggers onPlay with its own game id.
    fireEvent.click(within(rowOf('Hades')).getByRole('button', { name: 'Jogar' }))
    expect(onPlay).toHaveBeenCalledTimes(1)
    expect(onPlay).toHaveBeenCalledWith(20)
    fireEvent.click(within(rowOf('Celeste')).getByRole('button', { name: 'Jogar' }))
    expect(onPlay).toHaveBeenLastCalledWith(30)
  })

  it('UI-006 Em execução só na linha do gameId da sessão', () => {
    const { rerender } = render(
      <GameList games={games} current={session({ gameId: 10 })} pendingId={null} onPlay={() => {}} />,
    )
    expect(screen.getAllByText(/Em execução/)).toHaveLength(1)
    expect(rowOf('Hollow Knight')).toHaveTextContent('Em execução')
    expect(rowOf('Celeste')).not.toHaveTextContent('Em execução')
    expect(rowOf('Hades')).not.toHaveTextContent('Em execução')

    // grace also marks the row.
    rerender(
      <GameList
        games={games}
        current={session({ gameId: 20, state: 'grace' })}
        pendingId={null}
        onPlay={() => {}}
      />,
    )
    expect(screen.getAllByText(/Em execução/)).toHaveLength(1)
    expect(rowOf('Hades')).toHaveTextContent('Em execução')
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')

    rerender(<GameList games={games} current={null} pendingId={null} onPlay={() => {}} />)
    expect(screen.queryByText(/Em execução/)).toBeNull()
  })

  it('UI-008 Jogar pendente mostra Iniciando… desabilitado', () => {
    const onPlay = vi.fn()
    render(<GameList games={games} current={null} pendingId={10} onPlay={onPlay} />)

    const pending = within(rowOf('Hollow Knight')).getByRole('button')
    expect(pending).toHaveAccessibleName('Iniciando…')
    expect(pending).toBeDisabled()
    fireEvent.click(pending)
    expect(onPlay).not.toHaveBeenCalled()

    for (const title of ['Celeste', 'Hades']) {
      const btn = within(rowOf(title)).getByRole('button')
      expect(btn, title).toHaveAccessibleName('Jogar')
      expect(btn, title).toBeEnabled()
    }
    expect(screen.getAllByRole('button', { name: 'Iniciando…' })).toHaveLength(1)
  })
})
