// BBX-016 (T38). Assumed DOM contract (see report): description region = role "region" named "Descrição";
// gallery items = [data-gallery-item], selected one has aria-current="true"; focused button has data-focused="true".
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatPlayTime } from '../lib/games'
import type { GameDetailView } from '../lib/library'
import type { MediaItemView } from '../lib/metadata'
import { BigBoxDetails } from './BigBoxDetails'
import type { BigBoxDetailsProps } from './BigBoxDetails'

const game: GameDetailView = {
  id: 7,
  platformId: 2,
  title: 'Chrono Quest',
  platform: 'Super Nintendo',
  path: 'C:\\g\\cq.sfc',
  status: 'none' as GameDetailView['status'],
  favorite: false,
  year: 1995,
  totalPlaySeconds: 7260,
  playCount: 4,
  firstPlayedAt: '2024-01-01T10:00:00Z',
  lastPlayedAt: '2024-03-05T12:00:00Z',
  description: 'Uma descricao longa e completa do jogo '.repeat(30).trim(),
  emulatorId: null,
  launchType: 'exe',
  launchArgs: '',
  completedAt: null,
  developers: ['Squaresoft'],
  publishers: ['Enix'],
  genres: ['RPG', 'Aventura'],
  playModes: ['single', 'coop_local'],
}

const item = (id: number, kind: MediaItemView['kind']): MediaItemView => ({
  id,
  kind,
  provider: 'igdb',
  path: `p${id}`,
  thumbPath: `t${id}`,
  url: `/media/${id}.${kind === 'video' ? 'mp4' : 'png'}`,
  thumbUrl: `/media/t${id}.png`,
})

const TTB_ROWS: BigBoxDetailsProps['timeToBeat'] = [
  { label: 'História principal', value: '20 h' },
  { label: 'História + extras', value: '31½ h' },
  { label: '100%', value: '41 h' },
]
const TTB_EMPTY: BigBoxDetailsProps['timeToBeat'] = TTB_ROWS.map((r) => ({ ...r, value: '—' }))

function props(over: Partial<BigBoxDetailsProps> = {}): BigBoxDetailsProps {
  return {
    game,
    media: [item(1, 'screenshot'), item(2, 'video'), item(3, 'screenshot'), item(4, 'cover')],
    session: null,
    detailsFocus: 'play',
    gallery: 0,
    sound: false,
    scrollSignal: 0,
    onPlay: vi.fn(),
    onPlayUntracked: vi.fn(),
    // alterado em 2026-11-22 (fase 15): timeToBeat e timeToBeatSource são props obrigatórias (contrato temas-v2 §2).
    timeToBeat: TTB_ROWS,
    timeToBeatSource: 'HowLongToBeat',
    ...over,
  }
}

