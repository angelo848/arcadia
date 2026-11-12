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
// the game's row, without assuming a particular markup (li, tr, div...). Each
// row now has two buttons ("Jogar" and "Sem contar tempo"), so the caller picks
// one of them by name.
function rowOf(title: string): HTMLElement {
  let el: HTMLElement | null = screen.getByText(title)
  while (el && within(el).queryAllByRole('button').length === 0) {
    el = el.parentElement
  }
  if (!el) {
    throw new Error(`no row with a button for ${title}`)
  }
  return el
}

function precedes(a: Node, b: Node): boolean {
  return (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
}

describe('GameList', () => {
  it('UI-004 uma linha por jogo na ordem recebida, com título, total e Jogar', () => {
    const onPlay = vi.fn()
    render(
      <GameList
        games={games}
        current={null}
        pending={null}
        onPlay={onPlay}
        onPlayUntracked={() => {}}
      />,
    )

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
      <GameList
        games={games}
        current={session({ gameId: 10 })}
        pending={null}
        onPlay={() => {}}
        onPlayUntracked={() => {}}
      />,
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
        pending={null}
        onPlay={() => {}}
        onPlayUntracked={() => {}}
      />,
    )
    expect(screen.getAllByText(/Em execução/)).toHaveLength(1)
    expect(rowOf('Hades')).toHaveTextContent('Em execução')
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')

    rerender(
      <GameList games={games} current={null} pending={null} onPlay={() => {}} onPlayUntracked={() => {}} />,
    )
    expect(screen.queryByText(/Em execução/)).toBeNull()
  })

  it('UI-008 Jogar pendente mostra Iniciando… desabilitado', () => {
    const onPlay = vi.fn()
    render(
      <GameList
        games={games}
        current={null}
        pending={{ id: 10, mode: 'tracked' }}
        onPlay={onPlay}
        onPlayUntracked={() => {}}
      />,
    )

    const pending = within(rowOf('Hollow Knight')).getByRole('button', { name: 'Iniciando…' })
    expect(pending).toBeDisabled()
    fireEvent.click(pending)
    expect(onPlay).not.toHaveBeenCalled()

    for (const title of ['Celeste', 'Hades']) {
      const btn = within(rowOf(title)).getByRole('button', { name: 'Jogar' })
      expect(btn, title).toBeEnabled()
    }
    expect(screen.getAllByRole('button', { name: 'Iniciando…' })).toHaveLength(1)
  })

  it('UI-019/cada linha mostra Jogar e Sem contar tempo', () => {
    const onPlay = vi.fn()
    const onPlayUntracked = vi.fn()
    render(
      <GameList
        games={games}
        current={null}
        pending={null}
        onPlay={onPlay}
        onPlayUntracked={onPlayUntracked}
      />,
    )

    expect(screen.getAllByRole('button', { name: 'Sem contar tempo' })).toHaveLength(3)
    for (const g of games) {
      const row = rowOf(g.title)
      const play = within(row).getByRole('button', { name: 'Jogar' })
      const untracked = within(row).getByRole('button', { name: 'Sem contar tempo' })
      expect(play, `Jogar of ${g.title}`).toBeEnabled()
      expect(untracked, `Sem contar tempo of ${g.title}`).toBeEnabled()
      // The untracked button sits next to Jogar.
      expect(precedes(play, untracked), `order in ${g.title}`).toBe(true)
    }

    fireEvent.click(within(rowOf('Hades')).getByRole('button', { name: 'Sem contar tempo' }))
    expect(onPlayUntracked).toHaveBeenCalledTimes(1)
    expect(onPlayUntracked).toHaveBeenCalledWith(20)
    expect(onPlay).not.toHaveBeenCalled()
  })

  it('UI-021/lançamento pendente do jogo G desabilita os dois botões de G, só o acionado mostra Iniciando… e os outros jogos continuam habilitados', () => {
    // The triggered button shows Iniciando…; the other one keeps its own label.
    const cases = [
      { mode: 'tracked' as const, otherLabel: 'Sem contar tempo' },
      { mode: 'untracked' as const, otherLabel: 'Jogar' },
    ]
    for (const { mode, otherLabel } of cases) {
      const onPlay = vi.fn()
      const onPlayUntracked = vi.fn()
      const { unmount } = render(
        <GameList
          games={games}
          current={null}
          pending={{ id: 10, mode }}
          onPlay={onPlay}
          onPlayUntracked={onPlayUntracked}
        />,
      )

      const row = rowOf('Hollow Knight')
      const triggered = within(row).getByRole('button', { name: 'Iniciando…' })
      const other = within(row).getByRole('button', { name: otherLabel })
      expect(triggered, `${mode}: triggered button`).toBeDisabled()
      expect(other, `${mode}: other button of G`).toBeDisabled()

      fireEvent.click(triggered)
      fireEvent.click(other)
      expect(onPlay, `${mode}: onPlay`).not.toHaveBeenCalled()
      expect(onPlayUntracked, `${mode}: onPlayUntracked`).not.toHaveBeenCalled()

      for (const title of ['Celeste', 'Hades']) {
        const r = rowOf(title)
        expect(within(r).getByRole('button', { name: 'Jogar' }), `${mode}: Jogar of ${title}`).toBeEnabled()
        expect(
          within(r).getByRole('button', { name: 'Sem contar tempo' }),
          `${mode}: Sem contar tempo of ${title}`,
        ).toBeEnabled()
      }
      expect(screen.getAllByRole('button', { name: 'Iniciando…' })).toHaveLength(1)
      unmount()
    }
  })

  it('UI-024/sessão paused mostra Pausado no lugar de Em execução; running e grace mostram Em execução', () => {
    const { rerender } = render(
      <GameList
        games={games}
        current={session({ gameId: 10, state: 'paused' })}
        pending={null}
        onPlay={() => {}}
        onPlayUntracked={() => {}}
      />,
    )
    expect(rowOf('Hollow Knight')).toHaveTextContent('Pausado')
    expect(rowOf('Hollow Knight')).not.toHaveTextContent('Em execução')
    expect(screen.queryByText(/Em execução/)).toBeNull()
    expect(screen.getAllByText('Pausado')).toHaveLength(1)
    expect(rowOf('Celeste')).not.toHaveTextContent('Pausado')

    for (const state of ['running', 'grace'] as const) {
      rerender(
        <GameList
          games={games}
          current={session({ gameId: 10, state })}
          pending={null}
          onPlay={() => {}}
          onPlayUntracked={() => {}}
        />,
      )
      expect(rowOf('Hollow Knight'), state).toHaveTextContent('Em execução')
      expect(rowOf('Hollow Knight'), state).not.toHaveTextContent('Pausado')
      expect(screen.queryByText('Pausado'), state).toBeNull()
    }
  })

  it('UI-024/ended ou discarded: a marcação some', () => {
    const { rerender } = render(
      <GameList
        games={games}
        current={session({ gameId: 10, state: 'paused' })}
        pending={null}
        onPlay={() => {}}
        onPlayUntracked={() => {}}
      />,
    )
    for (const state of ['ended', 'discarded'] as const) {
      rerender(
        <GameList
          games={games}
          current={session({ gameId: 10, state, endedAt: '2026-11-09T10:10:00.000Z' })}
          pending={null}
          onPlay={() => {}}
          onPlayUntracked={() => {}}
        />,
      )
      expect(screen.queryByText(/Em execução|Pausado/), state).toBeNull()
      expect(rowOf('Hollow Knight'), state).not.toHaveTextContent('Em execução')
      expect(rowOf('Hollow Knight'), state).not.toHaveTextContent('Pausado')
    }
  })
})
