// TDK-037 (T12): the Desktop panel has exactly two buttons, "Jogar" then "Jogar sem contar tempo", and nothing else actionable.
// Same assumed markup as DesktopDetails.test.tsx (aside[aria-label="Detalhes do jogo"], viewer region "Imagens", thumbs not buttons).
// Not covered here: "credenciais do ScreenScraper prontas ou não" has no prop on BigBoxDetailsProps (the panel never reads it),
// so it is a no-op axis of the matrix (reported as spec gap).
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { desktopLabels } from '../../desktopLabels'
import type { GameDetailView } from '../../../lib/library'
import type { MediaItemView } from '../../../lib/metadata'
import type { BigBoxDetailsProps } from '../../BigBoxDetails'
import type { BigBoxViewModel } from '../../viewModel'
import { Desktop } from './Desktop'
import { DesktopDetails } from './DesktopDetails'

const ACTIONABLE = 'button, a[href], input, select, textarea, [role="button"], [role="link"], [role="menuitem"], [contenteditable]'
const base: GameDetailView = {
  id: 7, platformId: 2, title: 'Chrono Quest', platform: 'Super Nintendo', path: 'C:\\g\\cq.sfc', status: 'not_played' as GameDetailView['status'],
  favorite: false, year: 1995, totalPlaySeconds: 7260, playCount: 4, firstPlayedAt: '2024-01-01T10:00:00Z', lastPlayedAt: '2026-11-14T12:00:00Z',
  description: 'Descricao.', emulatorId: null, launchType: 'exe', launchArgs: '', sessionPlaySeconds: 0, completedAt: null,
  developers: ['Squaresoft'], publishers: ['Enix'], genres: ['RPG'], playModes: ['single'],
}
const item = (id: number, kind: MediaItemView['kind']): MediaItemView => ({
  id, kind, provider: 'igdb', path: `p${id}`, thumbPath: `t${id}`, url: `/media/${id}.${kind === 'video' ? 'mp4' : 'png'}`, thumbUrl: `/media/t${id}.png`,
})
const TTB = [{ label: 'História principal', value: '20 h' }, { label: '100%', value: '41 h' }]
function props(over: Partial<BigBoxDetailsProps> = {}): BigBoxDetailsProps {
  const g = over.game ?? base
  return {
    game: g, media: [], session: null, detailsFocus: 'play', gallery: 0, sound: false, scrollSignal: 0, onPlay: vi.fn(), onPlayUntracked: vi.fn(),
    timeToBeat: TTB, timeToBeatSource: 'HowLongToBeat', active: true, platformIconUrl: '/i/snes.png', coverUrl: 'http://m/c.jpg',
    labels: desktopLabels(g), preview: { gameId: g.id, videoUrl: null, active: true, failed: false, sound: false, onVideoError: vi.fn() }, ...over,
  }
}
const MATRIX: Array<[string, Partial<BigBoxDetailsProps>]> = [
  ['com vídeo e screenshots', { media: [item(1, 'video'), item(2, 'screenshot'), item(3, 'screenshot')] }],
  ['sem vídeo, uma screenshot', { media: [item(2, 'screenshot')] }],
  ['sem mídia', { media: [] }],
  ['sessão aberta (running)', { session: 'running' }],
  ['sessão pausada', { session: 'paused' }],
  ['favorito e concluído', { game: { ...base, favorite: true, completedAt: '2025-03-04T00:00:00Z' } }],
  ['não favorito e não concluído', { game: { ...base, favorite: false, completedAt: null } }],
  ['tempo para zerar em cache', { timeToBeat: TTB, timeToBeatSource: 'HowLongToBeat' }],
  ['tempo para zerar pendente', { timeToBeat: [], timeToBeatSource: null }],
  ['tempo para zerar ausente', { timeToBeat: [], timeToBeatSource: null, game: { ...base, id: 9 } }],
  ['emulador sem emulador configurado', { game: { ...base, launchType: 'emulator', emulatorId: null } }],
]
const FOCI: BigBoxDetailsProps['detailsFocus'][] = ['play', 'gallery']
const aside = (c: HTMLElement) => c.querySelector('aside[aria-label="Detalhes do jogo"]') as HTMLElement

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe.each(MATRIX)('DesktopDetails botões: %s', (_name, over) => {
  describe.each([false, true])('active = %s', (active) => {
    for (const detailsFocus of active ? FOCI : (['play'] as const)) {
      const label = `active=${active} foco=${detailsFocus}`
      it(`TDK-037/o aside tem exatamente dois botões, "Jogar" e "Jogar sem contar tempo", nesta ordem (${label})`, () => {
        const { container } = render(<DesktopDetails {...props({ ...over, active, detailsFocus })} />)
        const names = within(aside(container)).getAllByRole('button').map((b) => b.textContent?.trim())
        expect(names).toEqual(['Jogar', 'Jogar sem contar tempo'])
      })

      it(`TDK-037/a busca pelos seletores acionáveis devolve só esses dois; visor e miniaturas não são botões (${label})`, () => {
        const { container } = render(<DesktopDetails {...props({ ...over, active, detailsFocus })} />)
        const found = Array.from(aside(container).querySelectorAll(ACTIONABLE))
        expect(found.map((e) => e.textContent?.trim())).toEqual(['Jogar', 'Jogar sem contar tempo'])
        const viewer = screen.getByRole('region', { name: 'Imagens' })
        expect(viewer.querySelector(ACTIONABLE)).toBeNull()
        expect(viewer.matches(ACTIONABLE)).toBe(false)
        viewer.querySelectorAll('[data-gallery-item]').forEach((t) => expect(t.matches(ACTIONABLE)).toBe(false))
      })

      it(`TDK-037/Jogar chama só onPlay e Jogar sem contar tempo só onPlayUntracked (${label})`, () => {
        const onPlay = vi.fn()
        const onPlayUntracked = vi.fn()
        const { container } = render(<DesktopDetails {...props({ ...over, active, detailsFocus, onPlay, onPlayUntracked })} />)
        fireEvent.click(within(aside(container)).getByRole('button', { name: 'Jogar' }))
        expect(onPlay).toHaveBeenCalledTimes(1)
        expect(onPlayUntracked).not.toHaveBeenCalled()
        fireEvent.click(within(aside(container)).getByRole('button', { name: 'Jogar sem contar tempo' }))
        expect(onPlayUntracked).toHaveBeenCalledTimes(1)
        expect(onPlay).toHaveBeenCalledTimes(1)
      })
    }
  })
})

