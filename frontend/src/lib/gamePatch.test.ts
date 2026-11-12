import { describe, expect, it } from 'vitest'
import { diffPatch } from './gamePatch'
import type { GameDetailView } from './library'

// The draft type is the one diffPatch takes; the test does not depend on the module that exports it.
type EditDraft = Parameters<typeof diffPatch>[1]

const original: GameDetailView = {
  id: 10,
  platformId: 1,
  title: 'Hollow Knight',
  platform: 'PC',
  path: 'C:\\Games\\HK\\hollow_knight.exe',
  status: 'playing',
  favorite: true,
  year: 2017,
  totalPlaySeconds: 3900,
  playCount: 4,
  firstPlayedAt: '',
  lastPlayedAt: '',
  description: 'Um metroidvania difícil.',
  completedAt: '2026-11-12',
  developers: ['Team Cherry'],
  publishers: ['Team17'],
  genres: ['Metroidvania', 'Plataforma'],
  playModes: ['single'],
  launchType: 'exe',
  launchArgs: '',
  emulatorId: null,
}

// draftOf is the untouched draft of original: every field equals the stored value.
function draftOf(over: Partial<EditDraft> = {}): EditDraft {
  return {
    title: 'Hollow Knight',
    description: 'Um metroidvania difícil.',
    year: '2017',
    completedAt: '2026-11-12',
    status: 'playing',
    favorite: true,
    developers: ['Team Cherry'],
    publishers: ['Team17'],
    genres: ['Metroidvania', 'Plataforma'],
    playModes: ['single'],
    launchType: 'exe',
    launchTarget: 'C:\\Games\\HK\\hollow_knight.exe',
    launchArgs: '',
    emulatorId: null,
    ...over,
  } as EditDraft
}

describe('diffPatch', () => {
  it('UI-039/diffPatch envia só os campos alterados', () => {
    // Nothing changed: an empty patch (EDT-005 says it does not touch updated_at).
    expect(diffPatch(original, draftOf())).toEqual({})

    // Only the title changed: the patch carries the title and nothing else.
    expect(diffPatch(original, draftOf({ title: 'Hollow Knight GOTY' }))).toEqual({
      title: 'Hollow Knight GOTY',
    })

    // year is sent as a number (IPC-052 rejects text).
    expect(diffPatch(original, draftOf({ year: '2018' }))).toEqual({ year: 2018 })

    // Several fields at once, each with its own type: status string, favorite boolean, description string.
    expect(
      diffPatch(
        original,
        draftOf({ status: 'completed', favorite: false, description: 'Outra descrição' }),
      ),
    ).toEqual({ status: 'completed', favorite: false, description: 'Outra descrição' })

    // Arrays go whole when their content changed; equal arrays do not enter.
    expect(diffPatch(original, draftOf({ developers: ['Team Cherry', 'Outra'] }))).toEqual({
      developers: ['Team Cherry', 'Outra'],
    })
    expect(diffPatch(original, draftOf({ genres: ['Metroidvania'] }))).toEqual({ genres: ['Metroidvania'] })
    expect(diffPatch(original, draftOf({ playModes: ['single', 'coop_local'] }))).toEqual({
      playModes: ['single', 'coop_local'],
    })

    // Unchanged members must not be present, not even as undefined.
    const patch = diffPatch(original, draftOf({ title: 'Outro' }))
    expect(patch).not.toHaveProperty('description')
    expect(patch).not.toHaveProperty('developers')
    expect(patch).not.toHaveProperty('completedAt')
  })

  it('UI-039/completedAt apagado vira null; campo igual não entra', () => {
    // Erasing a set completion date sends null (IPC-052: null clears, absent does not touch).
    expect(diffPatch(original, draftOf({ completedAt: '' }))).toEqual({ completedAt: null })

    // Same date as stored: not in the patch.
    expect(diffPatch(original, draftOf({ completedAt: '2026-11-12' }))).not.toHaveProperty('completedAt')

    // Nothing stored and nothing typed: not in the patch, and not sent as null.
    const noDate: GameDetailView = { ...original, completedAt: null }
    expect(diffPatch(noDate, draftOf({ completedAt: '' }))).not.toHaveProperty('completedAt')

    // A new date is sent as YYYY-MM-DD; the server turns it into midnight UTC (EDT-001).
    expect(diffPatch(noDate, draftOf({ completedAt: '2026-11-13' }))).toEqual({ completedAt: '2026-11-13' })

    // The stored value may come as a full instant; the same calendar day (UTC) is still equal.
    const instant: GameDetailView = { ...original, completedAt: '2026-11-12T00:00:00Z' }
    expect(diffPatch(instant, draftOf({ completedAt: '2026-11-12' }))).not.toHaveProperty('completedAt')
  })

  it('UI-039/year apagado vira null e year igual ao gravado não entra', () => {
    expect(diffPatch(original, draftOf({ year: '' }))).toEqual({ year: null })
    expect(diffPatch(original, draftOf({ year: '2017' }))).not.toHaveProperty('year')

    const noYear: GameDetailView = { ...original, year: null }
    expect(diffPatch(noYear, draftOf({ year: '' }))).not.toHaveProperty('year')
    expect(diffPatch(noYear, draftOf({ year: '1998' }))).toEqual({ year: 1998 })
  })
})
