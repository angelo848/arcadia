// Assumed markup (T15 must follow) for DesktopDetails [TDK-008..TDK-011, TDK-018, TDK-022, TDK-025, TDK-030]:
//  root <aside aria-label="Detalhes do jogo" data-bb-details> and it is the only scroller (the panel scrolls back to the top when
//  game.id changes; scrollSignal calls scrollBy on it only with active = true); h1 title; platform icon <img src=platformIconUrl>,
//  platform name and session mark text ("Em execução"/"Pausado"); the viewer is <section role="region" aria-label="Imagens"> holding
//  the media (img/video), a type label ("Vídeo" or "Screenshot"), "<k> de <n>" in an element with aria-live="polite", and the thumbs
//  as [data-gallery-item] elements (aria-current="true" on the selected, not buttons) only with n >= 2; the viewer carries
//  data-focused="true" with detailsFocus = gallery and active. Buttons "Jogar" then "Jogar sem contar tempo" carry data-focused.
//  Fields are label/value pairs in the order of TDK-008; the description is plain text (whole). In Navegar (active = false) the viewer
//  shows coverUrl (or the placeholder) and the FocusVideo from `preview` (2000 ms); in Detalhes (active = true) it shows the gallery
//  item and plays only the selected video item (the Video of BigBoxDetails), never both.
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { desktopLabels } from '../../desktopLabels'
import { coverPlaceholderUrl } from '../../desktopLayout'
import type { GameDetailView } from '../../../lib/library'
import type { MediaItemView } from '../../../lib/metadata'
import type { BigBoxDetailsProps } from '../../BigBoxDetails'
import { DesktopDetails } from './DesktopDetails'

const game: GameDetailView = {
  id: 7, platformId: 2, title: 'Chrono Quest', platform: 'Super Nintendo', path: 'C:\\g\\cq.sfc', status: 'not_played' as GameDetailView['status'],
  favorite: false, year: 1995, totalPlaySeconds: 7260, playCount: 4, firstPlayedAt: '2024-01-01T10:00:00Z', lastPlayedAt: '2026-11-14T12:00:00Z',
  description: 'Uma descricao longa e completa do jogo '.repeat(20).trim(), emulatorId: null, launchType: 'exe', launchArgs: '',
  sessionPlaySeconds: 0, completedAt: null, developers: ['Squaresoft'], publishers: ['Enix'], genres: ['RPG', 'Aventura'], playModes: ['single'],
}
const item = (id: number, kind: MediaItemView['kind']): MediaItemView => ({
  id, kind, provider: 'igdb', path: `p${id}`, thumbPath: `t${id}`, url: `/media/${id}.${kind === 'video' ? 'mp4' : 'png'}`, thumbUrl: `/media/t${id}.png`,
})
const GALLERY = [item(1, 'video'), item(2, 'screenshot'), item(3, 'screenshot')]
const TTB: BigBoxDetailsProps['timeToBeat'] = [
  { label: 'História principal', value: '20 h' }, { label: 'História + extras', value: '31½ h' }, { label: '100%', value: '41 h' },
]
function props(over: Partial<BigBoxDetailsProps> = {}): BigBoxDetailsProps {
  const g = over.game ?? game
  return {
    game: g, media: GALLERY, session: null, detailsFocus: 'play', gallery: 0, sound: false, scrollSignal: 0, onPlay: vi.fn(),
    onPlayUntracked: vi.fn(), timeToBeat: TTB, timeToBeatSource: 'HowLongToBeat', active: true, platformIconUrl: '/i/snes.png',
    coverUrl: 'http://m/cover.jpg', labels: desktopLabels(g),
    preview: { gameId: g.id, videoUrl: '/media/1.mp4', active: true, failed: false, sound: false, onVideoError: vi.fn() }, ...over,
  }
}
const aside = (c: HTMLElement) => c.querySelector('aside[aria-label="Detalhes do jogo"]') as HTMLElement
const region = () => screen.getByRole('region', { name: 'Imagens' })
const after = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
const proto = Element.prototype as unknown as { scrollIntoView?: unknown }
const originalScrollIntoView = proto.scrollIntoView

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
  if (originalScrollIntoView === undefined) delete proto.scrollIntoView
  else proto.scrollIntoView = originalScrollIntoView
  delete (HTMLElement.prototype as unknown as { scrollBy?: unknown }).scrollBy
})

