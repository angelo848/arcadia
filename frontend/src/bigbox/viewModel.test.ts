import { describe, expect, it } from 'vitest'
import {
  buildTabs, buildViewModel, gridWindow, infoLine, pickBackground, pickVideo, relativeDays, sessionMark, shortDescription,
  stripCounter, stripWindow,
  GRID_MAX_NODES, GRID_MAX_ROWS, SHORT_DESCRIPTION_MAX, STRIP_ANCHOR, STRIP_FALLBACK_CAPACITY, STRIP_MAX,
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
  // alterado em 2026-11-22 (fase 15): buildTabs(catalog, initialPlatformId, kind) devolve { tabs, initial } com
  // BigBoxTab { key, label, filter } (temas-v2 §2).
  it('BBX-004/Todos e só as plataformas com gameCount > 0, na ordem do catálogo, só texto', () => {
    const { tabs } = buildTabs(catalog([plat(3, 'SNES', 2), plat(1, 'N64', 0), plat(2, 'PS1', 5)]), null, 'platforms')
    expect(tabs).toEqual([
      { key: 'all', label: 'Todos', filter: { platformId: null } },
      { key: 'p:3', label: 'SNES', filter: { platformId: 3 } },
      { key: 'p:2', label: 'PS1', filter: { platformId: 2 } },
    ])
    expect(JSON.stringify(tabs)).not.toContain('x.png')
  })

  it('BBX-004/a aba inicial é a plataforma da coluna esquerda ou Todos', () => {
    const c = catalog([plat(3, 'SNES', 2), plat(2, 'PS1', 5)])
    expect(buildTabs(c, 2, 'platforms').initial).toBe(2)
    expect(buildTabs(c, null, 'platforms').initial).toBe(0)
    expect(buildTabs(c, 99, 'platforms').initial).toBe(0)
    expect(buildTabs(undefined, null, 'platforms').tabs).toEqual([{ key: 'all', label: 'Todos', filter: { platformId: null } }])
  })

  it('BBX-054/tabs collections: Todos, Favoritos, Concluídos e depois as plataformas com jogos', () => {
    const { tabs } = buildTabs(catalog([plat(3, 'SNES', 2), plat(1, 'N64', 0), plat(2, 'PS1', 5)]), null, 'collections')
    expect(tabs.map((t) => t.label)).toEqual(['Todos', 'Favoritos', 'Concluídos', 'SNES', 'PS1'])
    expect(tabs.map((t) => t.key)).toEqual(['all', 'favorites', 'completed', 'p:3', 'p:2'])
    expect(tabs[0].filter).toEqual({ platformId: null })
    expect(tabs[1].filter).toEqual({ platformId: null, favorite: true })
    expect(tabs[2].filter).toEqual({ platformId: null, status: 'completed' })
    expect(tabs[3].filter).toEqual({ platformId: 3 })
  })

  it('BBX-054/aba inicial com collections: a plataforma da janela normal (índice depois das coleções) ou Todos', () => {
    const c = catalog([plat(3, 'SNES', 2), plat(2, 'PS1', 5)])
    expect(buildTabs(c, 2, 'collections').initial).toBe(4)
    expect(buildTabs(c, 3, 'collections').initial).toBe(3)
    expect(buildTabs(c, null, 'collections').initial).toBe(0)
    expect(buildTabs(c, 99, 'collections').initial).toBe(0)
    expect(buildTabs(undefined, null, 'collections').tabs.map((t) => t.label)).toEqual(['Todos', 'Favoritos', 'Concluídos'])
  })

  it('BBX-039/constantes da janela', () => {
    expect(STRIP_ANCHOR).toBe(2)
    expect(STRIP_MAX).toBe(15)
    expect(STRIP_FALLBACK_CAPACITY).toBe(12)
  })

  it('BBX-049/constantes da grade', () => {
    expect(GRID_MAX_ROWS).toBe(5)
    expect(GRID_MAX_NODES).toBe(30)
  })

  it.each([
    [40, 3, 12, 2, 1, 14],
    [40, 3, 4, 1, 2, 7],
    [40, 0, 4, 1, 0, 5],
    [40, 39, 4, 1, 36, 40],
  ])('BBX-051/stripWindow com âncora (T %i, f %i, V %i, A %i) = [%i, %i)', (t, f, v, a, start, end) => {
    expect(stripWindow(t, f, v, a)).toEqual({ start, end })
  })

  it('BBX-051/sem âncora o padrão continua 2', () => {
    expect(stripWindow(40, 3, 12)).toEqual(stripWindow(40, 3, 12, 2))
    expect(stripWindow(40, 3, 12)).toEqual({ start: 1, end: 14 })
  })

  it.each([
    [23, 0, 5, 1, 0, 10],
    [23, 7, 5, 1, 5, 15],
    [23, 22, 5, 1, 20, 23],
    [23, 22, 5, 2, 15, 23],
    [4, 3, 5, 1, 0, 4],
    [0, 0, 5, 1, 0, 0],
  ])('BBX-049/gridWindow(T %i, f %i, C %i, R %i) = [%i, %i)', (t, f, c, r, start, end) => {
    expect(gridWindow(t, f, c, r)).toEqual({ start, end })
  })

  it('BBX-049/gridWindow nunca passa de 30 cartões, mesmo com R grande, e a fileira do foco é a primeira', () => {
    for (let f = 0; f < 200; f += 7) {
      for (const r of [1, 3, 5, 9, 50]) {
        const w = gridWindow(200, f, 5, r)
        expect(w.end - w.start).toBeLessThanOrEqual(GRID_MAX_NODES)
        expect(w.start % 5).toBe(0)
        expect(w.end).toBeLessThanOrEqual(200)
      }
    }
    expect(gridWindow(200, 52, 5, 1).start).toBe(50)
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
      // alterado em 2026-11-22 (fase 15): tabs no formato novo e campos novos de entrada (axis, columns, sort, now)
      axis: 'horizontal', columns: 1, sort: 'title', now: new Date('2026-11-22T12:00:00Z'),
      screen: 'browse', tabs: [{ key: 'all', label: 'Todos', filter: { platformId: null } }], tab: 0,
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

  it('BBX-056/modelo: eixo, colunas, tabTotal, sortLabel e videoFailed', () => {
    const m = buildViewModel(input({ total: 23, loaded: 23, focus: 3 }))
    expect(m).toMatchObject({ axis: 'horizontal', columns: 1, tabTotal: 23, sortLabel: 'Título', videoFailed: false })
    expect(buildViewModel(input({ sort: 'lastPlayed' })).sortLabel).toBe('Jogado recentemente')
    expect(buildViewModel(input({ loading: true, loaded: 0, total: 0, games: [] })).tabTotal).toBeNull()
  })

  it('BBX-056/modelo: videoFailed é verdadeiro quando o vídeo do jogo em foco já falhou nesta visita', () => {
    const vid = [media('video', 'v1')]
    const ok = buildViewModel(input({ media: vid, videoFailed: null }))
    expect(ok.videoFailed).toBe(false)
    expect(ok.videoVisible).toBe(true)
    const bad = buildViewModel(input({ media: vid, videoFailed: 1 }))
    expect(bad.videoFailed).toBe(true)
    expect(bad.videoVisible).toBe(false)
  })

  it('BBX-056/modelo na grade: games é a janela de gridWindow recortada em L', () => {
    const m = buildViewModel(input({ axis: 'grid', columns: 5, capacity: 5, total: 60, loaded: 25, focus: 7 }))
    expect(m.stripStart).toBe(5)
    expect(m.games.map((g) => g.id)).toEqual(Array.from({ length: 10 }, (_, i) => i + 6))
    const deep = buildViewModel(input({ axis: 'grid', columns: 5, capacity: 5, total: 60, loaded: 25, focus: 24 }))
    expect(deep.stripStart).toBe(20)
    expect(deep.games.map((g) => g.id)).toEqual([21, 22, 23, 24, 25])
  })

  it('BBX-056/focado: favorite, lastPlayedRelative, screenshots e tempo para zerar (3 linhas, "—" sem dado)', () => {
    const fav = { ...gv(1), favorite: true, lastPlayedAt: '2026-11-15T12:00:00Z' } as unknown as GameView
    const f = buildViewModel(input({ games: [fav], loaded: 1, total: 1, focus: 0 })).focused!
    expect(f.favorite).toBe(true)
    expect(f.lastPlayedRelative).toBe('há 7 dias')
    expect(f.screenshots).toEqual([])
    expect(f.timeToBeat.map((r) => r.label)).toEqual(['História principal', 'História + extras', '100%'])
    expect(f.timeToBeat.map((r) => r.value)).toEqual(['—', '—', '—'])
    expect(f.timeToBeatSource).toBeNull()
    const none = buildViewModel(input({ focus: 0 })).focused!
    expect(none.favorite).toBe(false)
    expect(none.lastPlayedRelative).toBe('Nunca jogado')
  })

  it('BBX-056/focado: screenshots são até 3 miniaturas de mídia do tipo screenshot', () => {
    const shots = ['a', 'b', 'c', 'd'].map((u) => ({ ...media('screenshot', `full-${u}`), thumbUrl: `thumb-${u}` }) as MediaItemView)
    const f = buildViewModel(input({ media: [media('cover'), ...shots] })).focused!
    expect(f.screenshots).toEqual(['thumb-a', 'thumb-b', 'thumb-c'])
  })

  const NOW = new Date('2026-11-22T12:00:00Z')
  const ago = (days: number) => new Date(NOW.getTime() - days * 86_400_000).toISOString()
  it.each([
    [0, 'hoje'],
    [1, 'ontem'],
    [2, 'há 2 dias'],
    [7, 'há 7 dias'],
    [29, 'há 29 dias'],
    [60, 'há 2 meses'],
    [90, 'há 3 meses'],
    [330, 'há 11 meses'],
    [360, 'há 1 ano'],
    [730, 'há 2 anos'],
  ])('BBX-056/relativeDays %i dias atrás = %s', (days, want) => {
    expect(relativeDays(ago(days), NOW)).toBe(want)
  })

  it('BBX-056/relativeDays sem data ou inválida é "Nunca jogado"', () => {
    expect(relativeDays(null, NOW)).toBe('Nunca jogado')
    expect(relativeDays('', NOW)).toBe('Nunca jogado')
  })
})
