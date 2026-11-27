import { describe, expect, it } from 'vitest'
import { bigBoxNav, initialNav } from './nav'
import type { NavAction, NavCtx, NavState } from './nav'

const ctx = (o: Partial<NavCtx> = {}): NavCtx => ({
  tabs: 4,
  total: 100,
  loaded: 50,
  galleryCount: 3,
  focusedGameId: 7,
  videoVisible: false,
  columns: 1, // alterado em 2026-11-22 (fase 15): NavCtx.columns (BBX-048)
  axis: 'horizontal', // alterado em 2026-11-27 (fase 17): NavCtx.axis e NavCtx.layout (contrato §2)
  layout: 'screens',
  ...o,
})
const st = (o: Partial<NavState> = {}): NavState => ({ ...initialNav, ...o })
const run = (s: NavState, a: NavAction, c: NavCtx = ctx()) => bigBoxNav(s, a, c)

describe('bigBoxNav', () => {
  it('BBX-006/estado inicial', () => {
    expect(initialNav).toMatchObject({ screen: 'browse', tab: 0, focus: 0, suspended: false, sound: true, videoFailed: null, launchedGameId: null })
  })

  it('BBX-006/prev e next movem o foco de um em um e não dão a volta nas pontas', () => {
    expect(run(st({ focus: 5 }), { type: 'next' }).state.focus).toBe(6)
    expect(run(st({ focus: 5 }), { type: 'prev' }).state.focus).toBe(4)
    expect(run(st({ focus: 0 }), { type: 'prev' }).state.focus).toBe(0)
    expect(run(st({ focus: 99 }), { type: 'next' }, ctx({ loaded: 100 })).state.focus).toBe(99)
  })

  it('BBX-006/prev e next em browse não devolvem efeito algum (sem fetchNextPage)', () => {
    expect(run(st({ focus: 30 }), { type: 'next' }).effects).toEqual([])
    expect(run(st({ focus: 49 }), { type: 'next' }).effects).toEqual([])
    expect(run(st({ focus: 45 }), { type: 'prev' }).effects).toEqual([])
  })

  it('BBX-006/next no jogo total − 1 com tudo lido não muda', () => {
    const c = ctx({ total: 20, loaded: 20 })
    const r = run(st({ focus: 19 }), { type: 'next' }, c)
    expect(r.state.focus).toBe(19)
    expect(r.effects).toEqual([])
  })

  it('BBX-007/next no jogo loaded − 1 com loaded < total não muda o foco (página pendente)', () => {
    const c = ctx({ total: 120, loaded: 15 })
    const r = run(st({ focus: 14 }), { type: 'next' }, c)
    expect(r.state.focus).toBe(14)
    expect(r.effects).toEqual([])
  })

  it('BBX-007/com loaded maior, o mesmo next avança', () => {
    const c = ctx({ total: 120, loaded: 30 })
    expect(run(st({ focus: 14 }), { type: 'next' }, c).state.focus).toBe(15)
  })

  it('BBX-005/tabNext e tabPrev circulares e foco 0', () => {
    expect(run(st({ tab: 3, focus: 9 }), { type: 'tabNext' }).state).toMatchObject({ tab: 0, focus: 0 })
    expect(run(st({ tab: 0, focus: 9 }), { type: 'tabPrev' }).state).toMatchObject({ tab: 3, focus: 0 })
    expect(run(st({ tab: 1 }), { type: 'tabNext' }).state.tab).toBe(2)
  })

  it('BBX-014/accept em browse com jogo pede launch; sem jogo não pede', () => {
    expect(run(st(), { type: 'accept' }).effects).toEqual([{ kind: 'launch', gameId: 7 }])
    expect(run(st(), { type: 'accept' }, ctx({ focusedGameId: null })).effects).toEqual([])
  })

  it('BBX-016/details abre com detailsFocus e detailsButton em play e galeria 0', () => {
    const s = run(st({ gallery: 2, detailsFocus: 'gallery', detailsButton: 'playUntracked' }), { type: 'details' }).state
    expect(s).toMatchObject({ screen: 'details', detailsFocus: 'play', detailsButton: 'play', gallery: 0 })
    expect(run(st(), { type: 'details' }, ctx({ focusedGameId: null })).state.screen).toBe('browse')
  })

  const det = (o: Partial<NavState> = {}) => st({ screen: 'details', ...o })

  it('BBX-032/down de um botão com galleryCount > 0 vai à galeria; com galleryCount 0 não muda', () => {
    expect(run(det({ detailsFocus: 'play', detailsButton: 'play' }), { type: 'down' }).state.detailsFocus).toBe('gallery')
    expect(run(det({ detailsFocus: 'playUntracked', detailsButton: 'playUntracked' }), { type: 'down' }).state.detailsFocus).toBe('gallery')
    const s = det({ detailsFocus: 'play' })
    expect(run(s, { type: 'down' }, ctx({ galleryCount: 0 })).state).toEqual(s)
  })

  it('BBX-032/up na galeria volta ao último botão (play ou playUntracked)', () => {
    expect(run(det({ detailsFocus: 'gallery', detailsButton: 'play' }), { type: 'up' }).state.detailsFocus).toBe('play')
    expect(run(det({ detailsFocus: 'gallery', detailsButton: 'playUntracked' }), { type: 'up' }).state.detailsFocus).toBe('playUntracked')
  })

  it('BBX-032/up num botão e down na galeria não mudam nada', () => {
    const b = det({ detailsFocus: 'play' })
    expect(run(b, { type: 'up' }).state).toEqual(b)
    const g = det({ detailsFocus: 'gallery', gallery: 1 })
    expect(run(g, { type: 'down' }).state).toEqual(g)
  })

  it('BBX-032/galleryChanged(0) com foco na galeria volta ao último botão; vale com suspended', () => {
    const g = det({ detailsFocus: 'gallery', detailsButton: 'playUntracked' })
    expect(run(g, { type: 'galleryChanged', count: 0 }).state.detailsFocus).toBe('playUntracked')
    const susp = det({ detailsFocus: 'gallery', detailsButton: 'play', suspended: true, launchedGameId: 7 })
    expect(run(susp, { type: 'galleryChanged', count: 0 }).state.detailsFocus).toBe('play')
  })

  it('BBX-033/next em play vai a playUntracked e prev volta, sem dar a volta; gallery não muda', () => {
    const p = det({ detailsFocus: 'play', detailsButton: 'play', gallery: 1 })
    const n = run(p, { type: 'next' }).state
    expect(n).toMatchObject({ detailsFocus: 'playUntracked', detailsButton: 'playUntracked', gallery: 1 })
    expect(run(n, { type: 'next' }).state.detailsFocus).toBe('playUntracked')
    expect(run(n, { type: 'prev' }).state).toMatchObject({ detailsFocus: 'play', detailsButton: 'play' })
    expect(run(p, { type: 'prev' }).state.detailsFocus).toBe('play')
  })

  it('BBX-034/com foco na galeria prev e next movem gallery em [0, galleryCount − 1] sem dar a volta; detailsFocus não muda', () => {
    const g = det({ detailsFocus: 'gallery', gallery: 0, focus: 4 })
    expect(run(g, { type: 'prev' }).state.gallery).toBe(0)
    const n = run(g, { type: 'next' }).state
    expect(n).toMatchObject({ gallery: 1, detailsFocus: 'gallery', focus: 4 })
    expect(run({ ...g, gallery: 2 }, { type: 'next' }).state.gallery).toBe(2)
    expect(run({ ...g, gallery: 2 }, { type: 'prev' }).state.gallery).toBe(1)
  })

  it('BBX-034/galleryChanged(n) limita gallery a n − 1', () => {
    const g = det({ detailsFocus: 'gallery', gallery: 5 })
    expect(run(g, { type: 'galleryChanged', count: 3 }).state.gallery).toBe(2)
    expect(run(det({ gallery: 1 }), { type: 'galleryChanged', count: 4 }).state.gallery).toBe(1)
  })

  it('BBX-016/accept na galeria não pede efeito', () => {
    expect(run(det({ detailsFocus: 'gallery' }), { type: 'accept' }).effects).toEqual([])
  })

  it('BBX-016/accept em play e playUntracked pede launch e launchUntracked', () => {
    const d = st({ screen: 'details' })
    expect(run({ ...d, detailsFocus: 'play' }, { type: 'accept' }).effects).toEqual([{ kind: 'launch', gameId: 7 }])
    expect(run({ ...d, detailsFocus: 'playUntracked' }, { type: 'accept' }).effects).toEqual([{ kind: 'launchUntracked', gameId: 7 }])
  })

  it('BBX-002/back em browse pede exit; em details volta a browse com o mesmo foco', () => {
    expect(run(st(), { type: 'back' }).effects).toEqual([{ kind: 'exit' }])
    const r = run(st({ screen: 'details', focus: 12 }), { type: 'back' })
    expect(r.state).toMatchObject({ screen: 'browse', focus: 12 })
    expect(r.effects).toEqual([])
  })

  it('BBX-013/toggleSound só com vídeo visível', () => {
    // alterado em 2026-11-17 (1.0.3): o som começa ligado
    expect(run(st(), { type: 'toggleSound' }, ctx({ videoVisible: false })).state.sound).toBe(true)
    const off = run(st(), { type: 'toggleSound' }, ctx({ videoVisible: true })).state
    expect(off.sound).toBe(false)
    expect(run(off, { type: 'toggleSound' }, ctx({ videoVisible: true })).state.sound).toBe(true)
  })

  it('BBX-018/launched liga suspended e guarda o jogo; qualquer ação de entrada é ignorada', () => {
    const s = run(st(), { type: 'launched', gameId: 7 }).state
    expect(s).toMatchObject({ suspended: true, launchedGameId: 7 })
    for (const type of ['prev', 'next', 'up', 'down', 'tabPrev', 'tabNext', 'accept', 'details', 'back', 'toggleSound'] as const) {
      const r = run(s, { type }, ctx({ videoVisible: true }))
      expect(r.state).toEqual(s)
      expect(r.effects).toEqual([])
    }
  })

  it('BBX-018/windowBlur com suspended marca awaitingFocusAfterBlur; windowFocus só solta depois dele', () => {
    const s = st({ suspended: true, launchedGameId: 7 })
    const b = run(s, { type: 'windowBlur' }).state
    expect(b.awaitingFocusAfterBlur).toBe(true)
    expect(b.suspended).toBe(true)
    expect(run(b, { type: 'windowFocus' }).state).toMatchObject({ suspended: false, awaitingFocusAfterBlur: false })
  })

  it('BBX-018/windowFocus sem blur anterior não solta', () => {
    const s = st({ suspended: true, launchedGameId: 7 })
    expect(run(s, { type: 'windowFocus' }).state.suspended).toBe(true)
  })

  it('BBX-018/sessionEnded do jogo lançado solta; de outro jogo não', () => {
    const s = st({ suspended: true, launchedGameId: 7 })
    expect(run(s, { type: 'sessionEnded', gameId: 7 }).state.suspended).toBe(false)
    expect(run(s, { type: 'sessionEnded', gameId: 8 }).state.suspended).toBe(true)
  })

  it('CON-022/gameClosed solta a suspensão, sai de Detalhes, foca indexInTab (ou mantém) e pede bringToFront', () => {
    const s = st({ suspended: true, launchedGameId: 7, screen: 'details', focus: 3 })
    const r = run(s, { type: 'gameClosed', gameId: 7, indexInTab: 20 })
    expect(r.state).toMatchObject({ suspended: false, screen: 'browse', focus: 20 })
    expect(r.effects).toContainEqual({ kind: 'bringToFront' })
    const k = run(s, { type: 'gameClosed', gameId: 7, indexInTab: null })
    expect(k.state).toMatchObject({ screen: 'browse', focus: 3, suspended: false })
    expect(k.effects).toContainEqual({ kind: 'bringToFront' })
  })

  it('BBX-027/listLoaded na entrada foca o índice do jogo em foco da janela normal ou 0', () => {
    expect(run(st({ focus: 4 }), { type: 'listLoaded', initialIndex: 17 }).state.focus).toBe(17)
    expect(run(st({ focus: 4 }), { type: 'listLoaded', initialIndex: null }).state.focus).toBe(0)
  })

  it('BBX-012/foco muda: videoFailed volta a null', () => {
    const f = run(st({ videoFailed: 7 }), { type: 'videoFailed', gameId: 7 }).state
    expect(f.videoFailed).toBe(7)
    expect(run(f, { type: 'next' }).state.videoFailed).toBeNull()
    expect(run(f, { type: 'prev' }, ctx()).state.videoFailed).toBeNull()
  })

  it('BBX-024/tema falso vertical: o mesmo reducer com prev e next vindas de ↑/↓ funciona', () => {
    let s = st({ focus: 2 })
    s = run(s, { type: 'next' }).state
    s = run(s, { type: 'next' }).state
    s = run(s, { type: 'prev' }).state
    expect(s.focus).toBe(3)
  })

  it('BBX-014/accept repetido não é problema do reducer', () => {
    const r1 = run(st(), { type: 'accept' })
    expect(r1.effects).toHaveLength(1)
    expect(run(st(), { type: 'accept' })).toEqual(r1)
  })

  it('BBX-006/lista vazia: prev e next não saem de 0', () => {
    const c = ctx({ total: 0, loaded: 0, focusedGameId: null })
    expect(run(st(), { type: 'next' }, c).state.focus).toBe(0)
    expect(run(st(), { type: 'prev' }, c).state.focus).toBe(0)
  })
})