describe('DesktopDetails botões: painel sem detalhe', () => {
  const empty = (focused: BigBoxViewModel['focused']) =>
    ({
      screen: 'browse', tabs: [{ key: 'all', label: 'Todos', filter: { platformId: null }, iconUrl: null, count: 1 }], tab: 0, loading: false,
      games: focused ? [focused.game] : [], total: focused ? 1 : 0, focus: 0, stripStart: 0, counter: null, focused, axis: 'grid', columns: 6,
      tabTotal: 1, sortLabel: 'Título', videoFailed: false, videoVisible: false, sound: false, hints: [], alert: null, layout: 'columns',
      zone: 'games', view: 'grid',
    }) as unknown as BigBoxViewModel
  const run = (m: BigBoxViewModel) =>
    render(<Desktop model={m} details={null} onPick={vi.fn()} onPlay={vi.fn()} onDetails={vi.fn()} onVideoError={vi.fn()} onTab={vi.fn()} />)

  it('TDK-037/"Nenhum jogo selecionado" não tem nenhum botão', () => {
    const { container } = run(empty(null))
    expect(aside(container)).toHaveTextContent('Nenhum jogo selecionado')
    expect(aside(container).querySelectorAll(ACTIONABLE)).toHaveLength(0)
  })

  it('TDK-037/"Carregando jogo…" não tem nenhum botão', () => {
    const game = { id: 1, title: 'Jogo 1', coverUrl: 'http://m/1.jpg', session: null }
    const { container } = run(empty({ game, platform: 'N64', year: 2000, genre: '', shortDescription: '' } as unknown as BigBoxViewModel['focused']))
    expect(aside(container)).toHaveTextContent('Carregando jogo…')
    expect(aside(container).querySelectorAll(ACTIONABLE)).toHaveLength(0)
  })
})
