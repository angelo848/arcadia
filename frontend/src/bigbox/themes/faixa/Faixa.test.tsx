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
  totalPlaySeconds: 43500, lastPlayedAt: '2026-11-10T10:00:00Z', playCount: 14, backgroundUrl: null, videoUrl: null,
  // alterado em 2026-11-22 (fase 15): campos novos do modelo (BBX-056); as asserções do Faixa não mudaram.
  favorite: false, lastPlayedRelative: 'há 12 dias', screenshots: [],
  timeToBeat: [
    { label: 'História principal', value: '—' }, { label: 'História + extras', value: '—' }, { label: '100%', value: '—' },
  ],
  timeToBeatSource: null, ...o,
})
function model(o: Partial<BigBoxViewModel> = {}): BigBoxViewModel {
  const games = Array.from({ length: 15 }, (_, i) => game(i + 1))
  return {
    screen: 'browse',
    tabs: [
      { key: 'all', label: 'Todos', filter: { platformId: null } },
      { key: 'p:7', label: 'Nintendo 64', filter: { platformId: 7 } },
      { key: 'p:8', label: 'Super Nintendo', filter: { platformId: 8 } },
    ],
    tab: 1, loading: false, games, total: 40, focus: 7, stripStart: 0, counter: '8 / 40', focused: focused(games[7]),
    axis: 'horizontal', columns: 1, tabTotal: 40, sortLabel: 'Título', videoFailed: false,
    videoVisible: false, sound: false,
    hints: hintsFor('browse', { empty: false, videoVisible: false }), alert: null, ...o,
  }
}
const inPanel = (el: HTMLElement) => el.closest('[data-bb-panel]') !== null
function setup(m = model()) {
  const cb = { onPick: vi.fn(), onPlay: vi.fn(), onDetails: vi.fn(), onVideoError: vi.fn() }
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
    // alterado em 2026-11-22 (fase 15): onVideoError é obrigatório no contrato v2.
    rerender(<Faixa model={model({ games, focus: 1, total: 3, focused: focused(games[1]) })} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} onVideoError={vi.fn()} />)
    expect(screen.getAllByText('Pausado').length).toBeGreaterThanOrEqual(2)
  })

  it('BBX-020/o aviso aparece com role alert e some quando model.alert é null', () => {
    const { rerender } = setup(model({ alert: 'Já há um jogo em execução.' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Já há um jogo em execução.')
    // alterado em 2026-11-22 (fase 15): onVideoError é obrigatório no contrato v2.
    rerender(<Faixa model={model({ alert: null })} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} onVideoError={vi.fn()} />)
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('BBX-021/aba vazia: "Nenhum jogo nesta plataforma"', () => {
    setup(model({ games: [], total: 0, focus: 0, focused: null, hints: hintsFor('browse', { empty: true, videoVisible: false }) }))
    expect(screen.getByText('Nenhum jogo nesta plataforma')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Jogar' })).toBeNull()
  })

  it('BBX-021/biblioteca vazia: "Nenhum jogo na biblioteca"', () => {
    // alterado em 2026-11-22 (fase 15): abas no formato {key, label, filter}.
    setup(model({ tabs: [{ key: 'all', label: 'Todos', filter: { platformId: null } }], tab: 0, games: [], total: 0, focus: 0, focused: null }))
    expect(screen.getByText('Nenhum jogo na biblioteca')).toBeInTheDocument()
  })

  it('BBX-037/abas, título, linha de dados, descrição curta, tempo jogado, última vez, sessões e botões têm um ancestral data-bb-panel', () => {
    setup()
    expect(inPanel(screen.getByRole('tablist'))).toBe(true)
    for (const el of [
      screen.getByRole('heading', { name: /Jogo 8/ }),
      screen.getByText('Nintendo 64 · 1996 · Ação'),
      screen.getByText('Uma aventura curta.'),
      screen.getByText(/12 h 5 min/),
      screen.getByText(/10\/11\/2026/),
      screen.getByText(/Sessões: 14/),
      screen.getByRole('button', { name: 'Jogar' }),
      screen.getByRole('button', { name: 'Detalhes' }),
    ]) {
      expect(inPanel(el), el.textContent ?? '').toBe(true)
    }
  })

  it('BBX-037/"Nenhum jogo nesta plataforma", "Nenhum jogo na biblioteca" e "Carregando jogos…" ficam num painel', () => {
    const empty = { games: [], total: 0, focus: 0, focused: null, counter: null }
    const a = setup(model(empty))
    expect(inPanel(screen.getByText('Nenhum jogo nesta plataforma'))).toBe(true)
    a.unmount()
    // alterado em 2026-11-22 (fase 15): abas no formato {key, label, filter}.
    const b = setup(model({ ...empty, tabs: [{ key: 'all', label: 'Todos', filter: { platformId: null } }], tab: 0 }))
    expect(inPanel(screen.getByText('Nenhum jogo na biblioteca'))).toBe(true)
    b.unmount()
    setup(model({ ...empty, loading: true }))
    expect(inPanel(screen.getByText('Carregando jogos…'))).toBe(true)
  })

  it('BBX-038/model.loading mostra "Carregando jogos…" com as abas e sem bloco de informações', () => {
    setup(model({ loading: true, games: [], total: 0, focus: 0, focused: null, counter: null }))
    expect(screen.getByText('Carregando jogos…')).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(3)
    expect(screen.queryByRole('heading')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Jogar' })).toBeNull()
    expect(screen.queryByText('Nenhum jogo nesta plataforma')).toBeNull()
  })

  it('BBX-039/aba de 40 jogos, foco 0, stripStart 0 e 13 jogos no modelo: 13 capas, índices 0..12, a primeira em foco', () => {
    const games = Array.from({ length: 13 }, (_, i) => game(i + 1))
    setup(model({ games, focus: 0, stripStart: 0, total: 40, focused: focused(games[0]), counter: '1 / 40' }))
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers).toHaveLength(13)
    expect(covers[0]).toHaveAttribute('data-focused', 'true')
    expect(covers.filter((c) => c.getAttribute('data-focused') === 'true')).toHaveLength(1)
  })

  it('BBX-039/clique na capa i chama onPick(stripStart + i), duplo clique onPlay(stripStart + i)', () => {
    const games = Array.from({ length: 13 }, (_, i) => game(i + 19))
    const { onPick, onPlay } = setup(model({ games, focus: 20, stripStart: 18, total: 40, focused: focused(games[2]) }))
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers[2]).toHaveAttribute('data-focused', 'true')
    fireEvent.click(covers[4])
    expect(onPick).toHaveBeenCalledWith(22)
    fireEvent.doubleClick(covers[4])
    expect(onPlay).toHaveBeenCalledWith(22)
  })

  it('BBX-006/nunca mais de 15 capas no DOM', () => {
    setup(model({ games: Array.from({ length: 15 }, (_, i) => game(i + 1)), total: 500 }))
    expect(screen.getAllByTestId('bb-cover').length).toBeLessThanOrEqual(15)
  })

  it('BBX-041/a imagem da capa usa object-contain', () => {
    setup()
    const img = screen.getAllByTestId('bb-cover')[0].querySelector('img')!
    expect(img.className).toContain('object-contain')
    expect(img.className).not.toContain('object-cover')
  })

  it('BBX-045/counter "7 / 120" aparece em bb-strip-counter com ancestral data-bb-panel', () => {
    setup(model({ counter: '7 / 120', total: 120 }))
    const c = screen.getByTestId('bb-strip-counter')
    expect(c).toHaveTextContent('7 / 120')
    expect(inPanel(c)).toBe(true)
  })

  it('BBX-045/counter null (loading ou aba vazia): bb-strip-counter ausente', () => {
    const a = setup(model({ counter: null }))
    expect(screen.queryByTestId('bb-strip-counter')).toBeNull()
    a.unmount()
    setup(model({ counter: null, loading: true, games: [], total: 0, focused: null }))
    expect(screen.queryByTestId('bb-strip-counter')).toBeNull()
  })

  it('BBX-006/em Detalhes o tema mostra o elemento details', () => {
    setup(model({ screen: 'details' }))
    expect(screen.getByText('DETALHES')).toBeInTheDocument()
  })

  it('BBX-047/o Faixa segue o contrato v2: âncora 2, contain, vídeo no fundo, abas de plataforma (regressão)', () => {
    const def = BIGBOX_THEMES.find((t) => t.id === 'faixa')!
    expect(def).toMatchObject({ anchor: 2, backdrop: 'contain', videoSlot: 'background', tabs: 'platforms', sort: 'title' })
    const tabs = setup()
    expect(screen.getAllByRole('tab').map((t) => t.textContent)).toEqual(['Todos', 'Nintendo 64', 'Super Nintendo'])
    tabs.unmount()
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
