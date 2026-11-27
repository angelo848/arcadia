// Assumed markup (T15 must follow) for Desktop [TDK-001, TDK-002, TDK-004, TDK-023, TDK-026, TDK-029]:
//  root <div data-testid="dsk-root"> with a grid whose inline style or class uses var(--col-left) and var(--col-right);
//  children in DOM order: <header> (only the text "Arcadia"), optional error strip role="alert" right after it, then
//  <nav aria-label="Plataformas"> (DesktopSidebar), the center <section aria-label="Jogos"> holding the toolbar (text only) and the
//  <div role="listbox" aria-label="Jogos"> with the cards/rows (role="option", aria-label = title, aria-selected on the focus) and
//  <aside aria-label="Detalhes do jogo"> holding the `details` node; with no `details` the aside says "Nenhum jogo selecionado"
//  (no focused game) or "Carregando jogo…" (focused game without detail read).
//  The Desktop derives the zone: screen "details" is the panel zone (nothing in games/platforms is data-focused);
//  screen "browse" with zone "games" marks the focused option; zone "platforms" marks the active nav item.
//  The footer, the theme root attribute and the art belong to the shell. The theme never draws art or <video> itself.
import type { ReactNode } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { hintsFor } from '../../hints'
import type { BigBoxFocused, BigBoxGame, BigBoxViewModel } from '../../viewModel'
import { Desktop } from './Desktop'

const game = (id: number, o: Partial<BigBoxGame> = {}): BigBoxGame => ({
  id, title: `Jogo ${id}`, coverUrl: `http://m/${id}.jpg`, session: null, coverFit: 'contain', platform: 'Nintendo 64',
  playTimeText: '2 h 10 min', lastPlayedText: '12/10/2026 10:00', ...o,
})
const focused = (g: BigBoxGame): BigBoxFocused => ({
  game: g, platform: 'Nintendo 64', year: 2000, genre: 'Ação', shortDescription: '', playTimeLabel: '2 h 10 min',
  lastPlayedLabel: '12/10/2026', sessionsLabel: 'Sessões: 3', totalPlaySeconds: 7800, lastPlayedAt: '2026-10-12T10:00:00Z',
  playCount: 3, backgroundUrl: null, videoUrl: null, favorite: false, lastPlayedRelative: 'há 12 dias', screenshots: [],
  timeToBeat: [], timeToBeatSource: null,
})
const TABS = [
  { key: 'all', label: 'Todos', filter: { platformId: null }, iconUrl: null, count: 120 },
  { key: 'p:7', label: 'Nintendo 64', filter: { platformId: 7 }, iconUrl: '/i/n64.png', count: 40 },
  { key: 'p:8', label: 'Super Nintendo', filter: { platformId: 8 }, iconUrl: '/i/snes.png', count: 80 },
]
function model(o: Partial<BigBoxViewModel> = {}): BigBoxViewModel {
  const games = Array.from({ length: 12 }, (_, i) => game(i + 1))
  return {
    screen: 'browse', tabs: TABS, tab: 1, loading: false, games, total: 120, focus: 2, stripStart: 0, counter: '3 / 120',
    focused: focused(games[2]), axis: 'grid', columns: 6, tabTotal: 40, sortLabel: 'Título', videoFailed: false,
    videoVisible: false, sound: false, hints: hintsFor('browse', { empty: false, videoVisible: false }, 'Plataforma'),
    alert: null, layout: 'columns', zone: 'games', view: 'grid', ...o,
  }
}
const EMPTY = { games: [], total: 0, focus: 0, stripStart: 0, focused: null, counter: null, tabTotal: 0 }
const look = (el: Element) => `${el.className} ${el.getAttribute('style') ?? ''}`
const after = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
const STUB = <div data-testid="panel-stub" data-focused="true">DETALHES</div>
function setup(m = model(), details: ReactNode = STUB) {
  const cb = { onPick: vi.fn(), onPlay: vi.fn(), onDetails: vi.fn(), onVideoError: vi.fn(), onTab: vi.fn() }
  const r = render(<Desktop model={m} details={details} {...cb} />)
  return { ...cb, ...r }
}
const ACTIONABLE = 'button, select, input, textarea, a[href], [role="button"], [role="combobox"]'

afterEach(() => vi.restoreAllMocks())

