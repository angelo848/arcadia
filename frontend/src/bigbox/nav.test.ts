import { describe, expect, it } from 'vitest'
import { bigBoxNav, initialNav, PREFETCH_BEFORE_END } from './nav'
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
    expect(initialNav).toMatchObject({ screen: 'browse', tab: 0, focus: 0, suspended: false, sound: false, videoFailed: null, launchedGameId: null })
    expect(PREFETCH_BEFORE_END).toBe(10)
  })

  it('BBX-006/prev e next movem o foco de um em um e não dão a volta nas pontas', () => {
    expect(run(st({ focus: 5 }), { type: 'next' }).state.focus).toBe(6)
    expect(run(st({ focus: 5 }), { type: 'prev' }).state.focus).toBe(4)
    expect(run(st({ focus: 0 }), { type: 'prev' }).state.focus).toBe(0)
    expect(run(st({ focus: 99 }), { type: 'next' }, ctx({ loaded: 100 })).state.focus).toBe(99)
  })

  it('BBX-007/next com foco ≥ carregado − 10 pede fetchNextPage; antes disso não pede', () => {
    expect(run(st({ focus: 30 }), { type: 'next' }).effects).toEqual([])
    expect(run(st({ focus: 45 }), { type: 'next' }).effects).toContainEqual({ kind: 'fetchNextPage' })
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

  it('BBX-016/details abre Detalhes com detailsFocus play e galeria 0', () => {
    const s = run(st({ gallery: 2, detailsFocus: 'description' }), { type: 'details' }).state
    expect(s).toMatchObject({ screen: 'details', detailsFocus: 'play', gallery: 0 })
    expect(run(st(), { type: 'details' }, ctx({ focusedGameId: null })).state.screen).toBe('browse')
  })

  it('BBX-016/up e down percorrem play, playUntracked e description sem dar a volta', () => {
    let s = st({ screen: 'details', detailsFocus: 'play' })
    expect(run(s, { type: 'up' }).state.detailsFocus).toBe('play')
    s = run(s, { type: 'down' }).state
    expect(s.detailsFocus).toBe('playUntracked')
    s = run(s, { type: 'down' }).state
    expect(s.detailsFocus).toBe('description')
    expect(run(s, { type: 'down' }).state.detailsFocus).toBe('description')
    expect(run(s, { type: 'up' }).state.detailsFocus).toBe('playUntracked')
  })

  it('BBX-016/prev e next movem a galeria nos limites', () => {
    const d = st({ screen: 'details', gallery: 0, focus: 4 })
    expect(run(d, { type: 'prev' }).state.gallery).toBe(0)
    expect(run(d, { type: 'next' }).state.gallery).toBe(1)
    expect(run(d, { type: 'next' }).state.focus).toBe(4)
    expect(run({ ...d, gallery: 2 }, { type: 'next' }).state.gallery).toBe(2)
  })

  it('BBX-016/accept em play e playUntracked pede launch e launchUntracked', () => {
    const d = st({ screen: 'details' })
    expect(run({ ...d, detailsFocus: 'play' }, { type: 'accept' }).effects).toEqual([{ kind: 'launch', gameId: 7 }])
    expect(run({ ...d, detailsFocus: 'playUntracked' }, { type: 'accept' }).effects).toEqual([{ kind: 'launchUntracked', gameId: 7 }])
    expect(run({ ...d, detailsFocus: 'description' }, { type: 'accept' }).effects).toEqual([])
  })

  it('BBX-002/back em browse pede exit; em details volta a browse com o mesmo foco', () => {
    expect(run(st(), { type: 'back' }).effects).toEqual([{ kind: 'exit' }])
    const r = run(st({ screen: 'details', focus: 12 }), { type: 'back' })
    expect(r.state).toMatchObject({ screen: 'browse', focus: 12 })
    expect(r.effects).toEqual([])
  })

  it('BBX-013/toggleSound só com vídeo visível', () => {
    expect(run(st(), { type: 'toggleSound' }, ctx({ videoVisible: false })).state.sound).toBe(false)
    const on = run(st(), { type: 'toggleSound' }, ctx({ videoVisible: true })).state
    expect(on.sound).toBe(true)
    expect(run(on, { type: 'toggleSound' }, ctx({ videoVisible: true })).state.sound).toBe(false)
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
