import { afterEach, describe, expect, it, vi } from 'vitest'
import { applyImportInLots, IMPORT_LOT_SIZE, type ImportApplyResult, type ImportCandidate } from './imports'

function cand(i: number): ImportCandidate {
  return {
    source: 'launchbox',
    title: `Jogo ${i}`,
    platform: 'Windows',
    launchType: 'exe',
    launchTarget: `C:\\g\\${i}.exe`,
    installDir: '',
    developers: [],
    publishers: [],
    genres: [],
    year: null,
    description: '',
    playModes: [],
    playedSeconds: null,
    lastPlayed: null,
    exists: false,
  }
}

function many(n: number): ImportCandidate[] {
  return Array.from({ length: n }, (_, i) => cand(i))
}

function install(impl: (c: ImportCandidate[], p: string) => Promise<ImportApplyResult>) {
  const fn = vi.fn(impl)
  ;(window as unknown as { go?: unknown }).go = { main: { App: { ImportApply: fn } } }
  return fn
}

// Each lot: one applied per candidate, one skipped.
const okLot = (c: ImportCandidate[]): Promise<ImportApplyResult> =>
  Promise.resolve({
    applied: c.slice(1).map((x, i) => ({ gameId: i, title: x.title })),
    skipped: [{ title: c[0].title, reason: 'duplicate' }],
  })

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('applyImportInLots', () => {
  it('AJU-025/60 candidatos geram 3 chamadas ImportApply de 25, 25 e 10, na ordem, com o mesmo launchboxPath', async () => {
    const fn = install(okLot)
    const list = many(60)
    expect(IMPORT_LOT_SIZE).toBe(25)
    await applyImportInLots(list, 'D:\\LB\\Data\\Platforms', vi.fn())

    expect(fn).toHaveBeenCalledTimes(3)
    expect(fn.mock.calls.map((c) => c[0].length)).toEqual([25, 25, 10])
    expect(fn.mock.calls.flatMap((c) => c[0].map((x) => x.title))).toEqual(list.map((x) => x.title))
    expect(fn.mock.calls.map((c) => c[1])).toEqual(Array(3).fill('D:\\LB\\Data\\Platforms'))
  })

  it('AJU-025/o resultado soma applied e skipped dos lotes', async () => {
    install(okLot)
    const r = await applyImportInLots(many(60), '', vi.fn())

    expect(r.result.applied).toHaveLength(24 + 24 + 9)
    expect(r.result.skipped).toHaveLength(3)
    expect(r.notSent).toBe(0)
    expect(r.error).toBeUndefined()
  })

  it('AJU-025/falha no 2º lote: o 3º não é enviado, result tem só o 1º e notSent = 35', async () => {
    let n = 0
    const fn = install((c) => {
      n++
      return n === 2 ? Promise.reject(new Error('tempo esgotado')) : okLot(c)
    })
    const r = await applyImportInLots(many(60), '', vi.fn())

    expect(fn).toHaveBeenCalledTimes(2)
    expect(r.result.applied).toHaveLength(24)
    expect(r.result.skipped).toHaveLength(1)
    expect(r.notSent).toBe(35)
    expect(r.error).toBeInstanceOf(Error)
    expect(r.error?.message).toBe('tempo esgotado')
  })

  it('AJU-025/onProgress recebe 0, 25, 50 e 60', async () => {
    install(okLot)
    const onProgress = vi.fn()
    await applyImportInLots(many(60), '', onProgress)

    expect(onProgress.mock.calls).toEqual([
      [0, 60],
      [25, 60],
      [50, 60],
      [60, 60],
    ])
  })

  it('AJU-025/600 candidatos são aceitos (24 chamadas, nenhuma com mais de 500)', async () => {
    const fn = install(okLot)
    const r = await applyImportInLots(many(600), '', vi.fn())

    expect(fn).toHaveBeenCalledTimes(24)
    expect(Math.max(...fn.mock.calls.map((c) => c[0].length))).toBeLessThanOrEqual(500)
    expect(r.notSent).toBe(0)
    expect(r.error).toBeUndefined()
  })
})