describe('DesktopDetails: conteúdo', () => {
  it('TDK-008/título, ícone e plataforma, marcação de sessão, Jogar largo e Jogar sem contar tempo abaixo', () => {
    const { container } = render(<DesktopDetails {...props({ session: 'running' })} />)
    const root = aside(container)
    expect(within(root).getByRole('heading', { name: 'Chrono Quest' })).toBeInTheDocument()
    expect(within(root).getByText('Super Nintendo')).toBeInTheDocument()
    expect(root.querySelector('img[src="/i/snes.png"]')).not.toBeNull()
    expect(within(root).getByText('Em execução')).toBeInTheDocument()
    const play = within(root).getByRole('button', { name: 'Jogar' })
    const untracked = within(root).getByRole('button', { name: 'Jogar sem contar tempo' })
    expect(after(play, untracked)).toBe(true)
    expect(after(region(), play)).toBe(true)
    expect(`${play.className} ${play.getAttribute('style') ?? ''}`).toMatch(/bb-accent/)
  })

  it('TDK-008/campos na ordem, com "—" para vazio e a Descrição inteira', () => {
    const full = { ...game, completedAt: '2025-03-04T00:00:00Z', favorite: true }
    const { container, rerender } = render(<DesktopDetails {...props({ game: full })} />)
    const labelsOf = desktopLabels(full)
    const order = [
      'Tempo total', 'Primeira vez jogado', 'Última vez jogado', 'Tempo para zerar', 'Concluído em', 'Status', 'Marcação', 'Gêneros',
      'Modos de jogo', 'Desenvolvedora', 'Editora', 'Ano',
    ]
    const els = order.map((l) => within(aside(container)).getByText(l))
    for (let i = 1; i < els.length; i += 1) expect(after(els[i - 1], els[i]), order[i]).toBe(true)
    for (const v of [labelsOf.totalTime, labelsOf.status, labelsOf.genres, labelsOf.developers, labelsOf.publishers, '1995', 'Favorito']) {
      expect(within(aside(container)).getAllByText(v).length, v).toBeGreaterThanOrEqual(1)
    }
    expect(within(aside(container)).getByText(full.description)).toBeInTheDocument()
    const bare = { ...game, genres: [], playModes: [], developers: [], publishers: [], year: null, completedAt: null, favorite: false }
    rerender(<DesktopDetails {...props({ game: bare })} />)
    expect(within(aside(container)).getAllByText('—').length).toBeGreaterThanOrEqual(4)
    expect(within(aside(container)).queryByText('Concluído em')).toBeNull()
    expect(within(aside(container)).queryByText('Marcação')).toBeNull()
  })

  it('TDK-008/ao trocar de jogo o painel volta ao topo', () => {
    const { container, rerender } = render(<DesktopDetails {...props()} />)
    const root = aside(container)
    root.scrollTop = 120
    const other = { ...game, id: 8, title: 'Outro' }
    rerender(<DesktopDetails {...props({ game: other })} />)
    expect(aside(container).scrollTop).toBe(0)
  })

  it('TDK-011/o painel mostra o tempo para zerar sem botões de atualizar ou confirmar', () => {
    const { container } = render(<DesktopDetails {...props({ active: false })} />)
    const root = aside(container)
    expect(within(root).getByText('Tempo para zerar')).toBeInTheDocument()
    for (const t of ['História principal', '20 h', 'História + extras', '31½ h', '100%', '41 h']) expect(within(root).getByText(t)).toBeInTheDocument()
    expect(within(root).getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
    expect(within(root).queryByRole('button', { name: /atualizar|confirmar|escolher|buscar|nenhum destes/i })).toBeNull()
  })
})

describe('DesktopDetails: visor', () => {
  it('TDK-009/n = 0: capa (ou placeholder), sem foco, sem miniaturas', () => {
    const { container, rerender } = render(<DesktopDetails {...props({ media: [], detailsFocus: 'gallery' })} />)
    expect(region().querySelector('img')?.getAttribute('src')).toBe('http://m/cover.jpg')
    expect(region()).not.toHaveAttribute('data-focused', 'true')
    expect(container.querySelector('[data-gallery-item]')).toBeNull()
    rerender(<DesktopDetails {...props({ media: [], coverUrl: null })} />)
    expect(region().querySelector('img')?.getAttribute('src')).toBe(coverPlaceholderUrl)
  })

  it('TDK-009/n = 1: item inteiro, rótulo e "1 de 1", sem miniaturas', () => {
    const { container } = render(<DesktopDetails {...props({ media: [item(2, 'screenshot')] })} />)
    expect(within(region()).getByText('Screenshot')).toBeInTheDocument()
    expect(region().querySelector('img')?.getAttribute('src')).toBe('/media/2.png')
    expect(region().querySelector('[aria-live="polite"]')).toHaveTextContent('1 de 1')
    expect(container.querySelector('[data-gallery-item]')).toBeNull()
  })

  it('TDK-009/n = 3: rótulo Vídeo/Screenshot, "k de n" em aria-live polite e miniaturas com a selecionada marcada, sem papel de botão', () => {
    const { container, rerender } = render(<DesktopDetails {...props({ gallery: 0 })} />)
    expect(within(region()).getAllByText('Vídeo').length).toBeGreaterThanOrEqual(1)
    expect(region().querySelector('[aria-live="polite"]')).toHaveTextContent('1 de 3')
    const thumbs = () => Array.from(container.querySelectorAll('[data-gallery-item]'))
    expect(thumbs()).toHaveLength(3)
    expect(thumbs().map((t) => t.getAttribute('aria-current'))).toEqual(['true', null, null])
    rerender(<DesktopDetails {...props({ gallery: 2 })} />)
    expect(within(region()).getByText('Screenshot')).toBeInTheDocument()
    expect(region().querySelector('[aria-live="polite"]')).toHaveTextContent('3 de 3')
    expect(thumbs().map((t) => t.getAttribute('aria-current'))).toEqual([null, null, 'true'])
    for (const t of thumbs()) {
      expect(t.closest('button, [role="button"]')).toBeNull()
      expect(t.tagName).not.toBe('BUTTON')
    }
  })
})

describe('DesktopDetails: vídeo', () => {
  const navegar = (o: Partial<BigBoxDetailsProps> = {}) => props({ active: false, detailsFocus: 'play', ...o })

  it('TDK-010/Navegar: capa e, depois de 2.000 ms, o FocusVideo dentro do visor; no máximo um <video>', () => {
    const { container } = render(<DesktopDetails {...navegar()} />)
    expect(region().querySelector('img')?.getAttribute('src')).toBe('http://m/cover.jpg')
    act(() => { vi.advanceTimersByTime(1999) })
    expect(container.querySelectorAll('video')).toHaveLength(0)
    act(() => { vi.advanceTimersByTime(1) })
    const videos = container.querySelectorAll('video')
    expect(videos).toHaveLength(1)
    expect(region()).toContainElement(videos[0] as HTMLElement)
    expect(videos[0].getAttribute('src')).toBe('/media/1.mp4')
    expect(videos[0].hasAttribute('controls')).toBe(false)
  })

  it('TDK-010/Detalhes: o <video> de Navegar não existe e a galeria toca só o item selecionado', () => {
    const { container, rerender } = render(<DesktopDetails {...props({ active: true, gallery: 0 })} />)
    act(() => { vi.advanceTimersByTime(5000) })
    expect(container.querySelectorAll('video')).toHaveLength(1)
    expect(container.querySelector('video')?.getAttribute('src')).toBe('/media/1.mp4')
    expect(container.querySelector('[data-testid="bb-video"]')).toBeNull()
    rerender(<DesktopDetails {...props({ active: true, gallery: 1 })} />)
    act(() => { vi.advanceTimersByTime(5000) })
    expect(container.querySelectorAll('video')).toHaveLength(0)
  })

  it('TDK-010/failed não tenta de novo, e sem vídeo no jogo nada toca', () => {
    const { container, rerender } = render(<DesktopDetails {...navegar({ preview: { ...props().preview!, failed: true } })} />)
    act(() => { vi.advanceTimersByTime(10000) })
    expect(container.querySelectorAll('video')).toHaveLength(0)
    rerender(<DesktopDetails {...navegar({ preview: { ...props().preview!, videoUrl: null } })} />)
    act(() => { vi.advanceTimersByTime(10000) })
    expect(container.querySelectorAll('video')).toHaveLength(0)
  })
})

describe('DesktopDetails: foco e rolagem', () => {
  it('TDK-018/active=true marca o foco em Jogar, Jogar sem contar tempo ou visor (uma só marca); active=false nenhuma', () => {
    const { container, rerender } = render(<DesktopDetails {...props({ detailsFocus: 'play' })} />)
    const marks = () => Array.from(container.querySelectorAll('[data-focused="true"]'))
    expect(marks()).toEqual([screen.getByRole('button', { name: 'Jogar' })])
    rerender(<DesktopDetails {...props({ detailsFocus: 'playUntracked' })} />)
    expect(marks()).toEqual([screen.getByRole('button', { name: 'Jogar sem contar tempo' })])
    rerender(<DesktopDetails {...props({ detailsFocus: 'gallery' })} />)
    expect(marks()).toEqual([region()])
    rerender(<DesktopDetails {...props({ detailsFocus: 'play', active: false })} />)
    expect(marks()).toHaveLength(0)
  })

  it('TDK-018/o analógico direito rola o painel (scrollSignal) sem mover o foco; inativo não reage', () => {
    const scrollBy = vi.fn()
    ;(HTMLElement.prototype as unknown as { scrollBy: unknown }).scrollBy = scrollBy
    const { container, rerender } = render(<DesktopDetails {...props({ scrollSignal: 0 })} />)
    scrollBy.mockClear()
    act(() => { rerender(<DesktopDetails {...props({ scrollSignal: 1 })} />) })
    expect(scrollBy).toHaveBeenCalled()
    expect(scrollBy.mock.instances.at(-1)).toBe(aside(container))
    act(() => { rerender(<DesktopDetails {...props({ scrollSignal: -1 })} />) })
    const last = scrollBy.mock.calls.at(-1)?.[0] as { top?: number } | undefined
    expect(last?.top === undefined || last.top < 0).toBe(true)
    expect(Array.from(container.querySelectorAll('[data-focused="true"]'))).toEqual([screen.getByRole('button', { name: 'Jogar' })])
    scrollBy.mockClear()
    act(() => { rerender(<DesktopDetails {...props({ scrollSignal: 5, active: false })} />) })
    expect(scrollBy).not.toHaveBeenCalled()
  })

  it('TDK-022/o elemento focado é levado à vista (scrollIntoView block nearest)', () => {
    const scroll = vi.fn()
    proto.scrollIntoView = scroll
    const { rerender } = render(<DesktopDetails {...props({ detailsFocus: 'play' })} />)
    scroll.mockClear()
    rerender(<DesktopDetails {...props({ detailsFocus: 'playUntracked' })} />)
    expect(scroll).toHaveBeenCalled()
    expect(scroll.mock.calls.at(-1)?.[0]).toMatchObject({ block: 'nearest' })
    expect((scroll.mock.contexts as Element[]).at(-1)).toBe(screen.getByRole('button', { name: 'Jogar sem contar tempo' }))
    delete proto.scrollIntoView
    expect(() => rerender(<DesktopDetails {...props({ detailsFocus: 'play' })} />)).not.toThrow()
  })

  it('TDK-025/Jogar e Jogar sem contar tempo chamam onPlay e onPlayUntracked', () => {
    const p = props()
    render(<DesktopDetails {...p} />)
    fireEvent.click(screen.getByRole('button', { name: 'Jogar' }))
    expect(p.onPlay).toHaveBeenCalledTimes(1)
    expect(p.onPlayUntracked).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Jogar sem contar tempo' }))
    expect(p.onPlayUntracked).toHaveBeenCalledTimes(1)
    expect(p.onPlay).toHaveBeenCalledTimes(1)
  })
})

describe('DesktopDetails: o que fica fora', () => {
  it('TDK-030/nenhum Editar, Buscar metadados, Buscar vídeo, Abrir local, Remover, sessões, Ampliar nem avisos de emulador/rastreio no DOM', () => {
    const rich = { ...game, completedAt: '2025-03-04T00:00:00Z', favorite: true, launchType: 'emulator' as GameDetailView['launchType'] }
    for (const active of [true, false]) {
      const { container, unmount } = render(<DesktopDetails {...props({ game: rich, active, session: 'paused' })} />)
      const text = container.textContent ?? ''
      for (const re of [
        /Editar/, /Buscar metadados/, /Buscar vídeo/, /Abrir local do arquivo/, /Remover/, /Adicionar sessão/, /Carregar mais/, /Ampliar/,
        /Sessões/, /Sem emulador configurado/, /Rastreio por processo/, /Pausar|Retomar/, /Jogados recentemente/,
      ]) {
        expect(text, `${active}: ${re}`).not.toMatch(re)
      }
      expect(screen.queryByRole('button', { name: /editar|buscar|abrir|remover|adicionar|ampliar|pausar|retomar|confirmar|escolher/i })).toBeNull()
      expect(screen.getAllByRole('button').map((b) => b.textContent)).toEqual(['Jogar', 'Jogar sem contar tempo'])
      unmount()
    }
  })
})
