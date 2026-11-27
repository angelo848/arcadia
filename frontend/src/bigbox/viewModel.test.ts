import { describe, expect, it } from 'vitest'
import {
  buildTabs, buildViewModel, centeredRow, edgeScrollRow, effectiveRows, gridWindow, gridWindowAt, infoLine, pickBackground, pickVideo, relativeDays, sessionMark, shortDescription,
  stripCounter, stripWindow,
  GRID_MAX_NODES, GRID_MAX_ROWS, SHORT_DESCRIPTION_MAX, STRIP_ANCHOR, STRIP_FALLBACK_CAPACITY, STRIP_MAX,
} from './viewModel'
import type { ViewModelInput } from './viewModel'
import type { CatalogView, GameView } from '../lib/library'
import type { MediaItemView } from '../lib/metadata'
import type { SessionView } from '../lib/games'
import { formatPlayTime } from '../lib/games'
import { genericIconUrl, seedIconUrls } from '../lib/platformIcons'

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

// fase 17 (T05): janela por tema, rolagem pelas bordas, abas com ícone, rótulo herdado e campos do layout columns.
describe('viewModel (fase 17)', () => {
  it.each([
    [1, 5, 30, 1],
    [9, 5, 30, 5],
    [0, 5, 30, 1],
    [3, 40, 30, 1],
    [2, 6, 200, 2],
    [6, 17, 200, 6],
    [13, 1, 200, 13],
  ])('BBX-062/effectiveRows(R %i, C %i, N %i) = %i', (r, c, n, want) => {
    expect(effectiveRows(r, c, n)).toBe(want)
  })

  it('BBX-062/effectiveRows sem N vale 30', () => {
    expect(effectiveRows(9, 5)).toBe(effectiveRows(9, 5, 30))
    expect(effectiveRows(9, 5)).toBe(5)
  })

  it.each([
    [23, 0, 5, 1, 0, 10],
    [23, 4, 5, 2, 20, 23],
    [60, 0, 6, 2, 0, 18],
    [60, 3, 6, 2, 18, 36],
    [20, 1, 6, 2, 6, 20],
    [500, 0, 17, 6, 0, 119],
    [200, 0, 1, 13, 0, 14],
  ])('BBX-062/gridWindowAt(T %i, s %i, C %i, R %i) = [%i, %i)', (t, s, c, r, start, end) => {
    expect(gridWindowAt(t, s, c, r)).toEqual({ start, end })
  })

  it('BBX-062/gridWindowAt com T = 0 devolve {0, 0}', () => {
    expect(gridWindowAt(0, 0, 6, 2)).toEqual({ start: 0, end: 0 })
    expect(gridWindowAt(0, 3, 6, 2)).toEqual({ start: 0, end: 0 })
  })

  it('BBX-062/nunca desenha mais de (R + 1)·C cartões nem passa de N = 200', () => {
    for (const [c, r] of [[6, 2], [17, 6], [1, 13], [4, 1]]) {
      const rr = effectiveRows(r, c, 200)
      for (let s = 0; s < 40; s += 3) {
        const w = gridWindowAt(500, s, c, rr)
        expect(w.end - w.start).toBeLessThanOrEqual((rr + 1) * c)
        expect((rr + 1) * c).toBeLessThanOrEqual(200)
        expect(w.end).toBeLessThanOrEqual(500)
      }
    }
  })

  it('BBX-062/gridWindow com maxNodes 200 usa o limite do tema', () => {
    expect(gridWindow(60, 0, 6, 2, 200)).toEqual({ start: 0, end: 18 })
    expect(gridWindow(500, 0, 17, 6, 200)).toEqual({ start: 0, end: 119 })
  })

  it('BBX-049/gridWindow sem maxNodes continua igual (rede de segurança)', () => {
    expect(gridWindow(23, 7, 5, 1)).toEqual({ start: 5, end: 15 })
    expect(gridWindow(23, 22, 5, 2)).toEqual({ start: 15, end: 23 })
  })

  it.each([
    [0, 0, 2, 10, 0],
    [0, 1, 2, 10, 0],
    [0, 2, 2, 10, 1],
    [3, 2, 2, 10, 2],
    [0, 9, 2, 10, 8],
    [8, 9, 2, 10, 8],
    [0, 1, 1, 5, 1],
    [0, 1, 3, 2, 0],
  ])('BBX-063/edgeScrollRow(prev %i, r %i, R %i, rows %i) = %i', (prev, r, vr, rows, want) => {
    expect(edgeScrollRow(prev, r, vr, rows)).toBe(want)
  })

  it('BBX-063/edgeScrollRow fica limitado a [0, max(rows - R, 0)]', () => {
    for (const rows of [0, 1, 3, 10]) {
      for (let prev = 0; prev < 12; prev++) {
        for (let r = 0; r < 12; r++) {
          const s = edgeScrollRow(prev, r, 3, rows)
          expect(s).toBeGreaterThanOrEqual(0)
          expect(s).toBeLessThanOrEqual(Math.max(rows - 3, 0))
        }
      }
    }
  })

  it.each([
    [5, 3, 10, 4],
    [0, 3, 10, 0],
    [9, 3, 10, 7],
    [4, 2, 10, 4],
  ])('BBX-063/centeredRow(r %i, R %i, rows %i) = %i', (r, vr, rows, want) => {
    expect(centeredRow(r, vr, rows)).toBe(want)
  })

  const pl = (id: number, name: string, gameCount: number, seedKey: string | null, iconUrl: string | null) => ({
    id, name, category: '', emulatorId: null, seedKey, iconUrl, gameCount,
  })
  const cat = (ps: ReturnType<typeof pl>[]) => ({ platforms: ps, genres: [], companies: [], playModes: [] }) as unknown as CatalogView

  it('BBX-066/buildTabs com withIcons: ícone personalizado, embutido ou genérico e count = gameCount', () => {
    const c = cat([pl(1, 'A', 4, null, 'custom.png'), pl(2, 'B', 6, 'n64', null), pl(3, 'C', 2, null, null), pl(4, 'D', 0, 'snes', null)])
    const { tabs } = buildTabs(c, null, 'platforms', true)
    expect(tabs.map((t) => t.key)).toEqual(['all', 'p:1', 'p:2', 'p:3'])
    expect(tabs[1]).toMatchObject({ iconUrl: 'custom.png', count: 4 })
    expect(tabs[2]).toMatchObject({ iconUrl: seedIconUrls.n64, count: 6 })
    expect(tabs[3]).toMatchObject({ iconUrl: genericIconUrl, count: 2 })
  })

  it('BBX-066/"Todos" soma as plataformas com jogos', () => {
    const c = cat([pl(1, 'A', 4, null, null), pl(2, 'B', 6, null, null), pl(3, 'C', 0, null, null)])
    expect(buildTabs(c, null, 'platforms', true).tabs[0].count).toBe(10)
  })

  it('BBX-066/sem withIcons as abas são iguais às de hoje (toEqual)', () => {
    const c = cat([pl(1, 'A', 4, null, 'custom.png'), pl(2, 'B', 6, 'n64', null)])
    const want = [
      { key: 'all', label: 'Todos', filter: { platformId: null } },
      { key: 'p:1', label: 'A', filter: { platformId: 1 } },
      { key: 'p:2', label: 'B', filter: { platformId: 2 } },
    ]
    expect(buildTabs(c, null, 'platforms').tabs).toEqual(want)
    expect(buildTabs(c, null, 'platforms', false).tabs).toEqual(want)
  })

  const gv2 = (id: number, o: Record<string, unknown> = {}) =>
    ({ id, title: `J${id}`, platform: 'N64', year: 1996, totalPlaySeconds: 0, lastPlayedAt: '', playCount: 0, ...o }) as unknown as GameView
  const base = (o: Partial<ViewModelInput> = {}): ViewModelInput =>
    ({
      axis: 'grid', columns: 6, sort: 'title', now: new Date('2026-11-22T12:00:00Z'), screen: 'browse',
      tabs: [{ key: 'all', label: 'Todos', filter: { platformId: null } }], tab: 0,
      games: Array.from({ length: 60 }, (_, i) => gv2(i + 1)), total: 60, loaded: 60, focus: 0, capacity: 12, loading: false,
      covers: {}, media: [], description: '', genres: null, session: null, sound: false, videoFailed: null, alert: null,
      ...o,
    }) as ViewModelInput

  it.each([
    ['title', 'Título'],
    ['lastPlayed', 'Última vez jogado'],
    ['totalPlayed', 'Tempo jogado'],
    ['year', 'Ano'],
  ] as const)('BBX-067/sortLabel do modelo com sortInherited e %s = %s', (k, want) => {
    expect(buildViewModel(base({ sort: k, sortInherited: true })).sortLabel).toBe(want)
  })

  it('BBX-067/sem herança sortLabel continua "Título" e "Jogado recentemente"', () => {
    expect(buildViewModel(base({ sort: 'title' })).sortLabel).toBe('Título')
    expect(buildViewModel(base({ sort: 'lastPlayed' })).sortLabel).toBe('Jogado recentemente')
  })

  it('BBX-068/o modelo repassa layout, zone, view e columns', () => {
    const m = buildViewModel(base({ layout: 'columns', zone: 'platforms', view: 'list', columns: 1 }))
    expect(m).toMatchObject({ layout: 'columns', zone: 'platforms', view: 'list', columns: 1 })
  })

  it('BBX-068/sem opções o modelo é screens, games e grid', () => {
    expect(buildViewModel(base())).toMatchObject({ layout: 'screens', zone: 'games', view: 'grid' })
  })

  it('TDK-005/TDK-006/com layout columns os jogos trazem coverFit, platform, playTimeText e lastPlayedText', () => {
    const games = [gv2(1, { totalPlaySeconds: 3600, lastPlayedAt: '2026-11-15T12:00:00Z' }), gv2(2)]
    const covers = { 1: { url: 'c1', fit: 'cover' as const }, 2: { url: 'c2', fit: 'contain' as const } }
    const m = buildViewModel(base({ layout: 'columns', games, loaded: 2, total: 2, covers }))
    expect(m.games[0]).toMatchObject({ coverFit: 'cover', platform: 'N64', playTimeText: formatPlayTime(3600) })
    expect(m.games[0].lastPlayedText).toBeTruthy()
    expect(m.games[0].lastPlayedText).not.toBe('Nunca')
    expect(m.games[1]).toMatchObject({ coverFit: 'contain', platform: 'N64', lastPlayedText: 'Nunca' })
  })

  it('TDK-005/sem layout columns esses campos não existem', () => {
    const m = buildViewModel(base({ games: [gv2(1)], loaded: 1, total: 1, covers: { 1: { url: 'c1', fit: 'cover' } } }))
    for (const k of ['coverFit', 'platform', 'playTimeText', 'lastPlayedText']) expect(k in m.games[0]).toBe(false)
    expect(m.games[0]).toEqual({ id: 1, title: 'J1', coverUrl: 'c1', session: null, playTimeLabel: 'Nunca jogado' })
  })

  it('TDK-019/buildViewModel com startRow usa gridWindowAt e stripStart = startRow·C', () => {
    const m = buildViewModel(base({ layout: 'columns', startRow: 3, focus: 20, capacity: 12 }))
    expect(m.stripStart).toBe(18)
    expect(m.games.map((g) => g.id)).toEqual(Array.from({ length: 18 }, (_, i) => i + 19))
  })

  it('TDK-004/total e rótulos do modelo', () => {
    const m = buildViewModel(base({ layout: 'columns', total: 60, sort: 'totalPlayed', sortInherited: true }))
    expect(m.total).toBe(60)
    expect(m.tabTotal).toBe(60)
    expect(m.sortLabel).toBe('Tempo jogado')
  })
})
