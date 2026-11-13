import { describe, expect, it } from 'vitest'
import { acceptedScore, scoreLabel } from './metadata'

describe('metadata helpers', () => {
  it('MID-002/a pontuação aparece como porcentagem inteira', () => {
    expect(scoreLabel(0.95)).toBe('95%')
    expect(scoreLabel(0.4)).toBe('40%')
  })

  it('MID-002/o limiar da UI é o mesmo do agente (0,80)', () => {
    expect(acceptedScore).toBe(0.8)
  })
})
