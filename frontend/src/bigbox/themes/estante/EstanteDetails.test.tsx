// Assumed markup (T36 must follow) for EstanteDetails [ETN-010..ETN-012, TTB-016, BBX-035, BBX-036, BBX-043]:
//  root <section data-bb-details> (inline style height 100%, minHeight 0, overflow hidden) with est-d-left (62 %) before est-d-right (38 %);
//  left: h1 title; description region (role "region", name "Descrição", inline overflowY auto, the only scroller, inside a Panel);
//  buttons "Iniciar jogo" (detailsFocus play) and "Jogar sem contar tempo" (playUntracked) with data-focused; strip est-d-played inside a Panel;
//  carousel (only with gallery): est-d-viewer (data-focused when detailsFocus gallery; img/video object-contain) and est-d-dots with one
//  est-d-dot per item (aria-current="true" on the selected, bb-text fill; the others hollow with a bb-text-muted border).
//  right: panels est-d-stats ("Estatísticas do jogo": Tempo jogado, Sessões, Última vez), est-d-ttb ("Tempo para zerar" + TimeToBeatList) and
//  est-d-about ("Sobre": Plataforma, Gêneros, Modos de jogo, Desenvolvedor, Publicado por, Ano de lançamento), rows omitted when empty.
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatPlayTime } from '../../../lib/games'
import type { GameDetailView } from '../../../lib/library'
import type { MediaItemView } from '../../../lib/metadata'
import type { BigBoxDetailsProps } from '../../BigBoxDetails'
import { EstanteDetails } from './EstanteDetails'

