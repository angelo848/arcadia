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
