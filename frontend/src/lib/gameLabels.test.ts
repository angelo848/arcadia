import { describe, expect, it } from 'vitest'
import { playModeLabels as fromLibrary } from './library'
import { playModeLabels, statusLabels } from './gameLabels'

describe('gameLabels (fase 17)', () => {
  it('TDK-008/statusLabels cobre not_played, playing, completed, abandoned e backlog', () => {
    expect(statusLabels).toEqual({
      not_played: 'Não jogado',
      playing: 'Jogando',
      completed: 'Concluído',
      abandoned: 'Abandonado',
      backlog: 'Na fila',
    })
  })

  it('TDK-008/playModeLabels reexportado é o mesmo objeto de lib/library', () => {
    expect(playModeLabels).toBe(fromLibrary)
  })
})