// Grade 2D (fase 15, temas-v2 §3.1).
describe('bigBoxNav grade', () => {
  // alterado em 2026-11-27 (fase 17): a grade depende de axis = 'grid'
  const grid = (total: number, loaded: number) => ctx({ columns: 5, total, loaded, axis: 'grid' })

  it.each([
    [23, 23, 2, 'down', 7],
    [23, 23, 17, 'down', 22],
    [23, 23, 20, 'down', 20],
    [23, 23, 3, 'up', 3],
    [23, 23, 9, 'next', 10],
    [23, 23, 22, 'next', 22],
    [40, 25, 18, 'down', 23],
    [40, 25, 22, 'down', 22],
  ] as const)('BBX-048/reducer T %i L %i f %i %s -> %i', (t, l, f, action, want) => {
    expect(run(st({ focus: f }), { type: action }, grid(t, l)).state.focus).toBe(want)
  })

  it('BBX-048/up anda uma fileira e prev/next ficam em [0, M]', () => {
    expect(run(st({ focus: 12 }), { type: 'up' }, grid(23, 23)).state.focus).toBe(7)
    expect(run(st({ focus: 0 }), { type: 'prev' }, grid(23, 23)).state.focus).toBe(0)
    expect(run(st({ focus: 6 }), { type: 'prev' }, grid(23, 23)).state.focus).toBe(5)
  })

  it('BBX-048/M é limitado por L: next não passa de L − 1 e down não passa de L − 1', () => {
    const c = grid(60, 25)
    expect(run(st({ focus: 24 }), { type: 'next' }, c).state.focus).toBe(24)
    // alterado em 2026-11-22 (fase 15): contrato §3.1, M = 24; 21 e 24 estão na mesma fileira, então down não anda.
    expect(run(st({ focus: 21 }), { type: 'down' }, c).state.focus).toBe(21)
    expect(run(st({ focus: 18 }), { type: 'down' }, c).state.focus).toBe(23)
    expect(run(st({ focus: 24 }), { type: 'down' }, c).state.focus).toBe(24)
  })

  it('BBX-048/nos eixos horizontal e vertical nada muda (up/down sem efeito em Navegar, C = 1)', () => {
    const c = ctx({ columns: 1 })
    expect(run(st({ focus: 5 }), { type: 'up' }, c).state).toEqual(st({ focus: 5 }))
    expect(run(st({ focus: 5 }), { type: 'down' }, c).state).toEqual(st({ focus: 5 }))
    expect(run(st({ focus: 5 }), { type: 'next' }, c).state.focus).toBe(6)
  })

  it('BBX-048/na tela Detalhes as direções seguem BBX-032..BBX-034 em qualquer eixo', () => {
    const c = ctx({ columns: 5, galleryCount: 3 })
    const d = st({ screen: 'details', focus: 12 })
    expect(run(d, { type: 'down' }, c).state).toMatchObject({ detailsFocus: 'gallery', focus: 12 })
    expect(run(st({ screen: 'details', focus: 12, detailsFocus: 'gallery', detailsButton: 'play' }), { type: 'up' }, c).state)
      .toMatchObject({ detailsFocus: 'play', focus: 12 })
    expect(run(d, { type: 'next' }, c).state).toMatchObject({ detailsFocus: 'playUntracked', focus: 12 })
    expect(run(st({ screen: 'details', focus: 12, detailsFocus: 'gallery' }), { type: 'next' }, c).state)
      .toMatchObject({ gallery: 1, focus: 12 })
  })

  it('BBX-048/mover o foco na grade zera videoFailed e não gera efeitos', () => {
    const r = run(st({ focus: 2, videoFailed: 9 }), { type: 'down' }, grid(23, 23))
    expect(r.state.videoFailed).toBeNull()
    expect(r.effects).toEqual([])
  })
})

