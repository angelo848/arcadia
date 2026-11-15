import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { hintsFor } from '../../hints'
import type { BigBoxFocused, BigBoxGame, BigBoxViewModel } from '../../viewModel'
import { BIGBOX_THEMES } from '../registry'
import { Faixa } from './Faixa'

const sources = import.meta.glob<string>(['./Faixa.tsx', './FaixaStrip.tsx', './FaixaInfo.tsx', '../../Footer.tsx'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

// Assumed: Faixa(props: BigBoxThemeProps) named export, also registered as BIGBOX_THEMES[faixa].Component.
// Assumed markup: each cover is role="button"-like element with data-testid="bb-cover" and data-focused="true" on the focused one;
// the footer is rendered by the shell, not by Faixa. Tabs are role="tab" with aria-selected.
const game = (id: number, o: Partial<BigBoxGame> = {}): BigBoxGame => ({
  id, title: `Jogo ${id}`, coverUrl: `http://m/${id}.jpg`, session: null, ...o,
})
const focused = (g: BigBoxGame, o: Partial<BigBoxFocused> = {}): BigBoxFocused => ({
  game: g, platform: 'Nintendo 64', year: 1996, genre: 'Ação', shortDescription: 'Uma aventura curta.',
  playTimeLabel: '12 h 5 min', lastPlayedLabel: '10/11/2026', sessionsLabel: 'Sessões: 14',
  totalPlaySeconds: 43500, lastPlayedAt: '2026-11-10T10:00:00Z', playCount: 14, backgroundUrl: null, videoUrl: null, ...o,
})
function model(o: Partial<BigBoxViewModel> = {}): BigBoxViewModel {
  const games = Array.from({ length: 15 }, (_, i) => game(i + 1))
  return {
    screen: 'browse',
    tabs: [{ platformId: null, label: 'Todos' }, { platformId: 7, label: 'Nintendo 64' }, { platformId: 8, label: 'Super Nintendo' }],
    tab: 1, games, total: 40, focus: 7, focused: focused(games[7]), videoVisible: false, sound: false,
    hints: hintsFor('browse', { empty: false, videoVisible: false }), alert: null, ...o,
  }
}
function setup(m = model()) {
  const cb = { onPick: vi.fn(), onPlay: vi.fn(), onDetails: vi.fn() }
  const r = render(<Faixa model={m} details={<div>DETALHES</div>} {...cb} />)
  return { ...cb, ...r }
}

describe('Faixa', () => {
  it('BBX-006/as abas são só texto, sem ícone; Todos e cada plataforma, com a ativa destacada', () => {
    setup()
    const tabs = screen.getAllByRole('tab')
    expect(tabs.map((t) => t.textContent)).toEqual(['Todos', 'Nintendo 64', 'Super Nintendo'])
    tabs.forEach((t) => {
      expect(t.querySelector('img,svg')).toBeNull()
    })
    expect(tabs[1]).toHaveAttribute('aria-selected', 'true')
    expect(tabs[0]).toHaveAttribute('aria-selected', 'false')
  })

  it('BBX-006/no máximo 15 capas no DOM e a em foco marcada', () => {
    setup()
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers.length).toBeLessThanOrEqual(15)
    const marked = covers.filter((c) => c.getAttribute('data-focused') === 'true')
    expect(marked).toHaveLength(1)
    expect(marked[0].textContent + (marked[0].getAttribute('aria-label') ?? '')).toContain('Jogo 8')
  })

  it('BBX-008/capa, substituta e placeholder com o título', () => {
    const games = [game(1), game(2, { coverUrl: null }), game(3)]
    setup(model({ games, focus: 0, focused: focused(games[0]) }))
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers[0].querySelector('img')?.getAttribute('src')).toBe('http://m/1.jpg')
    expect(covers[1].querySelector('img')).toBeNull()
    expect(within(covers[1]).getByText('Jogo 2')).toBeInTheDocument()
  })

  it('BBX-009/o bloco mostra título, linha de dados, descrição curta, tempo jogado, última vez, sessões e os botões Jogar e Detalhes', () => {
    setup()
    expect(screen.getByRole('heading', { name: /Jogo 8/ })).toBeInTheDocument()
    expect(screen.getByText('Nintendo 64 · 1996 · Ação')).toBeInTheDocument()
    expect(screen.getByText('Uma aventura curta.')).toBeInTheDocument()
    expect(screen.getByText(/12 h 5 min/)).toBeInTheDocument()
    expect(screen.getByText(/10\/11\/2026/)).toBeInTheDocument()
    expect(screen.getByText(/Sessões: 14/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Jogar' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Detalhes' })).toBeInTheDocument()
  })

  it('BBX-009/nunca jogado mostra "Nunca jogado" e some a última vez', () => {
    const g = game(8)
    setup(model({ focused: focused(g, { playTimeLabel: 'Nunca jogado', lastPlayedLabel: null, lastPlayedAt: null, playCount: 0, sessionsLabel: 'Sessões: 0' }) }))
    expect(screen.getByText(/Nunca jogado/)).toBeInTheDocument()
    expect(screen.queryByText(/Última vez/)).toBeNull()
  })

  it('BBX-009/partes ausentes somem sem deixar separador', () => {
    const g = game(8)
    setup(model({ focused: focused(g, { year: null, genre: null, shortDescription: '' }) }))
    expect(screen.getByText('Nintendo 64')).toBeInTheDocument()
    expect(screen.queryByText(/·\s*·/)).toBeNull()
    expect(screen.queryByText(/Nintendo 64 ·\s*$/)).toBeNull()
  })

  it('BBX-014/clique na capa a põe em foco, duplo clique lança, os botões Jogar e Detalhes chamam onPlay e onDetails', () => {
    const { onPick, onPlay, onDetails } = setup()
    const covers = screen.getAllByTestId('bb-cover')
    fireEvent.click(covers[2])
    expect(onPick).toHaveBeenCalledTimes(1)
    fireEvent.doubleClick(covers[2])
    expect(onPlay).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Jogar' }))
    expect(onPlay).toHaveBeenCalledTimes(2)
    fireEvent.click(screen.getByRole('button', { name: 'Detalhes' }))
    expect(onDetails).toHaveBeenCalledTimes(1)
  })

  it('BBX-017/selo Em execução e Pausado na capa e ao lado do título', () => {
    const games = [game(1, { session: 'running' }), game(2, { session: 'paused' }), game(3)]
    const { rerender } = setup(model({ games, focus: 0, total: 3, focused: focused(games[0]) }))
    expect(screen.getAllByText('Em execução').length).toBeGreaterThanOrEqual(2) // cover seal + beside title
    expect(screen.getAllByText('Pausado').length).toBeGreaterThanOrEqual(1) // cover seal of game 2
    rerender(<Faixa model={model({ games, focus: 1, total: 3, focused: focused(games[1]) })} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} />)
    expect(screen.getAllByText('Pausado').length).toBeGreaterThanOrEqual(2)
  })

  it('BBX-020/o aviso aparece com role alert e some quando model.alert é null', () => {
    const { rerender } = setup(model({ alert: 'Já há um jogo em execução.' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Já há um jogo em execução.')
    rerender(<Faixa model={model({ alert: null })} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} />)
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('BBX-021/aba vazia: "Nenhum jogo nesta plataforma"', () => {
    setup(model({ games: [], total: 0, focus: 0, focused: null, hints: hintsFor('browse', { empty: true, videoVisible: false }) }))
    expect(screen.getByText('Nenhum jogo nesta plataforma')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Jogar' })).toBeNull()
  })

  it('BBX-021/biblioteca vazia: "Nenhum jogo na biblioteca"', () => {
    setup(model({ tabs: [{ platformId: null, label: 'Todos' }], tab: 0, games: [], total: 0, focus: 0, focused: null }))
    expect(screen.getByText('Nenhum jogo na biblioteca')).toBeInTheDocument()
  })

  it('BBX-006/em Detalhes o tema mostra o elemento details', () => {
    setup(model({ screen: 'details' }))
    expect(screen.getByText('DETALHES')).toBeInTheDocument()
  })

  it('BBX-014/o componente segue o contrato de tema do registry', () => {
    const def = BIGBOX_THEMES.find((t) => t.id === 'faixa')!
    expect(def.available).toBe(true)
    expect(def.axis).toBe('horizontal')
    expect(def.Component).toBe(Faixa)
  })

  it('BBX-020/só tokens --arc-bb-* e nenhuma cor literal nos arquivos do tema', () => {
    const entries = Object.entries(sources)
    expect(entries.map(([p]) => p.split('/').pop()).sort()).toEqual(['Faixa.tsx', 'FaixaInfo.tsx', 'FaixaStrip.tsx', 'Footer.tsx'])
    for (const [f, src] of entries) {
      expect(src, f).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
      expect(src, f).not.toMatch(/\brgba?\(|\bhsla?\(/)
      expect(src, f).not.toMatch(/\b(?:bg|text|border|ring)-(?:slate|gray|zinc|red|blue|green|white|black)\b/)
    }
  })
})
