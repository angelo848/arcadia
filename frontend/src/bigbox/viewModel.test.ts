import { describe, expect, it } from 'vitest'
import { buildTabs, infoLine, pickBackground, pickVideo, sessionMark, shortDescription, windowAround, SHORT_DESCRIPTION_MAX, STRIP_WINDOW } from './viewModel'
import type { CatalogView } from '../lib/library'
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

  it('BBX-006/windowAround devolve no máximo 15 itens (foco−7..foco+7) nas pontas e no meio', () => {
    expect(STRIP_WINDOW).toBe(7)
    const items = Array.from({ length: 100 }, (_, i) => i)
    const mid = windowAround(items, 50)
    expect(mid.slice).toHaveLength(15)
    expect(mid.offset).toBe(43)
    expect(mid.slice[0]).toBe(43)
    const start = windowAround(items, 2)
    expect(start.offset).toBe(0)
    expect(start.slice).toEqual(items.slice(0, 10))
    const end = windowAround(items, 99)
    expect(end.offset).toBe(92)
    expect(end.slice).toHaveLength(8)
    expect(windowAround([], 0)).toEqual({ slice: [], offset: 0 })
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