// Fase 17 (contrato §2): zonas, layout columns e tabSet.
describe('bigBoxNav layout columns', () => {
  const cc = (o: Partial<NavCtx> = {}) =>
    ctx({ columns: 6, total: 20, loaded: 20, tabs: 4, galleryCount: 3, layout: 'columns', axis: 'grid', ...o })
  const f = (focus: number, o: Partial<NavState> = {}) => st({ focus, ...o })
  const det = (o: Partial<NavState> = {}) => st({ screen: 'details', ...o })
  const pf = (o: Partial<NavState> = {}) => st({ zone: 'platforms', ...o })

  it('TDK-012/estado inicial tem zone games', () => {
    expect(initialNav.zone).toBe('games')
  })

  it('TDK-012/listLoaded e gameClosed levam a zone games (a partir de platforms)', () => {
    expect(run(pf(), { type: 'listLoaded', initialIndex: 2 }, cc()).state.zone).toBe('games')
    expect(run(pf(), { type: 'gameClosed', gameId: 7, indexInTab: 2 }, cc()).state.zone).toBe('games')
  })

  it.each([
    [0, 'prev', { zone: 'platforms', focus: 0 }],
    [3, 'prev', { zone: 'games', focus: 2 }],
    [4, 'next', { zone: 'games', focus: 5 }],
    [5, 'next', { screen: 'details', detailsFocus: 'play', detailsButton: 'play', gallery: 0 }],
    [19, 'next', { screen: 'details', detailsFocus: 'play' }],
    [13, 'down', { focus: 19 }],
    [14, 'down', { focus: 19 }],
    [18, 'down', { focus: 18 }],
    [8, 'up', { focus: 2 }],
    [2, 'up', { focus: 2 }],
  ] as const)('TDK-013/vetores de games com C=6: f %i %s', (focus, action, want) => {
    expect(run(f(focus), { type: action }, cc()).state).toMatchObject(want)
  })

  it('TDK-013/T=0: prev leva a platforms e next não faz nada', () => {
    const c = cc({ total: 0, loaded: 0, focusedGameId: null })
    expect(run(f(0), { type: 'prev' }, c).state.zone).toBe('platforms')
    const s = f(0)
    expect(run(s, { type: 'next' }, c).state).toEqual(s)
  })

  it('TDK-013/nunca entra em jogo não lido (loaded < total: down e next param em M = loaded-1)', () => {
    const c = cc({ total: 40, loaded: 15 })
    expect(run(f(13), { type: 'next' }, c).state.focus).toBe(14)
    expect(run(f(8), { type: 'down' }, c).state.focus).toBe(14)
    expect(run(f(14), { type: 'down' }, c).state.focus).toBe(14)
    expect(run(f(14), { type: 'next' }, c).state.focus).toBe(14)
  })

  it('TDK-013/next sem jogo em foco não vai a details', () => {
    const s = f(5)
    expect(run(s, { type: 'next' }, cc({ focusedGameId: null })).state).toEqual(s)
  })

  it('TDK-014/C=1, T=5: up/down/prev/next em 0/0/2/2 dão 0/1/platforms/details', () => {
    const c = cc({ columns: 1, total: 5, loaded: 5 })
    expect(run(f(0), { type: 'up' }, c).state.focus).toBe(0)
    expect(run(f(0), { type: 'down' }, c).state.focus).toBe(1)
    expect(run(f(2), { type: 'prev' }, c).state.zone).toBe('platforms')
    expect(run(f(2), { type: 'next' }, c).state.screen).toBe('details')
  })

  it('TDK-015/platforms: up em tab 0 e down em tab 3 (tabs 4) ficam iguais', () => {
    const a = pf({ tab: 0 })
    expect(run(a, { type: 'up' }, cc()).state).toEqual(a)
    const b = pf({ tab: 3 })
    expect(run(b, { type: 'down' }, cc()).state).toEqual(b)
  })

  it('TDK-015/up e down trocam a aba com focus 0 e videoFailed null', () => {
    expect(run(pf({ tab: 2, focus: 9, videoFailed: 7 }), { type: 'up' }, cc()).state)
      .toMatchObject({ tab: 1, focus: 0, videoFailed: null, zone: 'platforms' })
    expect(run(pf({ tab: 0, focus: 9, videoFailed: 7 }), { type: 'down' }, cc()).state)
      .toMatchObject({ tab: 1, focus: 0, videoFailed: null, zone: 'platforms' })
  })

  it('TDK-015/next, accept e back levam a games sem effects (nem launch nem exit)', () => {
    for (const type of ['next', 'accept', 'back'] as const) {
      const r = run(pf({ focus: 3 }), { type }, cc())
      expect(r.state).toMatchObject({ zone: 'games', screen: 'browse', focus: 3 })
      expect(r.effects).toEqual([])
    }
  })

  it('TDK-015/prev não faz nada', () => {
    const s = pf({ focus: 3 })
    const r = run(s, { type: 'prev' }, cc())
    expect(r.state).toEqual(s)
    expect(r.effects).toEqual([])
  })

  it('TDK-015/accept em platforms com aba vazia também volta a games', () => {
    const c = cc({ total: 0, loaded: 0, focusedGameId: null })
    const r = run(pf(), { type: 'accept' }, c)
    expect(r.state.zone).toBe('games')
    expect(r.effects).toEqual([])
  })

  it('TDK-016/tabPrev e tabNext são circulares, focus 0 e mantêm a zona (games e platforms)', () => {
    for (const zone of ['games', 'platforms'] as const) {
      expect(run(st({ zone, tab: 3, focus: 9 }), { type: 'tabNext' }, cc()).state).toMatchObject({ tab: 0, focus: 0, zone })
      expect(run(st({ zone, tab: 0, focus: 9 }), { type: 'tabPrev' }, cc()).state).toMatchObject({ tab: 3, focus: 0, zone })
    }
  })

  it('TDK-016/details (X) a partir de platforms com jogo em foco abre details com foco em play; sem jogo não faz nada', () => {
    expect(run(pf({ focus: 4 }), { type: 'details' }, cc()).state)
      .toMatchObject({ screen: 'details', detailsFocus: 'play', gallery: 0 })
    const s = pf()
    expect(run(s, { type: 'details' }, cc({ focusedGameId: null })).state).toEqual(s)
  })

  it('TDK-017/games: accept com jogo → launch; accept sem jogo nada; back → exit', () => {
    expect(run(f(3), { type: 'accept' }, cc()).effects).toEqual([{ kind: 'launch', gameId: 7 }])
    expect(run(f(3), { type: 'accept' }, cc({ focusedGameId: null })).effects).toEqual([])
    expect(run(f(3), { type: 'back' }, cc()).effects).toEqual([{ kind: 'exit' }])
  })

  it('TDK-018/details, play: up → gallery; down → playUntracked; prev → browse games com o mesmo focus; next igual; back → browse games', () => {
    const d = det({ focus: 4, detailsFocus: 'play', detailsButton: 'play', zone: 'platforms' })
    expect(run(d, { type: 'up' }, cc()).state.detailsFocus).toBe('gallery')
    expect(run(d, { type: 'down' }, cc()).state.detailsFocus).toBe('playUntracked')
    expect(run(d, { type: 'prev' }, cc()).state).toMatchObject({ screen: 'browse', zone: 'games', focus: 4 })
    expect(run(d, { type: 'next' }, cc()).state).toEqual(d)
    expect(run(d, { type: 'back' }, cc()).state).toMatchObject({ screen: 'browse', zone: 'games', focus: 4 })
  })

  it('TDK-018/play sem galeria: up igual', () => {
    const d = det({ detailsFocus: 'play' })
    expect(run(d, { type: 'up' }, cc({ galleryCount: 0 })).state).toEqual(d)
  })

  it('TDK-018/gallery: down → play; prev/next limitados a [0, galleryCount-1]; up igual', () => {
    const g = det({ detailsFocus: 'gallery', detailsButton: 'play', gallery: 1 })
    expect(run(g, { type: 'down' }, cc()).state.detailsFocus).toBe('play')
    expect(run(g, { type: 'up' }, cc()).state).toEqual(g)
    expect(run(g, { type: 'prev' }, cc()).state.gallery).toBe(0)
    expect(run({ ...g, gallery: 0 }, { type: 'prev' }, cc()).state.gallery).toBe(0)
    expect(run(g, { type: 'next' }, cc()).state.gallery).toBe(2)
    expect(run({ ...g, gallery: 2 }, { type: 'next' }, cc()).state.gallery).toBe(2)
  })

  it('TDK-018/playUntracked: up → play e down igual', () => {
    const d = det({ detailsFocus: 'playUntracked', detailsButton: 'playUntracked' })
    expect(run(d, { type: 'up' }, cc()).state.detailsFocus).toBe('play')
    expect(run(d, { type: 'down' }, cc()).state).toEqual(d)
  })

  it('TDK-018/galleryChanged(0) com foco no visor → play', () => {
    const g = det({ detailsFocus: 'gallery', detailsButton: 'play' })
    expect(run(g, { type: 'galleryChanged', count: 0 }, cc()).state.detailsFocus).toBe('play')
  })

  it('TDK-018/accept em play e playUntracked dá launch e launchUntracked; no visor nada', () => {
    expect(run(det({ detailsFocus: 'play' }), { type: 'accept' }, cc()).effects).toEqual([{ kind: 'launch', gameId: 7 }])
    expect(run(det({ detailsFocus: 'playUntracked' }), { type: 'accept' }, cc()).effects)
      .toEqual([{ kind: 'launchUntracked', gameId: 7 }])
    expect(run(det({ detailsFocus: 'gallery' }), { type: 'accept' }, cc()).effects).toEqual([])
  })

  it('BBX-065/tabSet: índice diferente e válido → browse, tab i, focus 0', () => {
    expect(run(st({ tab: 0, focus: 9, zone: 'platforms' }), { type: 'tabSet', index: 2 }, cc()).state)
      .toMatchObject({ screen: 'browse', tab: 2, focus: 0, zone: 'platforms' })
  })

  it('BBX-065/tabSet: índice igual, -1 e tabs ficam iguais', () => {
    const s = st({ tab: 1, focus: 5 })
    for (const index of [1, -1, 4, 9]) {
      expect(run(s, { type: 'tabSet', index }, cc()).state).toEqual(s)
    }
  })

  it('BBX-065/tabSet a partir de details volta a browse', () => {
    expect(run(det({ tab: 0, focus: 5 }), { type: 'tabSet', index: 3 }, cc()).state)
      .toMatchObject({ screen: 'browse', tab: 3, focus: 0 })
  })

  it('BBX-065/tabSet é ignorado com suspended (e windowFocus ainda libera)', () => {
    const s = st({ suspended: true, launchedGameId: 7, tab: 0 })
    expect(run(s, { type: 'tabSet', index: 2 }, cc()).state).toEqual(s)
    const b = run(s, { type: 'windowBlur' }, cc()).state
    expect(run(b, { type: 'windowFocus' }, cc()).state.suspended).toBe(false)
  })

  it('TDK-033/gameClosed volta a browse, zone games, focus = indexInTab, com os effects bringToFront e enterBigBox', () => {
    const r = run(det({ suspended: true, launchedGameId: 7, zone: 'platforms', focus: 1 }),
      { type: 'gameClosed', gameId: 7, indexInTab: 12 }, cc())
    expect(r.state).toMatchObject({ screen: 'browse', zone: 'games', focus: 12, suspended: false })
    expect(r.effects).toEqual([{ kind: 'bringToFront' }, { kind: 'enterBigBox' }])
  })

  it('BBX-048/layout screens: ←/→ passam de fileira e zone fica games', () => {
    const c = ctx({ columns: 5, total: 23, loaded: 23, axis: 'grid', layout: 'screens' })
    expect(run(f(0), { type: 'prev' }, c).state).toMatchObject({ focus: 0, zone: 'games' })
    expect(run(f(4), { type: 'next' }, c).state).toMatchObject({ focus: 5, zone: 'games', screen: 'browse' })
    expect(run(f(5), { type: 'prev' }, c).state).toMatchObject({ focus: 4, zone: 'games' })
  })

  it('BBX-048/axis horizontal com columns 1: up/down não fazem nada', () => {
    const c = ctx({ columns: 1, axis: 'horizontal', layout: 'screens' })
    expect(run(f(5), { type: 'up' }, c).state).toEqual(f(5))
    expect(run(f(5), { type: 'down' }, c).state).toEqual(f(5))
  })
})