const galleryItems = (c: HTMLElement) => Array.from(c.querySelectorAll('[data-gallery-item]'))

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('BigBoxDetails', () => {
  it('BBX-016/mostra título, descrição completa, plataforma, ano, gêneros, desenvolvedores, editoras e modos de jogo', () => {
    render(<BigBoxDetails {...props()} />)
    expect(screen.getByRole('heading', { name: 'Chrono Quest' })).toBeTruthy()
    const region = screen.getByRole('region', { name: /descrição/i })
    expect(region.textContent).toBe(game.description) // full, not shortened
    const text = document.body.textContent ?? ''
    for (const s of ['Super Nintendo', '1995', 'RPG', 'Aventura', 'Squaresoft', 'Enix', 'Um jogador', 'Cooperativo local']) {
      expect(text).toContain(s)
    }
  })

  it('BBX-016/mostra tempo jogado, última vez e sessões', () => {
    render(<BigBoxDetails {...props()} />)
    const text = document.body.textContent ?? ''
    expect(text).toContain(formatPlayTime(7260))
    expect(text).toContain(new Date(game.lastPlayedAt).toLocaleDateString('pt-BR'))
    expect(text).toContain('Sessões: 4')
  })

  it('BBX-016/a galeria tem o vídeo primeiro e os screenshots depois; sem vídeo, só screenshots', () => {
    const { container, rerender } = render(<BigBoxDetails {...props()} />)
    let items = galleryItems(container)
    expect(items).toHaveLength(3) // video + 2 screenshots; cover is not in the gallery
    expect(items[0].querySelector('video') ?? items[0].getAttribute('data-kind')).toBeTruthy()
    expect(items[0].getAttribute('data-kind')).toBe('video')
    expect(items.slice(1).map((e) => e.getAttribute('data-kind'))).toEqual(['screenshot', 'screenshot'])
    rerender(<BigBoxDetails {...props({ media: [item(1, 'screenshot'), item(3, 'screenshot')] })} />)
    items = galleryItems(container)
    expect(items.map((e) => e.getAttribute('data-kind'))).toEqual(['screenshot', 'screenshot'])
    expect(container.querySelector('video')).toBeNull()
  })

  it('BBX-016/o vídeo da galeria toca sem som ao aparecer e perde o src ao sair da galeria', () => {
    const { container, rerender } = render(<BigBoxDetails {...props({ gallery: 0 })} />)
    const v = container.querySelector('video') as HTMLVideoElement
    expect(v).not.toBeNull()
    expect(v.getAttribute('src')).toBe('/media/2.mp4')
    expect(v.muted).toBe(true)
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalled()
    rerender(<BigBoxDetails {...props({ gallery: 1 })} />)
    const after = container.querySelector('video')
    expect(after === null || !after.hasAttribute('src')).toBe(true)
  })

  it('BBX-016/gallery seleciona o item', () => {
    const { container, rerender } = render(<BigBoxDetails {...props({ gallery: 0 })} />)
    const sel = () => galleryItems(container).map((e) => e.getAttribute('aria-current') === 'true')
    expect(sel()).toEqual([true, false, false])
    rerender(<BigBoxDetails {...props({ gallery: 2 })} />)
    expect(sel()).toEqual([false, false, true])
  })

  it('BBX-016/os botões Jogar e Jogar sem contar tempo chamam onPlay e onPlayUntracked', () => {
    const p = props({ detailsFocus: 'playUntracked' })
    render(<BigBoxDetails {...p} />)
    fireEvent.click(screen.getByRole('button', { name: 'Jogar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Jogar sem contar tempo' }))
    expect(p.onPlay).toHaveBeenCalledTimes(1)
    expect(p.onPlayUntracked).toHaveBeenCalledTimes(1)
  })

  it('BBX-035/foco em play: só o botão Jogar tem data-focused true; foco em playUntracked: só ele; foco na galeria: só a fileira de miniaturas', () => {
    const { container, rerender } = render(<BigBoxDetails {...props({ detailsFocus: 'play' })} />)
    const play = screen.getByRole('button', { name: 'Jogar' })
    const untracked = screen.getByRole('button', { name: 'Jogar sem contar tempo' })
    const thumbs = () => container.querySelector('[data-bb-thumbs]') as HTMLElement
    const focused = () => Array.from(container.querySelectorAll('[data-focused="true"]'))
    expect(focused()).toEqual([play])
    rerender(<BigBoxDetails {...props({ detailsFocus: 'playUntracked' })} />)
    expect(focused()).toEqual([untracked])
    rerender(<BigBoxDetails {...props({ detailsFocus: 'gallery' })} />)
    expect(focused()).toEqual([thumbs()])
    expect(thumbs()).toBeTruthy()
  })

  it('BBX-035/a miniatura selecionada tem aria-current e não tem o contorno de foco', () => {
    const { container } = render(<BigBoxDetails {...props({ detailsFocus: 'gallery', gallery: 1 })} />)
    const items = galleryItems(container) as HTMLElement[]
    expect(items[1].getAttribute('aria-current')).toBe('true')
    expect(items[1].getAttribute('data-focused')).not.toBe('true')
    expect(items[1].style.outline).not.toContain('--arc-bb-focus')
    expect(items.filter((e) => e.getAttribute('aria-current') === 'true')).toHaveLength(1)
  })

  const viewer = (c: HTMLElement) => c.querySelector('[data-bb-viewer]') as HTMLElement
  const fitsContain = (el: Element) => el.className.includes('object-contain') || (el as HTMLElement).style.objectFit === 'contain'

  it('BBX-034/o visor mostra o item gallery em object-contain e o contador "i+1 / N"', () => {
    const { container } = render(<BigBoxDetails {...props({ gallery: 1 })} />)
    const v = viewer(container)
    expect(v).toBeTruthy()
    const img = v.querySelector('img') as HTMLImageElement
    expect(img.getAttribute('src')).toBe('/media/1.png') // galleryOf: video 2, screenshot 1, screenshot 3
    expect(fitsContain(img)).toBe(true)
    expect(v.textContent).toContain('2 / 3')
  })

  it('BBX-034/gallery 2 mostra o terceiro screenshot no visor', () => {
    const { container } = render(<BigBoxDetails {...props({ gallery: 2 })} />)
    expect(viewer(container).querySelector('img')?.getAttribute('src')).toBe('/media/3.png')
    expect(viewer(container).textContent).toContain('3 / 3')
  })

  it('BBX-034/gallery maior que N − 1 mostra o último', () => {
    const { container } = render(<BigBoxDetails {...props({ gallery: 9 })} />)
    expect(viewer(container).querySelector('img')?.getAttribute('src')).toBe('/media/3.png')
    expect(viewer(container).textContent).toContain('3 / 3')
  })

  it('BBX-034/vídeo selecionado toca no visor (muted sem som, loop, playsInline) e a miniatura dele não tem video', () => {
    const { container } = render(<BigBoxDetails {...props({ gallery: 0 })} />)
    const v = viewer(container).querySelector('video') as HTMLVideoElement
    expect(v).not.toBeNull()
    expect(v.getAttribute('src')).toBe('/media/2.mp4')
    expect(v.muted).toBe(true)
    expect(v.loop).toBe(true)
    expect(v.hasAttribute('playsinline') || v.playsInline).toBe(true)
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalled()
    const thumb = galleryItems(container)[0]
    expect(thumb.querySelector('video')).toBeNull()
    expect(thumb.textContent).toContain('Vídeo')
  })

  it('BBX-034/trocar do vídeo para um screenshot desmonta o video sem src', () => {
    const { container, rerender } = render(<BigBoxDetails {...props({ gallery: 0 })} />)
    rerender(<BigBoxDetails {...props({ gallery: 1 })} />)
    const after = container.querySelector('video')
    expect(after === null || !after.hasAttribute('src')).toBe(true)
  })

  it('BBX-034/no máximo um video na tela', () => {
    const { container, rerender } = render(<BigBoxDetails {...props({ gallery: 0 })} />)
    for (const g of [0, 1, 2, 0]) {
      rerender(<BigBoxDetails {...props({ gallery: g })} />)
      expect(container.querySelectorAll('video').length).toBeLessThanOrEqual(1)
    }
  })

  it('BBX-034/a troca de gallery chama scrollIntoView da miniatura (espião) sem quebrar quando ausente', () => {
    const proto = HTMLElement.prototype as unknown as { scrollIntoView?: unknown }
    const spy = vi.fn()
    proto.scrollIntoView = spy
    try {
      const { rerender } = render(<BigBoxDetails {...props({ gallery: 0 })} />)
      spy.mockClear()
      rerender(<BigBoxDetails {...props({ gallery: 2 })} />)
      expect(spy).toHaveBeenCalledWith({ inline: 'nearest', block: 'nearest' })
      expect(spy.mock.instances.at(-1)).toBe(galleryItems(document.body)[2])
    } finally {
      delete proto.scrollIntoView
    }
    const { rerender: again } = render(<BigBoxDetails {...props({ gallery: 0 })} />)
    expect(() => again(<BigBoxDetails {...props({ gallery: 1 })} />)).not.toThrow()
  })

  it('BBX-041/miniaturas em object-contain', () => {
    const { container } = render(<BigBoxDetails {...props()} />)
    const imgs = galleryItems(container).flatMap((e) => Array.from(e.querySelectorAll('img')))
    expect(imgs.length).toBeGreaterThan(0)
    for (const i of imgs) expect(fitsContain(i)).toBe(true)
  })

  it('BBX-037/título, linhas de dado, tempo, botões, descrição e contador têm ancestral data-bb-panel', () => {
    const { container } = render(<BigBoxDetails {...props({ gallery: 1 })} />)
    const inPanel = (el: Element | null) => el?.closest('[data-bb-panel]') != null
    expect(inPanel(screen.getByRole('heading', { name: 'Chrono Quest' }))).toBe(true)
    expect(inPanel(screen.getByText(/Squaresoft/))).toBe(true)
    expect(inPanel(screen.getByText(/Enix/))).toBe(true)
    expect(inPanel(screen.getByText(/Sessões: 4/))).toBe(true)
    expect(inPanel(screen.getByRole('button', { name: 'Jogar' }))).toBe(true)
    expect(inPanel(screen.getByRole('button', { name: 'Jogar sem contar tempo' }))).toBe(true)
    expect(inPanel(screen.getByRole('region', { name: /descrição/i }))).toBe(true)
    const counter = Array.from(viewer(container).querySelectorAll('*')).find((e) => e.children.length === 0 && /2 \/ 3/.test(e.textContent ?? ''))
    expect(counter).toBeTruthy()
    expect(inPanel(counter ?? null)).toBe(true)
  })

  it('BBX-043/a raiz data-bb-details tem altura 100%, min-height 0 e overflow hidden; visor e miniaturas estão dentro dela; só a descrição tem overflow-y auto', () => {
    const { container } = render(<BigBoxDetails {...props()} />)
    const root = container.querySelector('[data-bb-details]') as HTMLElement
    expect(root).toBeTruthy()
    expect(root.style.height).toBe('100%')
    expect(['0', '0px']).toContain(root.style.minHeight)
    expect(root.style.overflow).toBe('hidden')
    expect(root.contains(viewer(container))).toBe(true)
    expect(root.contains(container.querySelector('[data-bb-thumbs]'))).toBe(true)
    const scrollers = Array.from(root.querySelectorAll<HTMLElement>('*')).filter((e) => e.style.overflowY === 'auto')
    expect(scrollers).toEqual([screen.getByRole('region', { name: /descrição/i })])
  })

  it('BBX-043/sem galeria não há visor nem miniaturas e a coluna de texto vai a 60%', () => {
    const { container } = render(<BigBoxDetails {...props({ media: [item(4, 'cover')] })} />)
    expect(viewer(container)).toBeNull()
    expect(container.querySelector('[data-bb-thumbs]')).toBeNull()
    const column = screen.getByRole('heading', { name: 'Chrono Quest' }).closest('[data-bb-panel]') as HTMLElement
    expect(column.style.maxWidth).toBe('60%')
  })

  const em = (v: string) => (v === '' ? NaN : parseFloat(v))

  it('BBX-044/data-bb-facts agrupa plataforma·ano·gêneros, desenvolvedores, editoras, modos e tempo·última vez·sessões; rowGap ≤ 0,25em, lineHeight ≤ 1,4 e parágrafos sem margem', () => {
    const { container } = render(<BigBoxDetails {...props()} />)
    const facts = container.querySelector('[data-bb-facts]') as HTMLElement
    expect(facts).toBeTruthy()
    const t = facts.textContent ?? ''
    for (const s of ['Super Nintendo', '1995', 'RPG', 'Desenvolvedores: Squaresoft', 'Editoras: Enix', 'Um jogador', 'Sessões: 4']) {
      expect(t).toContain(s)
    }
    expect(em(facts.style.rowGap)).toBeLessThanOrEqual(0.25)
    expect(em(facts.style.lineHeight)).toBeLessThanOrEqual(1.4)
    const ps = Array.from(facts.querySelectorAll('p'))
    expect(ps.length).toBeGreaterThanOrEqual(5)
    for (const p of ps) expect(['0', '0px']).toContain(p.style.margin)
  })

  it('BBX-044/o gap entre título, fatos, botões e descrição é ≤ 0,75em', () => {
    render(<BigBoxDetails {...props()} />)
    const column = screen.getByRole('heading', { name: 'Chrono Quest' }).closest('[data-bb-panel]') as HTMLElement
    expect(em(column.style.gap)).toBeLessThanOrEqual(0.75)
  })

  it('BBX-016/scrollSignal rola a descrição', () => {
    const scrollBy = vi.fn()
    ;(HTMLElement.prototype as unknown as { scrollBy: unknown }).scrollBy = scrollBy
    const { rerender } = render(<BigBoxDetails {...props({ scrollSignal: 0 })} />)
    const before = scrollBy.mock.calls.length
    act(() => {
      rerender(<BigBoxDetails {...props({ scrollSignal: 1 })} />)
    })
    expect(scrollBy.mock.calls.length).toBeGreaterThan(before)
    const el = screen.getByRole('region', { name: /descrição/i })
    expect(scrollBy.mock.instances.at(-1)).toBe(el)
    act(() => {
      rerender(<BigBoxDetails {...props({ scrollSignal: -1 })} />)
    })
    const last = scrollBy.mock.calls.at(-1)?.[0] as { top?: number } | number | undefined
    const top = typeof last === 'object' ? last?.top : undefined
    expect(top === undefined || top < 0).toBe(true)
  })

  it('BBX-036/scrollSignal rola a descrição com o foco em qualquer zona', () => {
    const scrollBy = vi.fn()
    ;(HTMLElement.prototype as unknown as { scrollBy: unknown }).scrollBy = scrollBy
    for (const detailsFocus of ['play', 'playUntracked', 'gallery'] as const) {
      const { rerender, unmount } = render(<BigBoxDetails {...props({ detailsFocus, scrollSignal: 0 })} />)
      scrollBy.mockClear()
      act(() => {
        rerender(<BigBoxDetails {...props({ detailsFocus, scrollSignal: 1 })} />)
      })
      expect(scrollBy, detailsFocus).toHaveBeenCalled()
      expect(scrollBy.mock.instances.at(-1)).toBe(screen.getByRole('region', { name: /descrição/i }))
      unmount()
    }
  })

  it('TTB-016/mostra "Tempo para zerar" com os três rótulos e valores, num painel, e a linha da fonte', () => {
    render(<BigBoxDetails {...props()} />)
    const heading = screen.getByText('Tempo para zerar')
    expect(heading.closest('[data-bb-panel]')).not.toBeNull()
    for (const [label, value] of [['História principal', '20 h'], ['História + extras', '31½ h'], ['100%', '41 h']]) {
      const l = screen.getByText(label)
      expect(l.closest('[data-bb-panel]'), label).not.toBeNull()
      expect(screen.getByText(value).closest('[data-bb-panel]'), value).not.toBeNull()
    }
    expect(screen.getByText('Fonte: HowLongToBeat').closest('[data-bb-panel]')).not.toBeNull()
  })

  it('TTB-016/não há botão de atualizar nem de confirmar nos Detalhes', () => {
    render(<BigBoxDetails {...props()} />)
    expect(screen.queryByRole('button', { name: /atualizar|confirmar|escolher|buscar/i })).toBeNull()
    expect(screen.getAllByRole('button').map((b) => b.textContent)).toEqual(['Jogar', 'Jogar sem contar tempo'])
  })

  it('TTB-039/fonte IGDB: "Fonte: IGDB"; fonte HowLongToBeat: "Fonte: HowLongToBeat"', () => {
    const { rerender } = render(<BigBoxDetails {...props({ timeToBeatSource: 'IGDB' })} />)
    expect(screen.getByText('Fonte: IGDB')).toBeInTheDocument()
    expect(screen.queryByText('Fonte: HowLongToBeat')).toBeNull()
    rerender(<BigBoxDetails {...props({ timeToBeatSource: 'HowLongToBeat' })} />)
    expect(screen.getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
  })

  it('TTB-013/sem dado os três rótulos ficam com "—", sem linha da fonte, sem role alert', () => {
    render(<BigBoxDetails {...props({ timeToBeat: TTB_EMPTY, timeToBeatSource: null })} />)
    expect(screen.getByText('Tempo para zerar')).toBeInTheDocument()
    for (const l of ['História principal', 'História + extras', '100%']) expect(screen.getByText(l)).toBeInTheDocument()
    expect(screen.getAllByText('—')).toHaveLength(3)
    expect(screen.queryByText(/Fonte:/)).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('TTB-016/o bloco do tempo para zerar fica fora de data-bb-facts e não cria rolagem nova (BBX-043)', () => {
    const { container } = render(<BigBoxDetails {...props()} />)
    const facts = container.querySelector('[data-bb-facts]') as HTMLElement
    expect(facts.textContent).not.toContain('Tempo para zerar')
    const root = container.querySelector('[data-bb-details]') as HTMLElement
    const scrollers = Array.from(root.querySelectorAll<HTMLElement>('*')).filter((e) => e.style.overflowY === 'auto')
    expect(scrollers).toEqual([screen.getByRole('region', { name: /descrição/i })])
  })

  it('BBX-016/jogo sem descrição e sem mídia não quebra', () => {
    const bare: GameDetailView = { ...game, description: '', genres: [], developers: [], publishers: [], playModes: [], year: null, lastPlayedAt: '', playCount: 0, totalPlaySeconds: 0 }
    const { container } = render(<BigBoxDetails {...props({ game: bare, media: [], session: 'running' })} />)
    expect(screen.getByRole('heading', { name: 'Chrono Quest' })).toBeTruthy()
    expect(galleryItems(container)).toHaveLength(0)
    expect(within(container).getByRole('button', { name: 'Jogar' })).toBeTruthy()
  })
})
