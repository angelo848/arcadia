import { describe, expect, it } from 'vitest'
import { focusedGameId, initialSelection, selectionReducer } from './selection'
import type { Selection } from './selection'

const loading = (gameId: number): Selection => ({ kind: 'loading', gameId })
const shown = (gameId: number): Selection => ({ kind: 'shown', gameId })
const failed = (gameId: number, message = 'falhou'): Selection => ({ kind: 'error', gameId, message })

describe('selectionReducer', () => {
  it('UIX-024/estado inicial é empty', () => {
    expect(initialSelection).toEqual({ kind: 'empty' })
    expect(focusedGameId(initialSelection)).toBeNull()
  })

  it('UIX-017/select(H) de empty vai para loading(H)', () => {
    expect(selectionReducer(initialSelection, { type: 'select', gameId: 7 })).toEqual(loading(7))
  })

  it('UIX-017/select do mesmo jogo em loading, shown e error não muda o estado (mesma referência)', () => {
    for (const s of [loading(3), shown(3), failed(3)]) {
      expect(selectionReducer(s, { type: 'select', gameId: 3 })).toBe(s)
    }
  })

  it('UIX-017/select(H≠G) em loading, shown e error vai para loading(H)', () => {
    for (const s of [loading(3), shown(3), failed(3)]) {
      expect(selectionReducer(s, { type: 'select', gameId: 9 })).toEqual(loading(9))
    }
  })

  it('UIX-035/readOk(G) em loading(G) vai para shown(G)', () => {
    expect(selectionReducer(loading(3), { type: 'readOk', gameId: 3 })).toEqual(shown(3))
  })

  it('UIX-035/readError(G) em loading(G) vai para error(G, msg); retry volta a loading(G)', () => {
    const s = selectionReducer(loading(3), { type: 'readError', gameId: 3, message: 'sem agente' })
    expect(s).toEqual(failed(3, 'sem agente'))
    expect(selectionReducer(s, { type: 'retry' })).toEqual(loading(3))
  })

  it('UIX-035/retry fora de error não muda nada', () => {
    for (const s of [initialSelection, loading(3), shown(3)]) {
      expect(selectionReducer(s, { type: 'retry' })).toBe(s)
    }
  })

  it('UIX-022/readOk(G) em error(G) vai para shown(G)', () => {
    expect(selectionReducer(failed(3), { type: 'readOk', gameId: 3 })).toEqual(shown(3))
  })

  it('UIX-022/readError(G) em shown(G) mantém shown(G)', () => {
    const s = shown(3)
    const next = selectionReducer(s, { type: 'readError', gameId: 3, message: 'x' })
    expect(next).toEqual(shown(3))
  })

  it('UIX-023/readMissing(G) em loading(G) e em shown(G) vai para empty', () => {
    for (const s of [loading(3), shown(3)]) {
      expect(selectionReducer(s, { type: 'readMissing', gameId: 3 })).toEqual({ kind: 'empty' })
    }
  })

  it('UIX-021/readOk, readMissing e readError de J≠G são ignorados', () => {
    for (const s of [loading(3), shown(3), failed(3)]) {
      expect(selectionReducer(s, { type: 'readOk', gameId: 8 })).toEqual(s)
      expect(selectionReducer(s, { type: 'readMissing', gameId: 8 })).toEqual(s)
      expect(selectionReducer(s, { type: 'readError', gameId: 8, message: 'x' })).toEqual(s)
    }
  })

  it('UIX-034/removed(G) em shown(G) vai para empty; removed(J≠G) não muda nada', () => {
    expect(selectionReducer(shown(3), { type: 'removed', gameId: 3 })).toEqual({ kind: 'empty' })
    const s = shown(3)
    expect(selectionReducer(s, { type: 'removed', gameId: 8 })).toEqual(s)
  })

  it('UIX-022/focusedGameId devolve G em loading, shown e error e null em empty', () => {
    expect(focusedGameId(loading(4))).toBe(4)
    expect(focusedGameId(shown(4))).toBe(4)
    expect(focusedGameId(failed(4))).toBe(4)
    expect(focusedGameId({ kind: 'empty' })).toBeNull()
  })
})
