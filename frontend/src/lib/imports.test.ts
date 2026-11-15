import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildReviewRows,
  defaultPlatformName,
  isTitleEditable,
  markImported,
  repeatedFlags,
  scanImport,
  selectionCounts,
  titleProblem,
  type ImportCandidate,
  type ImportScanResult,
  type ReviewRow,
} from './imports'

function cand(over: Partial<ImportCandidate> = {}): ImportCandidate {
  return {
    source: 'folder',
    title: 'Jogo',
    platform: 'Windows',
    launchType: 'exe',
    launchTarget: 'D:\\jogos\\jogo.exe',
    installDir: 'D:\\jogos',
    developers: [],
    publishers: [],
    genres: [],
    year: null,
    description: '',
    playModes: [],
    playedSeconds: null,
    lastPlayed: null,
    exists: false,
    ...over,
  }
}

function row(title: string, exists = false, checked = true): ReviewRow {
  return { candidate: cand({ title, exists }), title, checked }
}

function install(impl?: (...args: unknown[]) => Promise<unknown>) {
  const fn = vi.fn(impl ?? (() => Promise.resolve({ candidates: [], warnings: [] } as ImportScanResult)))
  ;(window as unknown as { go?: unknown }).go = { main: { App: { ImportScan: fn } } }
  return fn
}

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

// Phase 12 (T36): ImportCandidate.searchName is optional; only the Arquivo source fills it (MET-040).
describe('searchName (MET-040, fase 12 T36)', () => {
  it('MET-040/searchName ausente em outras fontes não quebra o tipo: candidato de Steam passa por buildReviewRows', () => {
    const steam = cand({ source: 'steam', title: 'Hades' })
    expect(steam.searchName).toBeUndefined()
    const rows = buildReviewRows([steam])
    expect(rows[0].title).toBe('Hades')
    expect(rows[0].checked).toBe(true)
  })

  it('MET-040/searchName do candidato de Arquivo chega à linha de revisão sem alterar o título', () => {
    const file = cand({ source: 'file', title: 'Hades', searchName: 'Hades' })
    const rows = buildReviewRows([file])
    expect(rows[0].candidate.searchName).toBe('Hades')
    expect(rows[0].title).toBe('Hades')
  })
})

describe('scanImport (IMV-063)', () => {
  it('IMV-063/scanImport repassa platform, titleFrom e recursive ao binding', async () => {
    const result: ImportScanResult = { candidates: [cand()], warnings: ['aviso'] }
    const fn = install(() => Promise.resolve(result))
    const opts = { platform: 'Jogos de PC', titleFrom: 'folder' as const, recursive: false }

    const got = await scanImport('folder', 'D:\\jogos', opts)

    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith('folder', 'D:\\jogos', opts)
    expect(got).toEqual(result)
  })

  it('IMV-063/binding ausente falha como chamada com erro', async () => {
    delete (window as unknown as { go?: unknown }).go

    await expect(
      scanImport('folder', 'D:\\jogos', { platform: 'Windows', titleFrom: 'file', recursive: true }),
    ).rejects.toThrow(Error)
  })
})

describe('isTitleEditable (IMV-044)', () => {
  it('IMV-044/isTitleEditable só para file e folder', () => {
    expect(isTitleEditable('file')).toBe(true)
    expect(isTitleEditable('folder')).toBe(true)
    expect(isTitleEditable('steam')).toBe(false)
    expect(isTitleEditable('epic')).toBe(false)
    expect(isTitleEditable('launchbox')).toBe(false)
  })
})

describe('titleProblem (IMV-045)', () => {
  it.each([
    ['IMV-045/título vazio é "empty"', '', 'empty'],
    ['IMV-045/só espaços é "empty"', '   \t ', 'empty'],
    ['IMV-045/200 runas são válidas', 'a'.repeat(200), null],
    ['IMV-045/201 runas é "tooLong"', 'a'.repeat(201), 'tooLong'],
    ['IMV-045/200 acentos são válidos (acento conta como uma runa)', 'á'.repeat(200), null],
    ['IMV-045/201 acentos é "tooLong"', 'á'.repeat(201), 'tooLong'],
    ['IMV-045/espaços nas pontas não contam para o limite', '  ' + 'a'.repeat(200) + '  ', null],
  ])('%s', (_name, title, want) => {
    expect(titleProblem(title)).toBe(want)
  })
})

