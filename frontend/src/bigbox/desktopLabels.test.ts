// T05: TDK-008 (rótulos do painel do tema Desktop, mesmos textos de GameDetailsPanel).
import { describe, expect, it } from 'vitest'
import { formatLocalDateTime, formatUtcDate } from '../lib/formatDate'
import { formatPlayTime } from '../lib/games'
import type { GameDetailView } from '../lib/library'
import { desktopLabels } from './desktopLabels'

const detail = (o: Partial<GameDetailView> = {}): GameDetailView =>
  ({
    id: 1, platformId: 1, title: 'Zelda', platform: 'Nintendo 64', path: 'x', status: 'playing', favorite: false, year: 1998,
    totalPlaySeconds: 7260, playCount: 3, firstPlayedAt: '2026-01-02T10:00:00Z', lastPlayedAt: '2026-11-20T21:30:00Z',
    description: 'd', emulatorId: null, launchType: 'exe', launchArgs: '', sessionPlaySeconds: 0, completedAt: null,
    developers: ['Nintendo EAD'], publishers: ['Nintendo'], genres: ['Ação', 'Aventura'], playModes: ['single', 'coop_local'],
    ...o,
  }) as unknown as GameDetailView

describe('desktopLabels (fase 17)', () => {
  it('TDK-008/status, tempo total, primeira e última vez, modos, gêneros, desenvolvedora, editora e ano', () => {
    const l = desktopLabels(detail())
    expect(l.platform).toBe('Nintendo 64')
    expect(l.status).toBe('Jogando')
    expect(l.totalTime).toBe(formatPlayTime(7260))
    expect(l.firstPlayed).toBe(formatLocalDateTime('2026-01-02T10:00:00Z'))
    expect(l.lastPlayed).toBe(formatLocalDateTime('2026-11-20T21:30:00Z'))
    expect(l.genres).toContain('Ação')
    expect(l.genres).toContain('Aventura')
    expect(l.playModes).toContain('Um jogador')
    expect(l.playModes).toContain('Cooperativo local')
    expect(l.developers).toBe('Nintendo EAD')
    expect(l.publishers).toBe('Nintendo')
    expect(l.year).toBe('1998')
  })

  it('TDK-008/"Concluído em" só aparece quando há data', () => {
    expect(desktopLabels(detail()).completedAt).toBeNull()
    expect(desktopLabels(detail({ completedAt: '2026-05-06T00:00:00Z' })).completedAt).toBe(formatUtcDate('2026-05-06T00:00:00Z'))
  })

  it('TDK-008/vazio vira "—"', () => {
    const l = desktopLabels(detail({ genres: [], playModes: [], developers: [], publishers: [], year: null }))
    expect(l.genres).toBe('—')
    expect(l.playModes).toBe('—')
    expect(l.developers).toBe('—')
    expect(l.publishers).toBe('—')
    expect(l.year).toBe('—')
  })

  it('TDK-008/nunca jogado usa os mesmos textos da janela normal', () => {
    const l = desktopLabels(detail({ totalPlaySeconds: 0, firstPlayedAt: '', lastPlayedAt: '' }))
    expect(l.totalTime).toBe('Nunca jogado')
    expect(l.firstPlayed).toBe('Nunca')
    expect(l.lastPlayed).toBe('Nunca')
  })

  it('TDK-008/favorito', () => {
    expect(desktopLabels(detail({ favorite: true })).favorite).toBe(true)
    expect(desktopLabels(detail({ favorite: false })).favorite).toBe(false)
  })
})
