import { describe, expect, it } from 'vitest'
import { acceptedScore, orderMedia, scoreLabel } from './metadata'
import type { MediaItemView } from './metadata'

describe('metadata helpers', () => {
  it('MID-002/a pontuação aparece como porcentagem inteira', () => {
    expect(scoreLabel(0.95)).toBe('95%')
    expect(scoreLabel(0.4)).toBe('40%')
  })

  it('MID-002/o limiar da UI é o mesmo do agente (0,80)', () => {
    expect(acceptedScore).toBe(0.8)
  })
})

describe('orderMedia', () => {
  const item = (id: number, kind: MediaItemView['kind']): MediaItemView => ({
    id,
    kind,
    provider: 'p',
    path: '',
    thumbPath: '',
    url: `/m/${id}`,
    thumbUrl: `/t/${id}`,
  })

  it('UIX-026/orderMedia põe as capas primeiro e mantém a ordem das demais', () => {
    const input = [item(1, 'screenshot'), item(2, 'cover'), item(3, 'banner'), item(4, 'cover'), item(5, 'title')]
    expect(orderMedia(input).map((m) => m.id)).toEqual([2, 4, 1, 3, 5])
    expect(input.map((m) => m.id)).toEqual([1, 2, 3, 4, 5])
    expect(orderMedia([])).toEqual([])
  })
})