const game: GameDetailView = {
  id: 7, platformId: 2, title: 'Chrono Quest', platform: 'Super Nintendo', path: 'C:\\g\\cq.sfc', status: 'none' as GameDetailView['status'],
  favorite: false, year: 1995, totalPlaySeconds: 7260, playCount: 4, firstPlayedAt: '2024-01-01T10:00:00Z', lastPlayedAt: '2026-11-14T12:00:00Z',
  description: 'Uma descricao longa e completa do jogo '.repeat(30).trim(), emulatorId: null, launchType: 'exe', launchArgs: '', completedAt: null,
  developers: ['Squaresoft'], publishers: ['Enix'], genres: ['RPG', 'Aventura'], playModes: ['single', 'coop_local'],
}
const item = (id: number, kind: MediaItemView['kind']): MediaItemView => ({
  id, kind, provider: 'igdb', path: `p${id}`, thumbPath: `t${id}`, url: `/media/${id}.${kind === 'video' ? 'mp4' : 'png'}`, thumbUrl: `/media/t${id}.png`,
})
const TTB_ROWS: BigBoxDetailsProps['timeToBeat'] = [
  { label: 'História principal', value: '20 h' }, { label: 'História + extras', value: '31½ h' }, { label: '100%', value: '41 h' },
]
function props(over: Partial<BigBoxDetailsProps> = {}): BigBoxDetailsProps {
  return {
    game, media: [item(1, 'screenshot'), item(2, 'video'), item(3, 'screenshot'), item(4, 'cover')], session: null, detailsFocus: 'play', gallery: 0,
    sound: false, scrollSignal: 0, onPlay: vi.fn(), onPlayUntracked: vi.fn(), timeToBeat: TTB_ROWS, timeToBeatSource: 'HowLongToBeat', ...over,
  }
}
const inPanel = (el: Element | null) => el?.closest('[data-bb-panel]') != null
const after = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
const look = (el: Element) => `${el.className} ${el.getAttribute('style') ?? ''}`
const desc = () => screen.getByRole('region', { name: /descrição/i })
const dots = () => screen.getAllByTestId('est-d-dot')

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-11-22T12:00:00Z'))
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('EstanteDetails', () => {
  it('ETN-010/estrutura: coluna da esquerda (título, descrição, botões, faixa, carrossel) antes da coluna da direita (painéis)', () => {
    const { container } = render(<EstanteDetails {...props()} />)
    const left = screen.getByTestId('est-d-left')
    const right = screen.getByTestId('est-d-right')
    expect(after(left, right)).toBe(true)
    expect(within(left).getByRole('heading', { level: 1, name: 'Chrono Quest' })).toBeInTheDocument()
    expect(left).toContainElement(desc())
    expect(left).toContainElement(screen.getByRole('button', { name: 'Iniciar jogo' }))
    expect(left).toContainElement(screen.getByRole('button', { name: 'Jogar sem contar tempo' }))
    expect(left).toContainElement(screen.getByTestId('est-d-played'))
    expect(left).toContainElement(screen.getByTestId('est-d-viewer'))
    expect(left).toContainElement(screen.getByTestId('est-d-dots'))
    for (const id of ['est-d-stats', 'est-d-ttb', 'est-d-about']) expect(right).toContainElement(screen.getByTestId(id))
    expect(container.querySelector('[data-bb-details]')).toContainElement(left)
  })

  it('ETN-010/a descrição completa fica numa região com rolagem, dentro de um painel', () => {
    render(<EstanteDetails {...props()} />)
    expect(desc().textContent).toBe(game.description)
    expect(inPanel(desc())).toBe(true)
  })

  it('ETN-010/BBX-043/a raiz tem altura 100%, min-height 0 e overflow hidden; só a descrição tem overflow-y auto', () => {
    const { container } = render(<EstanteDetails {...props()} />)
    const root = container.querySelector('[data-bb-details]') as HTMLElement
    expect(root).toBeTruthy()
    expect(root.style.height).toBe('100%')
    expect(['0', '0px']).toContain(root.style.minHeight)
    expect(root.style.overflow).toBe('hidden')
    const scrollers = Array.from(root.querySelectorAll<HTMLElement>('*')).filter((e) => e.style.overflowY === 'auto')
    expect(scrollers).toEqual([desc()])
  })

  it('ETN-010/botões "Iniciar jogo" e "Jogar sem contar tempo" chamam onPlay e onPlayUntracked', () => {
    const p = props()
    render(<EstanteDetails {...p} />)
    fireEvent.click(screen.getByRole('button', { name: 'Iniciar jogo' }))
    fireEvent.click(screen.getByRole('button', { name: 'Jogar sem contar tempo' }))
    expect(p.onPlay).toHaveBeenCalledTimes(1)
    expect(p.onPlayUntracked).toHaveBeenCalledTimes(1)
  })

  it('ETN-010/BBX-035/uma única marca de foco: o botão focado, ou o visor quando o foco está no carrossel', () => {
    const { container, rerender } = render(<EstanteDetails {...props({ detailsFocus: 'play' })} />)
    const marks = () => Array.from(container.querySelectorAll('[data-focused="true"]'))
    expect(marks()).toEqual([screen.getByRole('button', { name: 'Iniciar jogo' })])
    rerender(<EstanteDetails {...props({ detailsFocus: 'playUntracked' })} />)
    expect(marks()).toEqual([screen.getByRole('button', { name: 'Jogar sem contar tempo' })])
    rerender(<EstanteDetails {...props({ detailsFocus: 'gallery' })} />)
    expect(marks()).toEqual([screen.getByTestId('est-d-viewer')])
    for (const d of dots()) expect(d).not.toHaveAttribute('data-focused', 'true')
  })

  it('ETN-010/faixa "Jogado por <tempo> · Jogado recentemente <relativo>" num painel', () => {
    render(<EstanteDetails {...props()} />)
    const played = screen.getByTestId('est-d-played')
    expect(played).toHaveTextContent(`Jogado por ${formatPlayTime(7260)} · Jogado recentemente há 8 dias`)
    expect(inPanel(played)).toBe(true)
  })

  it('ETN-010/jogo nunca jogado mostra "Nunca jogado" na faixa', () => {
    render(<EstanteDetails {...props({ game: { ...game, totalPlaySeconds: 0, playCount: 0, lastPlayedAt: '' } })} />)
    const played = screen.getByTestId('est-d-played')
    expect(played).toHaveTextContent('Nunca jogado')
    expect(played.textContent).not.toMatch(/Jogado por|Jogado recentemente/)
  })

  it('BBX-036/scrollSignal rola a descrição com o foco em qualquer zona', () => {
    const scrollBy = vi.fn()
    ;(HTMLElement.prototype as unknown as { scrollBy: unknown }).scrollBy = scrollBy
    for (const detailsFocus of ['play', 'playUntracked', 'gallery'] as const) {
      const { rerender, unmount } = render(<EstanteDetails {...props({ detailsFocus, scrollSignal: 0 })} />)
      scrollBy.mockClear()
      act(() => {
        rerender(<EstanteDetails {...props({ detailsFocus, scrollSignal: 1 })} />)
      })
      expect(scrollBy, detailsFocus).toHaveBeenCalled()
      expect(scrollBy.mock.instances.at(-1)).toBe(desc())
      act(() => {
        rerender(<EstanteDetails {...props({ detailsFocus, scrollSignal: -1 })} />)
      })
      const last = scrollBy.mock.calls.at(-1)?.[0] as { top?: number } | undefined
      expect(last?.top === undefined || last.top < 0).toBe(true)
      unmount()
    }
    delete (HTMLElement.prototype as unknown as { scrollBy?: unknown }).scrollBy
  })

  it('ETN-011/o visor mostra o item selecionado inteiro (contain) e o contador de itens é uma bolinha por item (vídeo primeiro)', () => {
    const { rerender } = render(<EstanteDetails {...props({ gallery: 1 })} />)
    expect(dots()).toHaveLength(3) // video + 2 screenshots; the cover is not in the gallery
    const viewer = screen.getByTestId('est-d-viewer')
    const img = viewer.querySelector('img') as HTMLImageElement
    expect(img.getAttribute('src')).toBe('/media/1.png')
    expect(img.className).toContain('object-contain')
    rerender(<EstanteDetails {...props({ gallery: 2 })} />)
    expect(screen.getByTestId('est-d-viewer').querySelector('img')?.getAttribute('src')).toBe('/media/3.png')
    rerender(<EstanteDetails {...props({ gallery: 9 })} />)
    expect(screen.getByTestId('est-d-viewer').querySelector('img')?.getAttribute('src')).toBe('/media/3.png')
  })

  it('ETN-011/bolinha selecionada cheia (bb-text) com aria-current; as outras vazadas (borda bb-text-muted, sem preenchimento)', () => {
    const { rerender } = render(<EstanteDetails {...props({ gallery: 0 })} />)
    const state = () => dots().map((d) => d.getAttribute('aria-current') === 'true')
    expect(state()).toEqual([true, false, false])
    expect(dots().filter((d) => d.hasAttribute('aria-current') && d.getAttribute('aria-current') !== 'true')).toHaveLength(0)
    const filled = /bg-bb-text(?![-\w])|background[^;]*--arc-bb-text\)/
    const hollow = /border-bb-text-muted|border[^;]*--arc-bb-text-muted/
    expect(look(dots()[0])).toMatch(filled)
    for (const d of dots().slice(1)) {
      expect(look(d)).toMatch(hollow)
      expect(look(d)).not.toMatch(filled)
    }
    rerender(<EstanteDetails {...props({ gallery: 2 })} />)
    expect(state()).toEqual([false, false, true])
    expect(look(dots()[2])).toMatch(filled)
  })

  it('ETN-011/vídeo selecionado toca no visor (muda sem som, loop, playsInline) e perde o src ao trocar; no máximo um video', () => {
    const { container, rerender } = render(<EstanteDetails {...props({ gallery: 0 })} />)
    const v = screen.getByTestId('est-d-viewer').querySelector('video') as HTMLVideoElement
    expect(v).not.toBeNull()
    expect(v.getAttribute('src')).toBe('/media/2.mp4')
    expect(v.muted).toBe(true)
    expect(v.loop).toBe(true)
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalled()
    expect(container.querySelectorAll('video')).toHaveLength(1)
    rerender(<EstanteDetails {...props({ gallery: 1 })} />)
    const after1 = container.querySelector('video')
    expect(after1 === null || !after1.hasAttribute('src')).toBe(true)
    expect(container.querySelectorAll('video').length).toBeLessThanOrEqual(1)
  })

  it('ETN-011/sem galeria o carrossel (visor e bolinhas) não aparece e os botões continuam', () => {
    render(<EstanteDetails {...props({ media: [item(4, 'cover')] })} />)
    expect(screen.queryByTestId('est-d-viewer')).toBeNull()
    expect(screen.queryByTestId('est-d-dots')).toBeNull()
    expect(screen.queryAllByTestId('est-d-dot')).toHaveLength(0)
    expect(screen.getByRole('button', { name: 'Iniciar jogo' })).toBeInTheDocument()
    expect(screen.getByTestId('est-d-right')).toBeInTheDocument()
  })

  it('ETN-012/"Estatísticas do jogo": Tempo jogado, Sessões e Última vez, num painel', () => {
    render(<EstanteDetails {...props()} />)
    const stats = screen.getByTestId('est-d-stats')
    expect(inPanel(stats)).toBe(true)
    expect(within(stats).getByText('Estatísticas do jogo')).toBeInTheDocument()
    expect(within(stats).getByText('Tempo jogado')).toBeInTheDocument()
    expect(within(stats).getByText(formatPlayTime(7260))).toBeInTheDocument()
    expect(within(stats).getByText('Sessões')).toBeInTheDocument()
    expect(within(stats).getByText('4')).toBeInTheDocument()
    expect(within(stats).getByText('Última vez')).toBeInTheDocument()
    expect(within(stats).getByText(new Date(game.lastPlayedAt).toLocaleDateString('pt-BR'))).toBeInTheDocument()
  })

  it('ETN-012/nunca jogado mostra "Nunca jogado" em Última vez', () => {
    render(<EstanteDetails {...props({ game: { ...game, totalPlaySeconds: 0, playCount: 0, lastPlayedAt: '' } })} />)
    const stats = screen.getByTestId('est-d-stats')
    expect(within(stats).getByText('Última vez')).toBeInTheDocument()
    // "Tempo jogado" also reads "Nunca jogado" with zero seconds (UIX-028), so the label is checked in the row of "Última vez".
    expect(within(stats).getAllByText('Nunca jogado').length).toBeGreaterThanOrEqual(1)
    expect(within(stats).getByText('Última vez').parentElement).toHaveTextContent('Nunca jogado')
    expect(within(stats).getByText('0')).toBeInTheDocument()
  })

  it('ETN-012/"Sobre": Plataforma, Gêneros, Modos de jogo, Desenvolvedor, Publicado por e Ano de lançamento', () => {
    render(<EstanteDetails {...props()} />)
    const about = screen.getByTestId('est-d-about')
    expect(inPanel(about)).toBe(true)
    expect(within(about).getByText('Sobre')).toBeInTheDocument()
    const pairs: Array<[string, string]> = [
      ['Plataforma', 'Super Nintendo'], ['Gêneros', 'RPG, Aventura'], ['Modos de jogo', 'Um jogador, Cooperativo local'],
      ['Desenvolvedor', 'Squaresoft'], ['Publicado por', 'Enix'], ['Ano de lançamento', '1995'],
    ]
    for (const [label, value] of pairs) {
      expect(within(about).getByText(label), label).toBeInTheDocument()
      expect(within(about).getByText(value), value).toBeInTheDocument()
    }
  })

  it('ETN-012/cada linha de "Sobre" vazia é omitida (rótulo e valor)', () => {
    const bare = { ...game, genres: [], playModes: [], developers: [], publishers: [], year: null }
    render(<EstanteDetails {...props({ game: bare })} />)
    const about = screen.getByTestId('est-d-about')
    expect(within(about).getByText('Plataforma')).toBeInTheDocument()
    for (const label of ['Gêneros', 'Modos de jogo', 'Desenvolvedor', 'Publicado por', 'Ano de lançamento']) {
      expect(within(about).queryByText(label), label).toBeNull()
    }
  })

  it('ETN-012/TTB-016/"Tempo para zerar" num painel com os três rótulos e valores e a linha da fonte', () => {
    render(<EstanteDetails {...props()} />)
    const ttb = screen.getByTestId('est-d-ttb')
    expect(inPanel(ttb)).toBe(true)
    expect(within(ttb).getByText('Tempo para zerar')).toBeInTheDocument()
    for (const [label, value] of [['História principal', '20 h'], ['História + extras', '31½ h'], ['100%', '41 h']]) {
      expect(within(ttb).getByText(label)).toBeInTheDocument()
      expect(within(ttb).getByText(value)).toBeInTheDocument()
    }
    expect(within(ttb).getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
  })

  it('TTB-016/sem dado os três valores são "—", sem fonte; fonte IGDB aparece como "Fonte: IGDB"', () => {
    const empty = TTB_ROWS.map((r) => ({ ...r, value: '—' }))
    const { rerender } = render(<EstanteDetails {...props({ timeToBeat: empty, timeToBeatSource: null })} />)
    const ttb = () => screen.getByTestId('est-d-ttb')
    expect(within(ttb()).getAllByText('—')).toHaveLength(3)
    expect(within(ttb()).queryByText(/Fonte:/)).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    rerender(<EstanteDetails {...props({ timeToBeatSource: 'IGDB' })} />)
    expect(within(ttb()).getByText('Fonte: IGDB')).toBeInTheDocument()
  })

  it('TTB-016/não há botão de atualizar nem de confirmar: só os dois botões de jogar', () => {
    render(<EstanteDetails {...props()} />)
    expect(screen.queryByRole('button', { name: /atualizar|confirmar|escolher|buscar|gerenciar/i })).toBeNull()
    expect(screen.getAllByRole('button').map((b) => b.textContent)).toEqual(['Iniciar jogo', 'Jogar sem contar tempo'])
  })

  it('ETN-013/nenhum destes textos ou controles nos Detalhes: logotipo, Gerenciar, favoritar, busca, Todos os jogos, conquistas, avaliação, downloads, jogando agora, requisitos, similares, Co-Op, Vs e menu lateral', () => {
    const { container } = render(<EstanteDetails {...props({ session: 'running' })} />)
    const text = container.textContent ?? ''
    for (const re of [
      /Gerenciar/i, /Todos os jogos/i, /Modernos/i, /Clássicos/i, /Conquistas/i, /Avalia[çc][ãa]o/i, /Downloads?/i, /jogando agora/i,
      /Requisitos/i, /Ver similares/i, /\bCo-?Op\b/i /* alterado em 2026-11-22 (fase 15): "Cooperativo local" e rotulo legitimo */, /\bVs\b/, /Buscar|Pesquisar/i, /Ordenar/i,
    ]) {
      expect(text, String(re)).not.toMatch(re)
    }
    expect(container.querySelector('input, select, textarea, [role="searchbox"], [role="combobox"], nav, aside, [role="navigation"]')).toBeNull()
    expect(container.querySelector('img[alt*="logo" i]')).toBeNull()
    expect(screen.queryByRole('button', { name: /favorit/i })).toBeNull()
  })

  it('BBX-016/jogo sem descrição e sem mídia não quebra', () => {
    const bare: GameDetailView = { ...game, description: '', genres: [], developers: [], publishers: [], playModes: [], year: null, lastPlayedAt: '', playCount: 0, totalPlaySeconds: 0 }
    render(<EstanteDetails {...props({ game: bare, media: [], session: 'running' })} />)
    expect(screen.getByRole('heading', { name: 'Chrono Quest' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Iniciar jogo' })).toBeTruthy()
  })
})