describe('repeatedFlags (IMV-046)', () => {
  it('IMV-046/repeatedFlags: segunda em diante, sem diferenciar maiúsculas, aparado, ignora exists', () => {
    const rows = [
      { title: 'Jogo', exists: false },
      { title: 'jogo ', exists: false },
      { title: 'Outro', exists: true },
      { title: 'Outro', exists: false },
      { title: 'JOGO', exists: false },
      { title: 'Único', exists: false },
    ]

    expect(repeatedFlags(rows)).toEqual([false, true, false, false, true, false])
  })
})

describe('defaultPlatformName (IMV-061)', () => {
  it('IMV-061/defaultPlatformName: chave windows com nome trocado, windows ausente → primeira, lista vazia → null', () => {
    expect(
      defaultPlatformName([
        { name: 'PS1', seedKey: 'psx' },
        { name: 'Meu PC', seedKey: 'windows' },
      ]),
    ).toBe('Meu PC')

    expect(
      defaultPlatformName([
        { name: 'PS1', seedKey: 'psx' },
        { name: 'N64', seedKey: 'n64' },
      ]),
    ).toBe('PS1')

    expect(defaultPlatformName([])).toBeNull()
  })
})

describe('buildReviewRows (IMV-041)', () => {
  it('IMV-041/buildReviewRows: tudo marcado menos exists e repetidas', () => {
    const rows = buildReviewRows([
      cand({ title: 'A' }),
      cand({ title: 'a ' }),
      cand({ title: 'B', exists: true }),
      cand({ title: 'C' }),
    ])

    expect(rows.map((r) => r.title)).toEqual(['A', 'a ', 'B', 'C'])
    expect(rows.map((r) => r.checked)).toEqual([true, false, false, true])
  })
})

describe('selectionCounts (IMV-047)', () => {
  it('IMV-047/selectionCounts com e sem existentes', () => {
    const withExisting: ReviewRow[] = [
      ...Array.from({ length: 13 }, (_, i) => row(`Jogo ${i}`)),
      row('Já 1', true, false),
      row('Já 2', true, false),
    ]
    withExisting[3].checked = false
    expect(selectionCounts(withExisting)).toEqual({ checked: 12, selectable: 13, existing: 2 })

    const without: ReviewRow[] = [row('A'), row('B'), row('C', false, false)]
    expect(selectionCounts(without)).toEqual({ checked: 2, selectable: 3, existing: 0 })
  })
})

describe('markImported (IMV-050)', () => {
  it('IMV-050/markImported: títulos aplicados viram exists, cada título do resultado consome uma linha, enviadas desmarcam, não enviadas continuam marcadas', () => {
    const a1 = row('A')
    const a2 = row('A', false, false)
    const b = row('B')
    const c = row('C')
    const rows = [a1, a2, b, c]

    const out = markImported(rows, [a1, a2, b], [{ title: 'A' }, { title: 'B' }])

    // one 'A' applied: only the first one becomes exists
    expect(out[0].candidate.exists).toBe(true)
    expect(out[0].checked).toBe(false)
    expect(out[1].candidate.exists).toBe(false)
    expect(out[1].checked).toBe(false)
    expect(out[2].candidate.exists).toBe(true)
    expect(out[2].checked).toBe(false)
    // not sent: keeps its checkbox
    expect(out[3].candidate.exists).toBe(false)
    expect(out[3].checked).toBe(true)
  })

  it('IMV-050/título editado é casado pelo título gravado', () => {
    const edited = { ...row('Antigo'), title: 'Novo nome' }
    const out = markImported([edited], [edited], [{ title: 'Novo nome' }])

    expect(out[0].candidate.exists).toBe(true)
    expect(out[0].checked).toBe(false)
  })

  it('IMV-050/o selo vai para a linha enviada quando uma não enviada de mesmo título vem antes; a não enviada fica como estava', () => {
    const unsent = row('Foo', false, false)
    const sent = row('Foo', false, true)

    const out = markImported([unsent, sent], [sent], [{ title: 'Foo' }])

    expect(out[0]).toEqual(unsent)
    expect(out[1].candidate.exists).toBe(true)
    expect(out[1].checked).toBe(false)
  })

  it('IMV-050/duas linhas com o mesmo título e dois títulos gravados viram exists as duas', () => {
    const x = row('A')
    const y = row('A')
    const out = markImported([x, y], [x, y], [{ title: 'A' }, { title: 'A' }])

    expect(out.map((r) => r.candidate.exists)).toEqual([true, true])
    expect(out.map((r) => r.checked)).toEqual([false, false])
  })
})
