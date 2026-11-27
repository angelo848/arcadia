import type { ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { hintsFor } from '../../hints'
import { BigBox } from '../../BigBox'
import { useMediaEvents } from '../../../lib/metadata'
import { useTimeToBeatEvents } from '../../../lib/timeToBeat'
import type { BigBoxFocused, BigBoxGame, BigBoxViewModel } from '../../viewModel'
import { BIGBOX_THEMES } from '../registry'
import { Estante } from './Estante'
import { EstanteDetails } from './EstanteDetails'

const sources = import.meta.glob<string>(['./*.tsx', './capacity.ts', '!./*.test.tsx'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

// Assumed markup (T36 must follow):
//  root est-root; hero slot est-hero-slot (always present, inline/Tailwind transition of 300 ms) holding est-hero only while the focus is on row 0;
//  hero: title est-title (h1, line-clamp-2, clamp(2.4rem, 4vw, 4.5rem), bb-heading), est-last ("Última sessão <relativo>" or "Nunca jogado"),
//  buttons "Iniciar jogo" (svg, bb-accent/bb-on-accent) and "Detalhes", est-favorite (aria-label Favorito/Não favorito, not a button),
//  blocks est-stat-time ("Tempo jogado") and est-stat-sessions ("Sessões"), both inside Panel;
//  header est-header: "Sua biblioteca", est-order (model.sortLabel), role="tablist" with role="tab" (aria-selected; active: bb-accent + bb-on-accent;
//  active label "<nome> (<tabTotal>)"), chips est-chip-lb / est-chip-rb (bb-chip / bb-on-chip) before the first and after the last tab;
//  grid est-grid (grid-cols-5 or data-columns="5", overflow hidden) with one est-card per game (data-focused) holding bb-cover (aria-label = title,
//  aspect 2/3, img object-contain, focus = 3px bb-focus outline + scale 1.05, session badge inside), est-card-title (truncate) and est-card-time
//  (game.playTimeLabel), both inside a Panel. The theme never draws a <video> or the art (videoSlot background; the shell owns BackgroundStage).
const game = (id: number, o: Partial<BigBoxGame> = {}): BigBoxGame => ({
  id, title: `Jogo ${id}`, coverUrl: `http://m/${id}.jpg`, session: null, playTimeLabel: id % 2 ? 'Jogou por 28 h 10 min' : 'Nunca jogado', ...o,
})
const focused = (g: BigBoxGame, o: Partial<BigBoxFocused> = {}): BigBoxFocused => ({
  game: g, platform: 'Nintendo 64', year: 2000, genre: 'Ação', shortDescription: 'Uma aventura curta.',
  playTimeLabel: '28 h 10 min', lastPlayedLabel: '12/10/2026', sessionsLabel: 'Sessões: 14',
  totalPlaySeconds: 101400, lastPlayedAt: '2026-11-15T10:00:00Z', playCount: 14, backgroundUrl: 'http://m/bg.jpg', videoUrl: null,
  favorite: false, lastPlayedRelative: 'há 7 dias', screenshots: [],
  timeToBeat: [
    { label: 'História principal', value: '20 h' }, { label: 'História + extras', value: '31½ h' }, { label: '100%', value: '41 h' },
  ],
  timeToBeatSource: 'HowLongToBeat', ...o,
})
const TABS = [
  { key: 'all', label: 'Todos', filter: { platformId: null } },
  { key: 'favorites', label: 'Favoritos', filter: { platformId: null, favorite: true as const } },
  { key: 'completed', label: 'Concluídos', filter: { platformId: null, status: 'completed' as const } },
  { key: 'p:7', label: 'Nintendo 64', filter: { platformId: 7 } },
]
// Row 0 of a 5-column grid: the window starts at 0 with two rows (10 games); `focus` is the absolute index.
// alterado em 2026-11-27 (fase 17): o modelo ganha layout, zone e view.
function model(o: Partial<BigBoxViewModel> = {}): BigBoxViewModel {
  const games = Array.from({ length: 10 }, (_, i) => game(i + 1))
  const focus = o.focus ?? 2
  const start = o.stripStart ?? 0
  return {
    screen: 'browse', tabs: TABS, tab: 0, loading: false, games, total: 40, focus, stripStart: start, counter: `${focus + 1} / 40`,
    focused: focused(games[focus - start] ?? games[0]),
    axis: 'grid', columns: 5, tabTotal: 23, sortLabel: 'Jogado recentemente', videoFailed: false, videoVisible: false, sound: false,
    hints: hintsFor('browse', { empty: false, videoVisible: false }, 'Aba'), alert: null,
    layout: 'screens', zone: 'games', view: 'grid', ...o,
  }
}
const noop = () => {}
const inPanel = (el: Element | null) => el?.closest('[data-bb-panel]') != null
const after = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
const look = (el: Element) => `${el.className} ${el.getAttribute('style') ?? ''}`
function setup(m = model()) {
  const cb = { onPick: vi.fn(), onPlay: vi.fn(), onDetails: vi.fn(), onVideoError: vi.fn() }
  const r = render(<Estante model={m} details={<div>DETALHES</div>} {...cb} />)
  const again = (n: BigBoxViewModel) => r.rerender(<Estante model={n} details={null} onPick={noop} onPlay={noop} onDetails={noop} onVideoError={noop} />)
  return { ...cb, ...r, again }
}
const EMPTY = { games: [], total: 0, focus: 0, stripStart: 0, focused: null, counter: null, tabTotal: 0 }

describe('Estante: tela Navegar', () => {
  beforeEach(() => {
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(() => Promise.resolve())
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('ETN-001/com o foco na fileira 0: destaque, cabeçalho "Sua biblioteca" e grade, nesta ordem, com as duas fileiras no DOM (a de baixo é cortada)', () => {
    const { container } = setup()
    const hero = screen.getByTestId('est-hero')
    const header = screen.getByTestId('est-header')
    const grid = screen.getByTestId('est-grid')
    expect(within(header).getByText('Sua biblioteca')).toBeInTheDocument()
    expect(after(hero, header)).toBe(true)
    expect(after(header, grid)).toBe(true)
    expect(within(grid).getAllByTestId('bb-cover')).toHaveLength(10)
    expect(look(grid)).toMatch(/overflow-hidden|overflow:\s*hidden/)
    expect(container.querySelector('nav, aside')).toBeNull()
  })

  it('ETN-001/o tema não desenha arte nem vídeo de fundo (o fundo cover e o vídeo de BBX-011 são do shell)', () => {
    const f = focused(game(3), { videoUrl: 'http://m/3/v.mp4' })
    const { container } = setup(model({ videoVisible: true, focused: f }))
    expect(container.querySelectorAll('video')).toHaveLength(0)
    expect(container.querySelector('[data-testid="bb-art"], [data-testid="bb-art-layer"], [data-testid="bb-cover-gradient"]')).toBeNull()
  })

  it('ETN-001/o tema segue o registro: disponível, grade, Component e Details da Estante', () => {
    const def = BIGBOX_THEMES.find((t) => t.id === 'estante')!
    expect(def.available).toBe(true)
    expect(def.Component).toBe(Estante)
    expect(def.Details).toBe(EstanteDetails)
  })

  it('ETN-002/destaque: título grande em texto (até duas linhas, clamp(2.4rem, 4vw, 4.5rem), heading) do jogo em foco', () => {
    const { again } = setup()
    const hero = screen.getByTestId('est-hero')
    const title = within(hero).getByRole('heading', { name: 'Jogo 3' })
    expect(title).toBe(screen.getByTestId('est-title'))
    expect(look(title)).toMatch(/clamp\(\s*2\.4rem\s*,\s*4vw\s*,\s*4\.5rem\s*\)/)
    expect(look(title)).toMatch(/line-clamp-2|line-clamp:\s*2/)
    expect(look(title)).toMatch(/bb-heading/)
    expect(hero.querySelector('img')).toBeNull() // no logo: the title is text (ETN-013)
    again(model({ focus: 4, focused: focused(game(5)) }))
    expect(screen.getByTestId('est-title')).toHaveTextContent('Jogo 5')
  })

  it('ETN-002/"Última sessão <relativo>" com hoje, ontem, há N dias, há N meses e há N anos', () => {
    const { again } = setup()
    for (const rel of ['hoje', 'ontem', 'há 7 dias', 'há 29 dias', 'há 3 meses', 'há 11 meses', 'há 2 anos']) {
      again(model({ focused: focused(game(3), { lastPlayedRelative: rel }) }))
      expect(screen.getByTestId('est-last'), rel).toHaveTextContent(`Última sessão ${rel}`)
    }
  })

  it('ETN-002/nunca jogado mostra "Nunca jogado" e não "Última sessão"', () => {
    setup(model({ focused: focused(game(3), { lastPlayedAt: null, lastPlayedRelative: 'Nunca jogado', playCount: 0 }) }))
    const last = screen.getByTestId('est-last')
    expect(last).toHaveTextContent('Nunca jogado')
    expect(last.textContent).not.toMatch(/Última sessão/)
  })

  it('ETN-002/"Iniciar jogo" (play, accent) age como A e "Detalhes" age como X', () => {
    const { onPlay, onDetails } = setup()
    const play = screen.getByRole('button', { name: 'Iniciar jogo' })
    expect(play.querySelector('svg')).not.toBeNull()
    expect(look(play)).toMatch(/bb-accent/)
    expect(look(play)).toMatch(/bb-on-accent/)
    const details = screen.getByRole('button', { name: 'Detalhes' })
    expect(look(details)).toMatch(/bb-surface/)
    fireEvent.click(play)
    expect(onPlay).toHaveBeenCalledTimes(1)
    expect(onPlay).toHaveBeenCalledWith(2)
    fireEvent.click(details)
    expect(onDetails).toHaveBeenCalledTimes(1)
  })

  it('ETN-002/o coração é só um indicador: cheio com favorite, vazio sem, nunca botão nem com onClick', () => {
    const a = setup(model({ focused: focused(game(3), { favorite: true }) }))
    const full = screen.getByLabelText('Favorito')
    expect(screen.queryByLabelText('Não favorito')).toBeNull()
    expect(full.closest('button')).toBeNull()
    expect(screen.queryByRole('button', { name: /favorit/i })).toBeNull()
    fireEvent.click(full)
    expect(a.onPick).not.toHaveBeenCalled()
    expect(a.onPlay).not.toHaveBeenCalled()
    expect(a.onDetails).not.toHaveBeenCalled()
    a.again(model({ focused: focused(game(3), { favorite: false }) }))
    expect(screen.getByLabelText('Não favorito')).toBeInTheDocument()
    expect(screen.queryByLabelText('Favorito')).toBeNull()
  })

  it('ETN-002/marcação de sessão ao lado do título e no selo da capa (BBX-017)', () => {
    const games = [game(1), game(2), game(3, { session: 'running' }), game(4, { session: 'paused' }), game(5)]
    setup(model({ games, focus: 2, total: 5, focused: focused(games[2]) }))
    expect(within(screen.getByTestId('est-hero')).getByText('Em execução')).toBeInTheDocument()
    const covers = screen.getAllByTestId('bb-cover')
    expect(within(covers[2]).getByText('Em execução')).toBeInTheDocument()
    expect(within(covers[3]).getByText('Pausado')).toBeInTheDocument()
    expect(within(covers[0]).queryByText(/Em execução|Pausado/)).toBeNull()
  })

  it('ETN-003/blocos "Tempo jogado" e "Sessões" à direita do destaque, em painéis', () => {
    setup()
    const time = screen.getByTestId('est-stat-time')
    expect(within(time).getByText('Tempo jogado')).toBeInTheDocument()
    expect(within(time).getByText('28 h 10 min')).toBeInTheDocument()
    expect(inPanel(time)).toBe(true)
    const sessions = screen.getByTestId('est-stat-sessions')
    expect(within(sessions).getByText('Sessões')).toBeInTheDocument()
    expect(within(sessions).getByText('14')).toBeInTheDocument()
    expect(inPanel(sessions)).toBe(true)
    expect(screen.getByTestId('est-hero')).toContainElement(time)
    expect(screen.getByTestId('est-hero')).toContainElement(sessions)
  })

  it('ETN-004/cabeçalho: "Sua biblioteca", rótulo da ordem, selos LB antes e RB depois das abas, em painel', () => {
    setup()
    const header = screen.getByTestId('est-header')
    expect(inPanel(within(header).getByText('Sua biblioteca'))).toBe(true)
    expect(screen.getByTestId('est-order')).toHaveTextContent('Jogado recentemente')
    const lb = screen.getByTestId('est-chip-lb')
    const rb = screen.getByTestId('est-chip-rb')
    expect(lb).toHaveTextContent(/^LB$/)
    expect(rb).toHaveTextContent(/^RB$/)
    for (const c of [lb, rb]) {
      expect(look(c)).toMatch(/bb-chip/)
      expect(look(c)).toMatch(/bb-on-chip/)
    }
    const tabs = screen.getAllByRole('tab')
    expect(after(lb, tabs[0])).toBe(true)
    expect(after(tabs[tabs.length - 1], rb)).toBe(true)
  })

  it('ETN-004/BBX-054/abas Todos, Favoritos, Concluídos e a plataforma; a ativa em accent com o total entre parênteses', () => {
    const { again } = setup()
    let tabs = screen.getAllByRole('tab')
    expect(tabs.map((t) => t.textContent)).toEqual(['Todos (23)', 'Favoritos', 'Concluídos', 'Nintendo 64'])
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true')
    expect(look(tabs[0])).toMatch(/bb-accent/)
    expect(look(tabs[0])).toMatch(/bb-on-accent/)
    for (const i of [1, 2, 3]) {
      expect(tabs[i]).toHaveAttribute('aria-selected', 'false')
      expect(look(tabs[i])).not.toMatch(/bb-accent/)
    }
    again(model({ tab: 3, tabTotal: 7 }))
    tabs = screen.getAllByRole('tab')
    expect(tabs.map((t) => t.textContent)).toEqual(['Todos', 'Favoritos', 'Concluídos', 'Nintendo 64 (7)'])
    again(model({ tabTotal: null }))
    expect(screen.getAllByRole('tab')[0].textContent).toBe('Todos')
  })

  it('ETN-005/5 colunas; cartão com capa 2:3 contain, título em uma linha e "Jogou por <tempo>" ou "Nunca jogado"', () => {
    setup()
    const grid = screen.getByTestId('est-grid')
    expect(grid.getAttribute('data-columns') === '5' || /grid-cols-5|repeat\(\s*5/.test(look(grid))).toBe(true)
    const covers = screen.getAllByTestId('bb-cover')
    expect(look(covers[0])).toMatch(/aspect-\[2\/3\]|aspect-ratio:\s*2\s*\/\s*3/)
    const img = covers[0].querySelector('img')!
    expect(img.className).toContain('object-contain')
    expect(img.className).not.toContain('object-cover')
    const cards = screen.getAllByTestId('est-card')
    expect(cards).toHaveLength(10)
    const title = within(cards[0]).getByTestId('est-card-title')
    expect(title).toHaveTextContent('Jogo 1')
    expect(look(title)).toMatch(/truncate|text-ellipsis/)
    expect(inPanel(title)).toBe(true)
    expect(within(cards[0]).getByTestId('est-card-time')).toHaveTextContent('Jogou por 28 h 10 min')
    expect(within(cards[1]).getByTestId('est-card-time')).toHaveTextContent('Nunca jogado')
    expect(inPanel(within(cards[0]).getByTestId('est-card-time'))).toBe(true)
    expect(look(within(cards[0]).getByTestId('est-card-time'))).toMatch(/bb-text-muted/)
  })

  it('ETN-005/só o cartão em foco tem o contorno --arc-bb-focus de 3 px e a escala 1,05', () => {
    setup()
    const covers = screen.getAllByTestId('bb-cover')
    const marked = covers.filter((c) => c.getAttribute('data-focused') === 'true')
    expect(marked).toHaveLength(1)
    expect(marked[0]).toBe(covers[2])
    expect(marked[0].getAttribute('aria-label')).toBe('Jogo 3')
    expect(look(marked[0])).toMatch(/bb-focus/)
    expect(look(marked[0])).toMatch(/3px/)
    expect(look(marked[0])).toMatch(/1\.05/)
    for (const c of covers.filter((x) => x !== marked[0])) expect(look(c)).not.toMatch(/1\.05/)
  })

  it('ETN-005/capa ausente mostra o título no cartão placeholder; clique foca (índice absoluto) e duplo clique lança', () => {
    const games = Array.from({ length: 10 }, (_, i) => game(i + 6, i === 1 ? { coverUrl: null } : {}))
    const { onPick, onPlay } = setup(model({ games, focus: 5, stripStart: 5, focused: focused(games[0]) }))
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers[1].querySelector('img')).toBeNull()
    expect(within(covers[1]).getByText('Jogo 7')).toBeInTheDocument()
    expect(look(covers[1])).toMatch(/bb-placeholder/)
    fireEvent.click(covers[3])
    expect(onPick).toHaveBeenCalledWith(8)
    fireEvent.doubleClick(covers[3])
    expect(onPlay).toHaveBeenCalledWith(8)
  })

  it('ETN-007/destaque presente com r = 0 (foco 0..4) e ausente com r = 1 (foco 5); o cabeçalho e as abas sempre presentes', () => {
    const { again } = setup(model({ focus: 4, focused: focused(game(5)) }))
    expect(screen.getByTestId('est-hero')).toBeInTheDocument()
    again(model({ focus: 5, stripStart: 5, focused: focused(game(1)) }))
    expect(screen.queryByTestId('est-hero')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Iniciar jogo' })).toBeNull()
    expect(screen.getByTestId('est-header')).toBeInTheDocument()
    expect(within(screen.getByTestId('est-header')).getByText('Sua biblioteca')).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(4)
    expect(screen.getByTestId('est-grid')).toBeInTheDocument()
    again(model({ focus: 0, focused: focused(game(1)) }))
    expect(screen.getByTestId('est-hero')).toBeInTheDocument()
  })

  it('ETN-007/a troca é animada em até 300 ms (o encaixe do destaque tem transição de 300 ms)', () => {
    const { again } = setup()
    const slot = screen.getByTestId('est-hero-slot')
    expect(look(slot)).toMatch(/300ms|duration-300/)
    again(model({ focus: 5, stripStart: 5, focused: focused(game(1)) }))
    expect(screen.getByTestId('est-hero-slot')).toBeInTheDocument()
    expect(look(screen.getByTestId('est-hero-slot'))).toMatch(/300ms|duration-300/)
  })

  it('ETN-007/foco na fileira 1: a grade começa pela fileira do foco (primeiro cartão = stripStart)', () => {
    const games = Array.from({ length: 10 }, (_, i) => game(i + 6))
    setup(model({ games, focus: 6, stripStart: 5, focused: focused(games[1]) }))
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers[0].getAttribute('aria-label')).toBe('Jogo 6')
    expect(covers[1]).toHaveAttribute('data-focused', 'true')
  })

  it('ETN-009/"Carregando jogos…" fica num painel, com cabeçalho e abas visíveis e sem destaque nem grade', () => {
    setup(model({ ...EMPTY, loading: true, tabTotal: null }))
    expect(inPanel(screen.getByText('Carregando jogos…'))).toBe(true)
    expect(within(screen.getByTestId('est-header')).getByText('Sua biblioteca')).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(4)
    expect(screen.queryByTestId('est-hero')).toBeNull()
    expect(screen.queryAllByTestId('bb-cover')).toHaveLength(0)
  })

  it('ETN-009/BBX-054/aba vazia mostra "Nenhum jogo nesta aba" num painel, com cabeçalho e abas', () => {
    for (const tab of [1, 2, 3]) {
      const { unmount } = setup(model({ ...EMPTY, tab }))
      expect(inPanel(screen.getByText('Nenhum jogo nesta aba')), `aba ${tab}`).toBe(true)
      expect(screen.getAllByRole('tab')).toHaveLength(4)
      expect(within(screen.getByTestId('est-header')).getByText('Sua biblioteca')).toBeInTheDocument()
      expect(screen.queryByTestId('est-hero')).toBeNull()
      unmount()
    }
  })

  it('ETN-009/o aviso de erro tem role alert e some quando model.alert é null', () => {
    const { again } = setup(model({ alert: 'Já há um jogo em execução.' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Já há um jogo em execução.')
    again(model({ alert: null }))
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('ETN-009/em Detalhes o tema mostra o elemento details e não a grade nem o destaque', () => {
    setup(model({ screen: 'details' }))
    expect(screen.getByText('DETALHES')).toBeInTheDocument()
    expect(screen.queryByTestId('bb-cover')).toBeNull()
    expect(screen.queryByTestId('est-hero')).toBeNull()
  })

  it('ETN-013/nenhum destes textos ou controles no DOM de Navegar: logotipo, Gerenciar, favoritar, busca, ordem, Todos os jogos, lista/grade, Modernos, Clássicos, conquistas, avaliação, downloads, jogando agora, requisitos, similares, Co-Op, Vs e menu lateral', () => {
    const { container } = setup()
    const text = container.textContent ?? ''
    for (const re of [
      /Gerenciar/i, /Todos os jogos/i, /Modernos/i, /Clássicos/i, /Conquistas/i, /Avalia[çc][ãa]o/i, /Downloads?/i, /jogando agora/i,
      /Requisitos/i, /Ver similares/i, /Co-?Op/i, /\bVs\b/, /Buscar|Pesquisar/i, /Ordenar/i, /Lista|Grade/,
    ]) {
      expect(text, String(re)).not.toMatch(re)
    }
    expect(container.querySelector('input, select, textarea, [role="searchbox"], [role="combobox"], nav, aside, [role="navigation"]')).toBeNull()
    expect(container.querySelector('img[alt*="logo" i]')).toBeNull()
    expect(screen.queryByRole('button', { name: /favorit|gerenciar|lista|grade/i })).toBeNull()
  })

  it('BBX-058/só tokens --arc-bb-* e nenhuma cor literal nos arquivos do tema; nada de wailsjs nem de escrita do tempo para zerar', () => {
    const entries = Object.entries(sources)
    expect(entries.length).toBeGreaterThanOrEqual(5)
    for (const [f, src] of entries) {
      expect(src, f).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
      expect(src, f).not.toMatch(/\brgba?\(|\bhsla?\(/)
      expect(src, f).not.toMatch(/\b(?:bg|text|border|ring|outline)-(?:slate|gray|zinc|red|blue|green|white|black)\b/)
      expect(src, f).not.toMatch(/wailsjs/)
      expect(src, f).not.toMatch(/TimeToBeat(?:Refresh|Search|Choose|SetSettings)/)
    }
  })
})

// ---------------------------------------------------------------------------------------------------------
// Shell with the Estante: fake bindings, fake gamepad (BBX-048..BBX-055, ETN-001, ETN-006, TTB-016, ETN-011).
// ---------------------------------------------------------------------------------------------------------
type Fn = ReturnType<typeof vi.fn>
const originalThemes = BIGBOX_THEMES.map((t) => ({ ...t }))
let app: Record<string, Fn>
const PAD = { A: 0, B: 1, X: 2, Y: 3, LB: 4, RB: 5, UP: 12, DOWN: 13, LEFT: 14, RIGHT: 15 }
let padPressed: Set<number>
let rafQueue: Map<number, FrameRequestCallback>
let rafSpies: Array<{ mockRestore(): void }> = []
const realViewport = { w: window.innerWidth, h: window.innerHeight }

const row = (id: number) => ({
  id, platformId: 1, title: `Jogo ${String(id).padStart(3, '0')}`, platform: 'Windows', path: `C:\\g\\${id}.exe`, status: 'not_played',
  favorite: false, year: null, totalPlaySeconds: 0, playCount: 0, firstPlayedAt: '', lastPlayedAt: '',
})
const ROWS = Array.from({ length: 12 }, (_, i) => row(i + 1))

function installShell() {
  app = {
    ListGames: vi.fn(({ offset, limit }: { offset: number; limit: number }) =>
      Promise.resolve({ games: ROWS.slice(offset, offset + limit), total: ROWS.length, offset })),
    GetGame: vi.fn((id: number) =>
      Promise.resolve({ ...ROWS[id - 1], description: 'Descrição.', completedAt: null, developers: [], publishers: [], genres: [], playModes: [], launchType: 'exe', launchArgs: '', sessionPlaySeconds: 0 /* alterado em 2026-11-25 (fase 16) */, emulatorId: null })),
    Catalog: vi.fn(() =>
      Promise.resolve({ platforms: [{ id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 12 }], genres: [], companies: [], playModes: [] })),
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    GameSessions: vi.fn(() => Promise.resolve({ sessions: [], total: 0, offset: 0 })),
    LaunchGame: vi.fn((id: number) => Promise.resolve({ sessionId: 1, gameId: id, title: `Jogo ${id}`, state: 'running', startedAt: '', endedAt: '', activeSeconds: 0, activeMs: 0, asOf: '' })),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    BringToFront: vi.fn(() => Promise.resolve()),
    TimeToBeatGet: vi.fn(() => Promise.resolve(null)),
  }
  const proxy = new Proxy(app, {
    get(target, name: string) {
      if (typeof name !== 'string') return undefined
      return (...args: unknown[]) => {
        const fn = target[name] as ((...a: unknown[]) => unknown) | undefined
        return fn ? fn(...args) : Promise.resolve(undefined)
      }
    },
  })
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: proxy } }
  w.runtime = {
    EventsOn: vi.fn(() => () => {}),
    WindowIsFullscreen: vi.fn(() => Promise.resolve(false)),
    WindowFullscreen: vi.fn(),
    WindowUnfullscreen: vi.fn(),
  }
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: 1920 })
  Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: 1080 })
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(() => Promise.resolve())
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  padPressed = new Set()
  rafQueue = new Map()
  let id = 0
  Object.defineProperty(navigator, 'getGamepads', {
    configurable: true,
    value: () => [{
      connected: true, mapping: 'standard', axes: [0, 0, 0, 0],
      buttons: Array.from({ length: 17 }, (_, i) => ({ pressed: padPressed.has(i), touched: padPressed.has(i), value: padPressed.has(i) ? 1 : 0 })),
    }],
  })
  rafSpies = [
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => { id += 1; rafQueue.set(id, cb); return id }),
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((n: number) => { rafQueue.delete(n) }),
  ]
}
function uninstallShell() {
  rafSpies.forEach((s) => s.mockRestore())
  rafSpies = []
  Object.defineProperty(navigator, 'getGamepads', { configurable: true, value: undefined })
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: realViewport.w })
  Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: realViewport.h })
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  BIGBOX_THEMES.splice(0, BIGBOX_THEMES.length, ...originalThemes.map((t) => ({ ...t })))
}
function frame() {
  const q = [...rafQueue.values()]
  rafQueue.clear()
  act(() => { for (const cb of q) cb(performance.now()) })
}
function tap(button: number) {
  padPressed.add(button)
  frame()
  padPressed.delete(button)
  frame()
}
function setupShell() {
  const onExit = vi.fn()
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const Events = ({ children }: { children: ReactNode }) => {
    useMediaEvents()
    useTimeToBeatEvents()
    return <>{children}</>
  }
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <Events>{children}</Events>
    </QueryClientProvider>
  )
  const r = render(<BigBox initialPlatformId={null} initialGameId={null} themeId="estante" onExit={onExit} />, { wrapper })
  return { onExit, ...r }
}
async function readyShell() {
  await waitFor(() => expect(screen.queryAllByTestId('bb-cover').length).toBeGreaterThanOrEqual(5))
  await waitFor(() => expect(screen.getByTestId('est-title')).toHaveTextContent(/./))
  frame() // primes the edge detector of the gamepad
}
const focusedTitle = () => screen.getAllByTestId('bb-cover').find((c) => c.getAttribute('data-focused') === 'true')?.getAttribute('aria-label')
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 20)) })
const shot = (id: number) => ({ id, kind: 'screenshot', provider: 'x', path: `p${id}`, thumbPath: `t${id}`, url: `/m/s${id}.jpg`, thumbUrl: `/m/s${id}-t.jpg` })
const VIDEO = { id: 9, kind: 'video', provider: 'x', path: 'v', thumbPath: '', url: '/m/v.mp4', thumbUrl: '' }
const BACKGROUND = { id: 8, kind: 'background', provider: 'x', path: 'b', thumbPath: '', url: '/m/bg.jpg', thumbUrl: '' }