describe('Desktop: estrutura', () => {
  it('TDK-001/marcos nesta ordem: header, nav "Plataformas", região "Jogos" e aside "Detalhes do jogo"', () => {
    const { container } = setup()
    const header = container.querySelector('header')!
    const nav = screen.getByRole('navigation', { name: 'Plataformas' })
    const center = container.querySelector('section[aria-label="Jogos"], [role="region"][aria-label="Jogos"]')!
    const aside = container.querySelector('aside[aria-label="Detalhes do jogo"]')!
    for (const el of [header, nav, center, aside]) expect(el).toBeTruthy()
    expect(after(header, nav)).toBe(true)
    expect(after(nav, center)).toBe(true)
    expect(after(center, aside)).toBe(true)
    expect(within(center as HTMLElement).getByRole('listbox', { name: 'Jogos' })).toBeInTheDocument()
  })

  it('TDK-001/a grade usa as variáveis --col-left e --col-right', () => {
    const { container } = setup()
    expect(container.innerHTML).toMatch(/var\(--col-left\)/)
    expect(container.innerHTML).toMatch(/var\(--col-right\)/)
  })

  it('TDK-001/sem bb-art e sem arte de fundo nem vídeo no tema', () => {
    const { container } = setup(model({ videoVisible: true, focused: { ...focused(game(3)), videoUrl: 'http://m/v.mp4' } }))
    expect(container.querySelector('[data-testid="bb-art"], [data-testid="bb-art-layer"], [data-testid="bb-cover-gradient"]')).toBeNull()
    expect(container.querySelectorAll('video')).toHaveLength(0)
  })

  it('TDK-002/o header mostra "Arcadia" e nenhum elemento acionável', () => {
    const { container } = setup()
    const header = container.querySelector('header')!
    expect(header).toHaveTextContent('Arcadia')
    expect(header.querySelector(`${ACTIONABLE}, [role="link"], [role="menuitem"]`)).toBeNull()
  })

  it('TDK-002/nenhum botão nem combobox com os rótulos da barra superior; "Plataformas" é só o texto do título da coluna', () => {
    setup()
    for (const name of ['Big Box', 'Tema do Big Box', 'Importar', 'Provedores', 'Emuladores', 'Plataformas', 'Estatísticas']) {
      expect(screen.queryByRole('button', { name }), name).toBeNull()
      expect(screen.queryByRole('combobox', { name }), name).toBeNull()
    }
    expect(screen.getByRole('navigation', { name: 'Plataformas' })).toHaveTextContent('Plataformas')
  })

  it('TDK-004/barra: "<T> jogos", "Ordenar por: <rótulo>" e "Grade" só como texto', () => {
    const { container } = setup()
    expect(screen.getByText('120 jogos')).toBeInTheDocument()
    expect(screen.getByText('Ordenar por: Título')).toBeInTheDocument()
    expect(screen.getByText('Grade')).toBeInTheDocument()
    expect(screen.getByText('Grade').closest('button, select, [role="button"], [role="combobox"]')).toBeNull()
    expect(container.querySelector('select, input')).toBeNull()
  })

  it('TDK-004/"1 jogo" com T = 1, "Lista" na lista e o rótulo da ordem do modelo', () => {
    setup(model({ total: 1, games: [game(1)], focus: 0, focused: focused(game(1)), view: 'list', columns: 1, sortLabel: 'Última vez jogado' }))
    expect(screen.getByText('1 jogo')).toBeInTheDocument()
    expect(screen.getByText('Lista')).toBeInTheDocument()
    expect(screen.getByText('Ordenar por: Última vez jogado')).toBeInTheDocument()
  })
})

