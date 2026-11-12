import { describe, expect, it } from 'vitest'
import { validateDraft } from './validation'

// The draft type is the one validateDraft takes; the test does not depend on the module that exports it.
type EditDraft = Parameters<typeof validateDraft>[0]

function draft(over: Partial<EditDraft> = {}): EditDraft {
  return {
    title: 'Hollow Knight',
    description: 'Um metroidvania difícil.',
    year: '2017',
    completedAt: '',
    status: 'playing',
    favorite: false,
    developers: ['Team Cherry'],
    publishers: ['Team17'],
    genres: ['Metroidvania'],
    playModes: ['single'],
    launchType: 'exe',
    launchTarget: 'C:\\Games\\x.exe',
    launchArgs: '',
    ...over,
  } as EditDraft
}

// Rune count (code points), as the server counts (EDT-001).
function runes(n: number, ch = 'a'): string {
  return Array.from({ length: n }, () => ch).join('')
}

describe('validateDraft', () => {
  it('UI-040/título vazio depois de cortar, ano fora de 1900..2100, data que não é AAAA-MM-DD e descrição acima de 10 000 geram mensagem', () => {
    expect(validateDraft(draft({ title: '' }))).toHaveProperty('title')
    expect(validateDraft(draft({ title: '   \t ' }))).toHaveProperty('title')
    expect(validateDraft(draft({ title: runes(201) }))).toHaveProperty('title')

    for (const year of ['1899', '2101', '0', 'abc', '2017.5']) {
      expect(validateDraft(draft({ year })), `year ${year}`).toHaveProperty('year')
    }

    for (const completedAt of ['12/11/2026', '2026-11-1', '2026-1-01', '2026-11-12T00:00:00Z', 'ontem']) {
      expect(validateDraft(draft({ completedAt })), `completedAt ${completedAt}`).toHaveProperty('completedAt')
    }

    expect(validateDraft(draft({ description: runes(10001) }))).toHaveProperty('description')

    // Each message is a non-empty text, as the form shows it.
    const errs = validateDraft(draft({ title: '', year: '1899', description: runes(10001) }))
    expect(errs.title).toEqual(expect.any(String))
    expect(errs.title).not.toBe('')
    expect(errs.year).toEqual(expect.any(String))
    expect(errs.description).toEqual(expect.any(String))
  })

  it('UI-040/limites: título de 200, descrição de 10 000, ano 1900 e 2100, ano e data vazios passam', () => {
    expect(validateDraft(draft({ title: runes(200) }))).toEqual({})
    expect(validateDraft(draft({ description: runes(10000) }))).toEqual({})
    expect(validateDraft(draft({ description: '' }))).toEqual({})
    expect(validateDraft(draft({ year: '1900' }))).toEqual({})
    expect(validateDraft(draft({ year: '2100' }))).toEqual({})
    expect(validateDraft(draft({ year: '' }))).toEqual({})
    expect(validateDraft(draft({ completedAt: '' }))).toEqual({})
    expect(validateDraft(draft({ completedAt: '2026-11-12' }))).toEqual({})
  })

  it('UI-040/o limite do título conta runas, não unidades UTF-16 (200 emojis passam, 201 falham)', () => {
    expect(validateDraft(draft({ title: runes(200, '🎮') }))).toEqual({})
    expect(validateDraft(draft({ title: runes(201, '🎮') }))).toHaveProperty('title')
  })

  it('UI-040/nome em branco em desenvolvedora, editora ou gênero gera mensagem', () => {
    const cases: Array<[keyof EditDraft, Partial<EditDraft>]> = [
      ['developers', { developers: ['Team Cherry', '  '] }],
      ['publishers', { publishers: [''] }],
      ['genres', { genres: ['\t'] }],
    ]
    for (const [field, over] of cases) {
      expect(validateDraft(draft(over)), field).toHaveProperty(field)
    }
  })

  it('UI-040/status fora dos cinco valores gera mensagem', () => {
    for (const status of ['not_played', 'playing', 'completed', 'abandoned', 'backlog']) {
      expect(validateDraft(draft({ status: status as EditDraft['status'] })), status).toEqual({})
    }
    expect(validateDraft(draft({ status: 'done' as unknown as EditDraft['status'] }))).toHaveProperty('status')
  })

  it('UI-040/rascunho válido devolve objeto vazio', () => {
    expect(validateDraft(draft())).toEqual({})
  })
})
