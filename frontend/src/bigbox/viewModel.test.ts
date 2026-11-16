import { describe, expect, it } from 'vitest'
import {
  buildTabs, buildViewModel, infoLine, pickBackground, pickVideo, sessionMark, shortDescription, stripCounter, stripWindow,
  SHORT_DESCRIPTION_MAX, STRIP_ANCHOR, STRIP_FALLBACK_CAPACITY, STRIP_MAX,
} from './viewModel'
import type { ViewModelInput } from './viewModel'
import type { CatalogView, GameView } from '../lib/library'
import type { MediaItemView } from '../lib/metadata'
import type { SessionView } from '../lib/games'

const plat = (id: number, name: string, gameCount: number) => ({
  id, name, category: '', emulatorId: null, seedKey: null, iconUrl: 'x.png', gameCount,
})
const catalog = (platforms: ReturnType<typeof plat>[]): CatalogView =>
  ({ platforms, genres: [], companies: [], playModes: [] }) as unknown as CatalogView
const media = (kind: string, url = `u-${kind}`) => ({ id: 1, kind, provider: '', path: '', thumbPath: '', url, thumbUrl: '' }) as unknown as MediaItemView
const sess = (gameId: number, state: string) => ({ gameId, state }) as unknown as SessionView

describe('viewModel', () => {
  it('BBX-004/Todos e só as plataformas com gameCount > 0, na ordem do catálogo, só texto', () => {
    const { tabs } = buildTabs(catalog([plat(3, 'SNES', 2), plat(1, 'N64', 0), plat(2, 'PS1', 5)]), null)
    expect(tabs).toEqual([
      { platformId: null, label: 'Todos' },
      { platformId: 3, label: 'SNES' },
      { platformId: 2, label: 'PS1' },
    ])
    expect(JSON.stringify(tabs)).not.toContain('x.png')
  })

  it('BBX-004/a aba inicial é a plataforma da coluna esquerda ou Todos', () => {
    const c = catalog([plat(3, 'SNES', 2), plat(2, 'PS1', 5)])
    expect(buildTabs(c, 2).initialTab).toBe(2)
    expect(buildTabs(c, null).initialTab).toBe(0)
    expect(buildTabs(c, 99).initialTab).toBe(0)
    expect(buildTabs(undefined, null).tabs).toEqual([{ platformId: null, label: 'Todos' }])
  })

  it('BBX-039/constantes da janela', () => {
    expect(STRIP_ANCHOR).toBe(2)
    expect(STRIP_MAX).toBe(15)
    expect(STRIP_FALLBACK_CAPACITY).toBe(12)
  })

  it.each([
    [40, 0, 12, 0, 13],
    [40, 2, 12, 0, 13],
    [40, 3, 12, 1, 14],
    [40, 39, 12, 28, 40],
    [8, 5, 12, 0, 8],
    [0, 0, 12, 0, 0],
    [100, 50, 30, 48, 63],
  ])('BBX-039/stripWindow(T %i, f %i, V %i) = [%i, %i)', (t, f, v, start, end) => {
    expect(stripWindow(t, f, v)).toEqual({ start, end })
  })

  it('BBX-039/a capa em foco fica sempre em posição ≤ V − 1 (varredura de f em 0..T − 1 para T 40 e V 12)', () => {
    for (let f = 0; f < 40; f++) {
      const w = stripWindow(40, f, 12)
      expect(f - w.start).toBeGreaterThanOrEqual(0)
      expect(f - w.start).toBeLessThanOrEqual(11)
      expect(w.end - w.start).toBeLessThanOrEqual(STRIP_MAX)
    }
  })

  const gv = (id: number) =>
    ({ id, title: `J${id}`, platform: 'N64', year: 1996, totalPlaySeconds: 0, lastPlayedAt: '', playCount: 0 }) as unknown as GameView
  const input = (o: Partial<ViewModelInput> = {}): ViewModelInput => {
    const loaded = o.loaded ?? 40
    return {
      screen: 'browse', tabs: [{ platformId: null, label: 'Todos' }], tab: 0,
      games: Array.from({ length: loaded }, (_, i) => gv(i + 1)), total: 40, loaded, focus: 0, capacity: 12, loading: false,
      covers: {}, media: [], description: '', genres: null, session: null, sound: false, videoFailed: null, alert: null,
      ...o,
    } as ViewModelInput
  }

  it('BBX-039/buildViewModel devolve stripStart e games da janela', () => {
    const m = buildViewModel(input({ focus: 20 }))
    expect(m.stripStart).toBe(18)
    expect(m.games).toHaveLength(13)
    expect(m.games[0].id).toBe(19)
    expect(buildViewModel(input({ focus: 0 })).stripStart).toBe(0)
  })

  it('BBX-039/total do modelo é T mesmo com loaded menor (T 120, loaded 15)', () => {
    const m = buildViewModel(input({ total: 120, loaded: 15, focus: 0 }))
    expect(m.total).toBe(120)
  })

  it('BBX-039/janela recortada em loaded quando a página atrasa (nunca item indefinido em games)', () => {
    const m = buildViewModel(input({ total: 120, loaded: 15, focus: 14 }))
    expect(m.games.length).toBe(15 - m.stripStart)
    expect(m.games.every((g) => g !== undefined && g !== null)).toBe(true)
    expect(m.games[m.games.length - 1].id).toBe(15)
  })

  it('BBX-045/stripCounter(120, 6, false) é "7 / 120"; null com loading; null com T 0', () => {
    expect(stripCounter(120, 6, false)).toBe('7 / 120')
    expect(stripCounter(120, 6, true)).toBeNull()
    expect(stripCounter(0, 0, false)).toBeNull()
  })

  it('BBX-045/buildViewModel põe counter com T da aba e só a primeira página lida', () => {
    expect(buildViewModel(input({ total: 120, loaded: 15, focus: 6 })).counter).toBe('7 / 120')
  })

  it('BBX-038/loading verdadeiro: focused null, counter null e lista vazia no modelo', () => {
    const m = buildViewModel(input({ loading: true, loaded: 0, total: 0, games: [] }))
    expect(m.loading).toBe(true)
    expect(m.focused).toBeNull()
    expect(m.counter).toBeNull()
    expect(m.games).toEqual([])
  })

  it('BBX-009/a linha plataforma · ano · gênero omite partes vazias com o separador e usa o primeiro gênero', () => {
    expect(infoLine('N64', 1996, 'Ação')).toBe('N64 · 1996 · Ação')
    expect(infoLine('N64', null, 'Ação')).toBe('N64 · Ação')
    expect(infoLine('N64', 1996, null)).toBe('N64 · 1996')
    expect(infoLine('N64', null, '')).toBe('N64')
    expect(infoLine('N64', 1996, 'Ação, Aventura')).toBe('N64 · 1996 · Ação')
    expect(infoLine('N64', 1996, 'Ação, Aventura')).not.toContain(',')
    expect(infoLine('N64', 1996, 'Ação, Aventura')).not.toMatch(/\s{2}|^·|·$/)
  })

  it('BBX-009/descrição de 280 caracteres passa inteira; maior é cortada no último espaço com "…"; sem descrição, nada', () => {
    expect(SHORT_DESCRIPTION_MAX).toBe(280)
    const exact = 'a'.repeat(280)
    expect(shortDescription(exact)).toBe(exact)
    expect(shortDescription('')).toBe('')
    const long = 'palavra '.repeat(60)
    const out = shortDescription(long)
    expect(out.length).toBeLessThanOrEqual(SHORT_DESCRIPTION_MAX)
    expect(out.endsWith('…')).toBe(true)
    expect(out.endsWith(' …')).toBe(false)
    expect(out.slice(0, -1).endsWith('palavra')).toBe(true)
  })

  it('BBX-010/pickBackground na ordem background, screenshot, title, select, banner, cover; sem nenhuma, null', () => {
    const order = ['background', 'screenshot', 'title', 'select', 'banner', 'cover']
    for (let i = 0; i < order.length; i++) {
      const present = order.slice(i).reverse().map((k) => media(k))
      expect(pickBackground(present)).toBe(`u-${order[i]}`)
    }
    expect(pickBackground([media('video'), media('manual')])).toBeNull()
    expect(pickBackground([])).toBeNull()
  })

  it('BBX-010/pickVideo devolve a url do primeiro vídeo ou null', () => {
    expect(pickVideo([media('cover'), media('video', 'v1'), media('video', 'v2')])).toBe('v1')
    expect(pickVideo([media('cover')])).toBeNull()
  })

  it('BBX-017/sessionMark: running e grace viram running, paused vira paused, outro jogo nada', () => {
    expect(sessionMark(sess(7, 'running'), 7)).toBe('running')
    expect(sessionMark(sess(7, 'grace'), 7)).toBe('running')
    expect(sessionMark(sess(7, 'paused'), 7)).toBe('paused')
    expect(sessionMark(sess(8, 'running'), 7)).toBeNull()
    expect(sessionMark(null, 7)).toBeNull()
  })
})