describe('Desktop: estados', () => {
  it('TDK-026/carregando: "Carregando jogos…" no centro e "Nenhum jogo selecionado" no painel', () => {
    const { container } = setup(model({ ...EMPTY, loading: true, tabTotal: null }), null)
    expect(screen.getByText('Carregando jogos…')).toBeInTheDocument()
    const aside = container.querySelector('aside[aria-label="Detalhes do jogo"]') as HTMLElement
    expect(aside).toHaveTextContent('Nenhum jogo selecionado')
    expect(aside.querySelector('button')).toBeNull()
  })

  it('TDK-026/aba vazia: "Nenhum jogo nesta plataforma" (ou "Nenhum jogo na biblioteca" em Todos)', () => {
    const { unmount } = setup(model({ ...EMPTY, tab: 1 }), null)
    expect(screen.getByText('Nenhum jogo nesta plataforma')).toBeInTheDocument()
    expect(screen.getByText('Nenhum jogo selecionado')).toBeInTheDocument()
    unmount()
    setup(model({ ...EMPTY, tab: 0 }), null)
    expect(screen.getByText('Nenhum jogo na biblioteca')).toBeInTheDocument()
    expect(screen.queryByText('Nenhum jogo nesta plataforma')).toBeNull()
  })

  it('TDK-026/jogo sem detalhe: "Carregando jogo…" no painel', () => {
    const { container } = setup(model(), null)
    const aside = container.querySelector('aside[aria-label="Detalhes do jogo"]') as HTMLElement
    expect(aside).toHaveTextContent('Carregando jogo…')
    expect(aside.querySelector('button')).toBeNull()
  })

  it('TDK-026/erro em role="alert" logo abaixo do cabeçalho, sem botão de fechar; some sem erro', () => {
    const { container, rerender } = setup(model({ alert: 'Já há um jogo em execução.' }))
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Já há um jogo em execução.')
    expect(alert.querySelector('button, [role="button"]')).toBeNull()
    expect(after(container.querySelector('header')!, alert)).toBe(true)
    expect(after(alert, screen.getByRole('navigation', { name: 'Plataformas' }))).toBe(true)
    rerender(<Desktop model={model({ alert: null })} details={STUB} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} onVideoError={vi.fn()} />)
    expect(screen.queryByRole('alert')).toBeNull()
  })
})

describe('Desktop: acessibilidade e foco', () => {
  it('TDK-029/o centro é listbox "Jogos" com opções (aria-label com o título, aria-selected no foco)', () => {
    setup()
    const list = screen.getByRole('listbox', { name: 'Jogos' })
    const options = within(list).getAllByRole('option')
    expect(options).toHaveLength(12)
    expect(options[0]).toHaveAttribute('aria-label', 'Jogo 1')
    options.forEach((o, i) => expect(o).toHaveAttribute('aria-selected', i === 2 ? 'true' : 'false'))
  })

  it('TDK-023/exatamente um elemento com data-focused="true" por zona (games, platforms, painel)', () => {
    const { container, rerender } = setup(model({ zone: 'games' }))
    const marks = () => Array.from(container.querySelectorAll('[data-focused="true"]'))
    const nonPanel = () => marks().filter((e) => e.getAttribute('data-testid') !== 'panel-stub')
    // games: the focused option
    expect(nonPanel()).toEqual([screen.getAllByRole('option')[2]])
    const props = { details: STUB, onPick: vi.fn(), onPlay: vi.fn(), onDetails: vi.fn(), onVideoError: vi.fn(), onTab: vi.fn() }
    // platforms: the active nav item, and no option
    rerender(<Desktop model={model({ zone: 'platforms' })} {...props} />)
    const nav = screen.getByRole('navigation', { name: 'Plataformas' })
    expect(nonPanel()).toHaveLength(1)
    expect(nav).toContainElement(nonPanel()[0] as HTMLElement)
    expect(nonPanel()[0]).toHaveAttribute('aria-current', 'true')
    // panel (screen details): nothing in the games or platforms columns; the mark is the details node's
    rerender(<Desktop model={model({ screen: 'details', zone: 'games' })} {...props} />)
    expect(nonPanel()).toHaveLength(0)
    expect(marks()).toHaveLength(1)
  })

  it('TDK-023/fora da zona games o jogo em foco mantém o destaque de seleção sem data-focused', () => {
    setup(model({ zone: 'platforms' }))
    const sel = screen.getAllByRole('option')[2]
    expect(sel).toHaveAttribute('aria-selected', 'true')
    expect(sel).not.toHaveAttribute('data-focused', 'true')
    expect(look(sel)).toMatch(/bb-surface/)
    expect(look(sel)).not.toMatch(/outline:\s*2px solid/)
  })

  it('TDK-025/clicar numa plataforma chama onTab com o índice; clicar num jogo chama onPick e duplo clique onPlay', () => {
    const { onTab, onPick, onPlay } = setup()
    fireEvent.click(screen.getByRole('button', { name: 'Super Nintendo, 80 jogos' }))
    expect(onTab).toHaveBeenCalledWith(2)
    fireEvent.click(screen.getAllByRole('option')[4])
    expect(onPick).toHaveBeenCalledWith(4)
    fireEvent.doubleClick(screen.getAllByRole('option')[4])
    expect(onPlay).toHaveBeenCalledWith(4)
  })
})
