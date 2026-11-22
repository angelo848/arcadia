import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { hintsFor } from '../../hints'
import type { BigBoxFocused, BigBoxGame, BigBoxViewModel } from '../../viewModel'
import { BIGBOX_THEMES } from '../registry'
import { Mosaico } from './Mosaico'

const sources = import.meta.glob<string>(['./*.tsx', './capacity.ts', '!./*.test.tsx'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

// Assumed markup (T34 must follow): tabs role="tablist"/"tab" (aria-selected); count data-testid="bb-count";
// title data-testid="mos-title" (single line, class truncate/ellipsis); covers data-testid="bb-cover" + data-focused;
// panel data-testid="mos-panel" with data-bb-panel and columns data-testid="mos-col-info" / "mos-col-times" / "mos-col-shots";
// description data-testid="mos-description" (line-clamp-4); screenshots are <img> inside mos-col-shots.
// The footer, the theme root attribute (data-bigbox-theme) and the time-to-beat fetch timer belong to the shell.
const game = (id: number, o: Partial<BigBoxGame> = {}): BigBoxGame => ({
  id, title: `Jogo ${id}`, coverUrl: `http://m/${id}.jpg`, session: null, ...o,
})
const focused = (g: BigBoxGame, o: Partial<BigBoxFocused> = {}): BigBoxFocused => ({
  game: g, platform: 'Nintendo 64', year: 2000, genre: 'Ação', shortDescription: 'Uma aventura curta.',
  playTimeLabel: '12 h 5 min', lastPlayedLabel: '12/10/2026', sessionsLabel: 'Sessões: 14',
  totalPlaySeconds: 43500, lastPlayedAt: '2026-10-12T10:00:00Z', playCount: 14, backgroundUrl: null, videoUrl: null,
  favorite: false, lastPlayedRelative: 'há 12 dias', screenshots: [],
  timeToBeat: [
    { label: 'História principal', value: '20 h' }, { label: 'História + extras', value: '31½ h' }, { label: '100%', value: '41 h' },
  ],
  timeToBeatSource: 'HowLongToBeat', ...o,
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
    tab: 1, loading: false, games, total: 120, focus: 7, stripStart: 0, counter: '8 / 120', focused: focused(games[7]),
    axis: 'horizontal', columns: 1, tabTotal: 120, sortLabel: 'Título', videoFailed: false,
    videoVisible: false, sound: false,
    hints: hintsFor('browse', { empty: false, videoVisible: false }), alert: null, ...o,
  }
}
const inPanel = (el: HTMLElement) => el.closest('[data-bb-panel]') !== null
function setup(m = model()) {
  const cb = { onPick: vi.fn(), onPlay: vi.fn(), onDetails: vi.fn(), onVideoError: vi.fn() }
  const r = render(<Mosaico model={m} details={<div>DETALHES</div>} {...cb} />)
  return { ...cb, ...r }
}
const after = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
const look = (el: Element) => `${el.className} ${el.getAttribute('style') ?? ''}`

describe('Mosaico', () => {
  beforeEach(() => {
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(() => Promise.resolve())
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('MOS-001/de cima para baixo: abas, título, fileira de capas e painel; sem bb-art, sem menu lateral', () => {
    const { container } = setup()
    const tabs = screen.getByRole('tablist')
    const title = screen.getByTestId('mos-title')
    const first = screen.getAllByTestId('bb-cover')[0]
    const panel = screen.getByTestId('mos-panel')
    expect(after(tabs, title)).toBe(true)
    expect(after(title, first)).toBe(true)
    expect(after(first, panel)).toBe(true)
    expect(container.querySelector('[data-testid="bb-art"], [data-testid="bb-art-layer"], [data-testid="bb-art-fill"]')).toBeNull()
    expect(container.querySelector('nav, aside')).toBeNull()
  })

  it('MOS-002/pílulas: surface com text-muted; a ativa com accent e on-accent; só texto', () => {
    setup()
    const tabs = screen.getAllByRole('tab')
    expect(tabs.map((t) => t.textContent)).toEqual(['Todos', 'Nintendo 64', 'Super Nintendo'])
    expect(tabs[1]).toHaveAttribute('aria-selected', 'true')
    expect(look(tabs[1])).toMatch(/bb-accent/)
    expect(look(tabs[1])).toMatch(/bb-on-accent/)
    for (const i of [0, 2]) {
      expect(tabs[i]).toHaveAttribute('aria-selected', 'false')
      expect(look(tabs[i])).toMatch(/bb-surface/)
      expect(look(tabs[i])).toMatch(/bb-text-muted/)
      expect(look(tabs[i])).not.toMatch(/bb-accent/)
    }
  })

  it('MOS-003/"120 jogos", "1 jogo" com T = 1; nada em "Carregando jogos…" nem em aba vazia', () => {
    const a = setup()
    expect(screen.getByTestId('bb-count')).toHaveTextContent('120 jogos')
    a.unmount()
    const g = game(1)
    const b = setup(model({ games: [g], total: 1, focus: 0, focused: focused(g) }))
    expect(screen.getByTestId('bb-count')).toHaveTextContent(/^1 jogo$/)
    b.unmount()
    const c = setup(model({ loading: true, games: [], total: 0, focus: 0, focused: null, counter: null }))
    expect(screen.queryByTestId('bb-count')).toBeNull()
    c.unmount()
    setup(model({ games: [], total: 0, focus: 0, focused: null, counter: null }))
    expect(screen.queryByTestId('bb-count')).toBeNull()
  })

  it('MOS-004/cartões com a imagem inteira (contain) e só o em foco marcado, com escala 1,09 e contorno de 5 px afastado 4 px', () => {
    setup()
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers.length).toBeLessThanOrEqual(15)
    expect(covers.filter((c) => c.getAttribute('data-focused') === 'true')).toHaveLength(1)
    const f = covers.find((c) => c.getAttribute('data-focused') === 'true')!
    expect(f.getAttribute('aria-label') ?? f.textContent).toContain('Jogo 8')
    expect(look(f)).toMatch(/1\.09/)
    expect(look(f)).toMatch(/5px/)
    expect(look(f)).toMatch(/offset-4|offset-\[4px\]|outline-offset:\s*4px/)
    expect(look(f)).toMatch(/bb-focus/)
    expect(look(covers.find((c) => c.getAttribute('data-focused') === 'false')!)).not.toMatch(/1\.09/)
    const img = covers[0].querySelector('img')!
    expect(img.className).toContain('object-contain')
    expect(img.className).not.toContain('object-cover')
  })

  it('MOS-004/janela com âncora 1: stripStart 18 e foco 19 deixa o em foco no índice 1 do DOM; clique foca, duplo clique lança', () => {
    const games = Array.from({ length: 6 }, (_, i) => game(i + 19))
    const { onPick, onPlay } = setup(model({ games, focus: 19, stripStart: 18, total: 40, focused: focused(games[1]) }))
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers[1]).toHaveAttribute('data-focused', 'true')
    fireEvent.click(covers[3])
    expect(onPick).toHaveBeenCalledWith(21)
    fireEvent.doubleClick(covers[3])
    expect(onPlay).toHaveBeenCalledWith(21)
  })

  it('MOS-004/nunca mais de 15 cartões no DOM', () => {
    setup(model({ games: Array.from({ length: 15 }, (_, i) => game(i + 1)), total: 500 }))
    expect(screen.getAllByTestId('bb-cover').length).toBeLessThanOrEqual(15)
  })

  it('MOS-004/o tema segue o registro: âncora 1, fundo liso, vídeo na capa, TTB em Navegar, disponível', () => {
    const def = BIGBOX_THEMES.find((t) => t.id === 'mosaico')!
    expect(def).toMatchObject({
      available: true, axis: 'horizontal', anchor: 1, backdrop: 'plain', videoSlot: 'cover', tabs: 'platforms', sort: 'title', timeToBeatInBrowse: true,
    })
    expect(def.Component).toBe(Mosaico)
    expect(def.listCapacity({ width: 1920, height: 1080 })).toBe(4)
  })

  it('MOS-005/sem capa mostra o título no cartão placeholder; com imagem não mostra texto', () => {
    const games = [game(1), game(2, { coverUrl: null }), game(3)]
    setup(model({ games, focus: 0, total: 3, focused: focused(games[0]) }))
    const covers = screen.getAllByTestId('bb-cover')
    expect(covers[0].querySelector('img')?.getAttribute('src')).toBe('http://m/1.jpg')
    expect(covers[1].querySelector('img')).toBeNull()
    expect(within(covers[1]).getByText('Jogo 2')).toBeInTheDocument()
    expect(look(covers[1])).toMatch(/bb-placeholder/)
  })

  it('MOS-006/o título do jogo em foco fica acima da fileira, em uma linha, com heading, e muda a cada foco', () => {
    const games = [game(1), game(2), game(3)]
    const a = setup(model({ games, focus: 0, total: 3, focused: focused(games[0]) }))
    const t = screen.getByTestId('mos-title')
    expect(t).toHaveTextContent('Jogo 1')
    expect(look(t)).toMatch(/bb-heading/)
    expect(look(t)).toMatch(/truncate|ellipsis/)
    expect(look(t)).toMatch(/nowrap|truncate/)
    a.rerender(
      <Mosaico model={model({ games, focus: 1, total: 3, focused: focused(games[1]) })} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} onVideoError={vi.fn()} />,
    )
    expect(screen.getByTestId('mos-title')).toHaveTextContent('Jogo 2')
  })

  it('MOS-006/marcação Em execução ou Pausado ao lado do título e no cartão (BBX-017)', () => {
    const games = [game(1, { session: 'running' }), game(2, { session: 'paused' }), game(3)]
    const a = setup(model({ games, focus: 0, total: 3, focused: focused(games[0]) }))
    expect(screen.getAllByText('Em execução').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('Pausado').length).toBeGreaterThanOrEqual(1)
    a.rerender(
      <Mosaico model={model({ games, focus: 1, total: 3, focused: focused(games[1]) })} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} onVideoError={vi.fn()} />,
    )
    expect(screen.getAllByText('Pausado').length).toBeGreaterThanOrEqual(2)
  })

  it('MOS-007/o vídeo entra na capa em foco depois de 2 s, só um <video>, dentro do cartão em foco', () => {
    vi.useFakeTimers()
    const g = game(8)
    const m = model({ videoVisible: true, focused: focused(g, { videoUrl: 'http://m/8/v.mp4' }) })
    const { container } = setup(m)
    expect(container.querySelectorAll('video')).toHaveLength(0)
    act(() => { vi.advanceTimersByTime(1999) })
    expect(container.querySelectorAll('video')).toHaveLength(0)
    act(() => { vi.advanceTimersByTime(1) })
    const videos = container.querySelectorAll('video')
    expect(videos).toHaveLength(1)
    expect(videos[0].closest('[data-testid="bb-cover"]')).toHaveAttribute('data-focused', 'true')
  })

  it('MOS-007/sem videoVisible ou com vídeo falho nunca toca', () => {
    vi.useFakeTimers()
    const f = focused(game(8), { videoUrl: 'http://m/8/v.mp4' })
    const a = setup(model({ videoVisible: false, focused: f }))
    act(() => { vi.advanceTimersByTime(5000) })
    expect(a.container.querySelectorAll('video')).toHaveLength(0)
    a.unmount()
    const b = setup(model({ videoVisible: true, videoFailed: true, focused: f }))
    act(() => { vi.advanceTimersByTime(5000) })
    expect(b.container.querySelectorAll('video')).toHaveLength(0)
  })

  it('MOS-008/o painel tem três colunas (com screenshots), linha plataforma · ano · gênero e a descrição curta limitada a 4 linhas', () => {
    const g = game(8)
    setup(model({ focused: focused(g, { screenshots: ['http://m/s1.jpg'] }) }))
    const panel = screen.getByTestId('mos-panel')
    expect(inPanel(panel) || panel.hasAttribute('data-bb-panel')).toBe(true)
    const info = within(panel).getByTestId('mos-col-info')
    expect(within(info).getByText('Nintendo 64 · 2000 · Ação')).toBeInTheDocument()
    const d = within(info).getByTestId('mos-description')
    expect(d).toHaveTextContent('Uma aventura curta.')
    expect(look(d)).toMatch(/line-clamp-4|line-clamp:\s*4/)
    expect(within(panel).getByTestId('mos-col-times')).toBeInTheDocument()
    expect(within(panel).getByTestId('mos-col-shots')).toBeInTheDocument()
  })

  it('MOS-008/partes ausentes somem sem deixar separador', () => {
    setup(model({ focused: focused(game(8), { year: null, genre: null, shortDescription: '' }) }))
    // The active tab is also called "Nintendo 64", so the lookup is scoped to the info column.
    expect(within(screen.getByTestId('mos-col-info')).getByText('Nintendo 64')).toBeInTheDocument()
    expect(screen.queryByText(/·\s*·/)).toBeNull()
    expect(screen.queryByText(/Nintendo 64 ·\s*$/)).toBeNull()
  })

  it('MOS-009/segunda coluna: Tempo jogado, Última vez e Sessões em pares rótulo/valor', () => {
    setup()
    const col = screen.getByTestId('mos-col-times')
    expect(within(col).getByText('Tempo jogado')).toBeInTheDocument()
    expect(within(col).getByText('12 h 5 min')).toBeInTheDocument()
    expect(within(col).getByText('Última vez')).toBeInTheDocument()
    expect(within(col).getByText('12/10/2026')).toBeInTheDocument()
    expect(within(col).getByText('Sessões')).toBeInTheDocument()
    expect(within(col).getByText('14')).toBeInTheDocument()
  })

  it('MOS-009/nunca jogado mostra "Nunca jogado" em Última vez', () => {
    setup(model({ focused: focused(game(8), { playTimeLabel: '0 min', lastPlayedLabel: null, lastPlayedAt: null, playCount: 0 }) }))
    const col = screen.getByTestId('mos-col-times')
    expect(within(col).getByText('Última vez')).toBeInTheDocument()
    expect(within(col).getByText('Nunca jogado')).toBeInTheDocument()
    expect(within(col).getByText('0')).toBeInTheDocument()
  })

  it('MOS-010/segunda coluna, abaixo dos dados: os três tempos para zerar e a fonte', () => {
    setup()
    const col = screen.getByTestId('mos-col-times')
    const ttb = col.querySelector('[data-bb-ttb]') as HTMLElement
    expect(ttb).not.toBeNull()
    for (const [l, v] of [['História principal', '20 h'], ['História + extras', '31½ h'], ['100%', '41 h']]) {
      expect(within(ttb).getByText(l)).toBeInTheDocument()
      expect(within(ttb).getByText(v)).toBeInTheDocument()
    }
    expect(after(within(col).getByText('Sessões'), ttb)).toBe(true)
  })

  it('MOS-010/sem dado mostra "—" nos três valores', () => {
    const rows = [
      { label: 'História principal', value: '—' }, { label: 'História + extras', value: '—' }, { label: '100%', value: '—' },
    ]
    setup(model({ focused: focused(game(8), { timeToBeat: rows, timeToBeatSource: null }) }))
    const ttb = screen.getByTestId('mos-col-times').querySelector('[data-bb-ttb]') as HTMLElement
    expect(within(ttb).getAllByText('—')).toHaveLength(3)
  })

  it('MOS-010/o registro pede a leitura do tempo para zerar também em Navegar (a espera de 1 s é do shell)', () => {
    expect(BIGBOX_THEMES.find((t) => t.id === 'mosaico')!.timeToBeatInBrowse).toBe(true)
  })

  it('MOS-011/até 3 miniaturas na ordem de media.list, contain', () => {
    const shots = ['http://m/s1.jpg', 'http://m/s2.jpg', 'http://m/s3.jpg', 'http://m/s4.jpg']
    setup(model({ focused: focused(game(8), { screenshots: shots }) }))
    const imgs = Array.from(screen.getByTestId('mos-col-shots').querySelectorAll('img'))
    expect(imgs.map((i) => i.getAttribute('src'))).toEqual(shots.slice(0, 3))
    imgs.forEach((i) => expect(i.className).toContain('object-contain'))
  })

  it('MOS-011/com menos de 3 só as que existem; sem nenhuma a coluna some e as outras duas ficam', () => {
    const a = setup(model({ focused: focused(game(8), { screenshots: ['http://m/s1.jpg'] }) }))
    expect(screen.getByTestId('mos-col-shots').querySelectorAll('img')).toHaveLength(1)
    a.unmount()
    setup(model({ focused: focused(game(8), { screenshots: [] }) }))
    expect(screen.queryByTestId('mos-col-shots')).toBeNull()
    expect(screen.getByTestId('mos-col-info')).toBeInTheDocument()
    expect(screen.getByTestId('mos-col-times')).toBeInTheDocument()
  })

  it('MOS-013/"Carregando jogos…" e abas vazias ficam em painéis; as abas continuam e não há painel de informações', () => {
    const empty = { games: [], total: 0, focus: 0, focused: null, counter: null }
    const a = setup(model({ ...empty, loading: true }))
    expect(inPanel(screen.getByText('Carregando jogos…'))).toBe(true)
    expect(screen.getAllByRole('tab')).toHaveLength(3)
    expect(screen.queryByTestId('mos-panel')).toBeNull()
    a.unmount()
    const b = setup(model(empty))
    expect(inPanel(screen.getByText('Nenhum jogo nesta plataforma'))).toBe(true)
    b.unmount()
    setup(model({ ...empty, tabs: [{ key: 'all', label: 'Todos', filter: { platformId: null } }], tab: 0 }))
    expect(inPanel(screen.getByText('Nenhum jogo na biblioteca'))).toBe(true)
  })

  it('MOS-013/o aviso de erro aparece com role alert e some quando model.alert é null', () => {
    const a = setup(model({ alert: 'Já há um jogo em execução.' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Já há um jogo em execução.')
    a.rerender(<Mosaico model={model({ alert: null })} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} onVideoError={vi.fn()} />)
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('MOS-014/em Detalhes o tema mostra o elemento details (BigBoxDetails comum, sem Details próprio) e não a fileira', () => {
    setup(model({ screen: 'details' }))
    expect(screen.getByText('DETALHES')).toBeInTheDocument()
    expect(screen.queryByTestId('bb-cover')).toBeNull()
    expect(BIGBOX_THEMES.find((t) => t.id === 'mosaico')!.Details).toBeUndefined()
  })

  it('BBX-051/a janela usa a âncora 1 do registro', () => {
    expect(BIGBOX_THEMES.find((t) => t.id === 'mosaico')!.anchor).toBe(1)
  })

  it('BBX-053/o vídeo fica na capa: o tema declara videoSlot cover', () => {
    expect(BIGBOX_THEMES.find((t) => t.id === 'mosaico')!.videoSlot).toBe('cover')
  })

  it('BBX-058/só tokens --arc-bb-* e nenhuma cor literal nos arquivos do tema; imports só por ../../widgets', () => {
    const entries = Object.entries(sources)
    expect(entries.length).toBeGreaterThanOrEqual(3)
    for (const [f, src] of entries) {
      expect(src, f).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
      expect(src, f).not.toMatch(/\brgba?\(|\bhsla?\(/)
      expect(src, f).not.toMatch(/\b(?:bg|text|border|ring|outline)-(?:slate|gray|zinc|red|blue|green|white|black)\b/)
      expect(src, f).not.toMatch(/from ['"][./]*(?:\.\.\/)+lib\/(?!timeToBeat)/)
      expect(src, f).not.toMatch(/wailsjs/)
    }
  })
})
