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

  it('BBX-016/os botões Jogar e Jogar sem contar tempo chamam onPlay e onPlayUntracked; o foco atual tem contorno', () => {
    const p = props({ detailsFocus: 'playUntracked' })
    const { rerender } = render(<BigBoxDetails {...p} />)
    const play = screen.getByRole('button', { name: 'Jogar' })
    const untracked = screen.getByRole('button', { name: 'Jogar sem contar tempo' })
    fireEvent.click(play)
    fireEvent.click(untracked)
    expect(p.onPlay).toHaveBeenCalledTimes(1)
    expect(p.onPlayUntracked).toHaveBeenCalledTimes(1)
    expect(untracked.getAttribute('data-focused')).toBe('true')
    expect(play.getAttribute('data-focused')).not.toBe('true')
    rerender(<BigBoxDetails {...p} detailsFocus="play" />)
    expect(play.getAttribute('data-focused')).toBe('true')
    expect(untracked.getAttribute('data-focused')).not.toBe('true')
    rerender(<BigBoxDetails {...p} detailsFocus="description" />)
    expect(screen.getByRole('region', { name: /descrição/i }).getAttribute('data-focused')).toBe('true')
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

  it('BBX-016/jogo sem descrição e sem mídia não quebra', () => {
    const bare: GameDetailView = { ...game, description: '', genres: [], developers: [], publishers: [], playModes: [], year: null, lastPlayedAt: '', playCount: 0, totalPlaySeconds: 0 }
    const { container } = render(<BigBoxDetails {...props({ game: bare, media: [], session: 'running' })} />)
    expect(screen.getByRole('heading', { name: 'Chrono Quest' })).toBeTruthy()
    expect(galleryItems(container)).toHaveLength(0)
    expect(within(container).getByRole('button', { name: 'Jogar' })).toBeTruthy()
  })
})