describe('Estante no shell do Big Box', () => {
  beforeEach(installShell)
  afterEach(uninstallShell)

  it('ETN-001/BBX-052/o shell monta o tema estante com fundo cover (arte em tela cheia com degradê, sem arte desfocada)', async () => {
    app.MediaList.mockResolvedValue({ media: [BACKGROUND] })
    const { container } = setupShell()
    await readyShell()
    expect(container.querySelector('[data-bigbox-theme]')).toHaveAttribute('data-bigbox-theme', 'estante')
    const art = await screen.findByTestId('bb-art')
    expect(art.className).toContain('object-cover')
    expect(art.className).not.toContain('object-contain')
    expect(screen.getByTestId('bb-cover-gradient')).toBeInTheDocument()
    expect(screen.queryByTestId('bb-art-fill')).toBeNull()
  })

  it('ETN-001/BBX-053/o vídeo de fundo toca no fundo (um só, fora dos cartões) e Y liga e desliga o som', async () => {
    app.MediaList.mockResolvedValue({ media: [BACKGROUND, VIDEO] })
    const { container } = setupShell()
    await readyShell()
    const video = (await screen.findByTestId('bb-video', undefined, { timeout: 5000 })) as HTMLVideoElement
    expect(container.querySelectorAll('video')).toHaveLength(1)
    expect(video.closest('[data-testid="bb-cover"]')).toBeNull()
    expect(video.muted).toBe(false)
    tap(PAD.Y)
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(true))
    tap(PAD.Y)
    await waitFor(() => expect((screen.getByTestId('bb-video') as HTMLVideoElement).muted).toBe(false))
  }, 15000)

  it('ETN-006/↓ da fileira 0 vai à 1, → no fim da fileira passa à seguinte, ↑ volta; o destaque some na fileira 1 e volta na 0', async () => {
    setupShell()
    await readyShell()
    expect(focusedTitle()).toBe('Jogo 001')
    expect(screen.getByTestId('est-hero')).toBeInTheDocument()
    tap(PAD.DOWN)
    expect(focusedTitle()).toBe('Jogo 006')
    expect(screen.queryByTestId('est-hero')).toBeNull()
    expect(screen.getByTestId('est-header')).toBeInTheDocument()
    tap(PAD.UP)
    expect(focusedTitle()).toBe('Jogo 001')
    expect(screen.getByTestId('est-hero')).toBeInTheDocument()
    for (let i = 0; i < 4; i += 1) tap(PAD.RIGHT)
    expect(focusedTitle()).toBe('Jogo 005')
    tap(PAD.RIGHT)
    expect(focusedTitle()).toBe('Jogo 006')
    expect(screen.queryByTestId('est-hero')).toBeNull()
    tap(PAD.LEFT)
    expect(focusedTitle()).toBe('Jogo 005')
    expect(screen.getByTestId('est-hero')).toBeInTheDocument()
  })

  it('ETN-006/↓ da última fileira parcial vai ao último jogo e depois não faz nada', async () => {
    setupShell()
    await readyShell()
    tap(PAD.DOWN)
    tap(PAD.DOWN)
    expect(focusedTitle()).toBe('Jogo 011')
    tap(PAD.DOWN)
    expect(focusedTitle()).toBe('Jogo 011')
    tap(PAD.RIGHT)
    tap(PAD.RIGHT)
    expect(focusedTitle()).toBe('Jogo 012')
  })

  it('ETN-006/A lança o jogo em foco', async () => {
    setupShell()
    await readyShell()
    tap(PAD.RIGHT)
    tap(PAD.A)
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(2))
    await flush()
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
  })

  it('ETN-005/clicar no cartão foca e duplo clique lança; o botão "Iniciar jogo" do destaque lança o jogo em foco', async () => {
    setupShell()
    await readyShell()
    fireEvent.click(screen.getAllByTestId('bb-cover')[3])
    expect(focusedTitle()).toBe('Jogo 004')
    fireEvent.click(screen.getByRole('button', { name: 'Iniciar jogo' }))
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(4))
    expect(app.LaunchGame).toHaveBeenCalledTimes(1)
  })

  it('ETN-005/duplo clique no cartão lança esse jogo', async () => {
    setupShell()
    await readyShell()
    fireEvent.doubleClick(screen.getAllByTestId('bb-cover')[2])
    await waitFor(() => expect(app.LaunchGame).toHaveBeenCalledWith(3))
  })

  it('ETN-006/X abre os Detalhes da Estante e B volta; B em Navegar sai', async () => {
    const { onExit } = setupShell()
    await readyShell()
    tap(PAD.X)
    expect(await screen.findByRole('button', { name: 'Jogar sem contar tempo' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Iniciar jogo' })).toHaveAttribute('data-focused', 'true')
    expect(screen.queryByTestId('est-grid')).toBeNull()
    tap(PAD.B)
    expect(screen.queryByRole('button', { name: 'Jogar sem contar tempo' })).toBeNull()
    expect(onExit).not.toHaveBeenCalled()
    tap(PAD.B)
    expect(onExit).toHaveBeenCalledTimes(1)
  })

  it('ETN-006/o botão "Detalhes" do destaque abre os Detalhes (age como X)', async () => {
    setupShell()
    await readyShell()
    fireEvent.click(screen.getByRole('button', { name: 'Detalhes' }))
    expect(await screen.findByRole('button', { name: 'Jogar sem contar tempo' })).toBeInTheDocument()
  })

  it('BBX-054/LB/RB trocam entre Todos, Favoritos, Concluídos e as plataformas; o rodapé diz "Aba"; "Favoritos" lê só favoritos', async () => {
    setupShell()
    await readyShell()
    expect(screen.getAllByRole('tab').map((t) => t.textContent?.replace(/\s*\(\d+\)$/, ''))).toEqual(['Todos', 'Favoritos', 'Concluídos', 'Windows'])
    expect(screen.getByText('Aba')).toBeInTheDocument()
    expect(screen.queryByText('Plataforma')).toBeNull()
    await waitFor(() => expect(screen.getAllByRole('tab')[0].textContent).toBe('Todos (12)'))
    tap(PAD.RB)
    await waitFor(() => expect(screen.getAllByRole('tab')[1]).toHaveAttribute('aria-selected', 'true'))
    await waitFor(() => expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ filter: { version: 1, favorite: true } })))
    tap(PAD.RB)
    await waitFor(() => expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ filter: { version: 1, statuses: ['completed'] } })))
    tap(PAD.LB)
    await waitFor(() => expect(screen.getAllByRole('tab')[1]).toHaveAttribute('aria-selected', 'true'))
  })

  it('BBX-055/o shell lê a lista por último jogado e o cabeçalho mostra "Jogado recentemente"', async () => {
    setupShell()
    await readyShell()
    expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ sort: 'lastPlayed' }))
    expect(screen.getByTestId('est-order')).toHaveTextContent('Jogado recentemente')
  })

  it('BBX-050/a primeira página da grade tem (R + 1 + 3) × 5 jogos (1920×1080: R = 1, 25)', async () => {
    setupShell()
    await readyShell()
    expect(app.ListGames).toHaveBeenCalledWith(expect.objectContaining({ offset: 0, limit: 25 }))
  })

  it('TTB-016/os Detalhes da Estante mostram o tempo para zerar e pedem fetch true uma vez ao abrir', async () => {
    app.TimeToBeatGet.mockImplementation((id: number) => Promise.resolve({
      gameId: id, state: 'found', source: 'hltb', externalId: '1', externalTitle: 'Jogo', mainSeconds: 72000, mainExtraSeconds: 113400,
      completionistSeconds: 147600, fetchedAt: '2026-11-20T10:00:00Z', confirmed: false, stale: false, pending: false, unavailable: false, candidates: [],
    }))
    setupShell()
    await readyShell()
    const fetchTrue = () => (app.TimeToBeatGet.mock.calls as unknown as Array<[number, boolean]>).filter((c) => c[1] === true)
    expect(fetchTrue()).toHaveLength(0)
    tap(PAD.X)
    const panel = (await screen.findByText('Tempo para zerar')).closest('[data-bb-panel]')
    expect(panel).not.toBeNull()
    for (const s of ['História principal', '20 h', 'História + extras', '31½ h', '100%', '41 h']) {
      expect(await within(panel as HTMLElement).findByText(s)).toBeInTheDocument()
    }
    expect(await within(panel as HTMLElement).findByText('Fonte: HowLongToBeat')).toBeInTheDocument()
    await waitFor(() => expect(fetchTrue()).toHaveLength(1))
    expect(fetchTrue()[0]).toEqual([1, true])
    await flush()
    expect(fetchTrue()).toHaveLength(1)
    expect(screen.queryByRole('button', { name: /atualizar|confirmar|escolher|buscar/i })).toBeNull()
  })

  it('ETN-010/os Detalhes ficam sobre o fundo cover do jogo', async () => {
    app.MediaList.mockResolvedValue({ media: [BACKGROUND] })
    setupShell()
    await readyShell()
    await screen.findByTestId('bb-art')
    tap(PAD.X)
    await screen.findByRole('button', { name: 'Jogar sem contar tempo' })
    expect(screen.getByTestId('bb-art').className).toContain('object-cover')
    expect(screen.getByTestId('bb-cover-gradient')).toBeInTheDocument()
  })

  it('ETN-011/sem galeria o carrossel não aparece e ↓ não faz nada (o foco fica em "Iniciar jogo")', async () => {
    setupShell()
    await readyShell()
    tap(PAD.X)
    await screen.findByRole('button', { name: 'Jogar sem contar tempo' })
    expect(screen.queryByTestId('est-d-viewer')).toBeNull()
    expect(screen.queryAllByTestId('est-d-dot')).toHaveLength(0)
    tap(PAD.DOWN)
    expect(screen.getByRole('button', { name: 'Iniciar jogo' })).toHaveAttribute('data-focused', 'true')
  })

  it('ETN-011/com galeria ↓ leva o foco ao visor (única marca), ←/→ trocam o item e as bolinhas acompanham, ↑ volta ao botão', async () => {
    app.MediaList.mockResolvedValue({ media: [shot(1), shot(2)] })
    const { container } = setupShell()
    await readyShell()
    tap(PAD.X)
    await screen.findByRole('button', { name: 'Jogar sem contar tempo' })
    await waitFor(() => expect(screen.getAllByTestId('est-d-dot')).toHaveLength(2))
    tap(PAD.DOWN)
    const viewer = () => screen.getByTestId('est-d-viewer')
    expect(viewer()).toHaveAttribute('data-focused', 'true')
    expect(Array.from(container.querySelectorAll('[data-focused="true"]'))).toEqual([viewer()])
    expect(viewer().querySelector('img')?.getAttribute('src')).toBe('/m/s1.jpg')
    tap(PAD.RIGHT)
    expect(viewer().querySelector('img')?.getAttribute('src')).toBe('/m/s2.jpg')
    expect(screen.getAllByTestId('est-d-dot').map((d) => d.getAttribute('aria-current'))).toEqual([null, 'true'])
    tap(PAD.UP)
    expect(screen.getByRole('button', { name: 'Iniciar jogo' })).toHaveAttribute('data-focused', 'true')
    expect(screen.getByTestId('est-d-viewer')).not.toHaveAttribute('data-focused', 'true')
  })
})
