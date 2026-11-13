import { describe, expect, it } from 'vitest'
import { needsPath, sourceLabels } from './imports'

describe('import sources (IMP-004)', () => {
  it('IMP-004/a pasta de jogos é uma fonte com caminho escolhido pelo usuário', () => {
    expect(sourceLabels.folder).toBe('Pasta de jogos')
    expect(needsPath('folder')).toBe(true)
    expect(needsPath('file')).toBe(true)
    expect(needsPath('steam')).toBe(false)
  })
})
