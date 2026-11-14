import { describe, expect, it } from 'vitest'
import { acceptedScore, cardFallbackOrder, cardImageOf, carouselKindOrder, orderMedia, scoreLabel } from './metadata'
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

describe('carrossel e imagem do cartão', () => {
  const item = (id: number, kind: MediaItemView['kind'], thumbUrl = `/t/${id}`): MediaItemView => ({
    id,
    kind,
    provider: 'p',
    path: '',
    thumbPath: '',
    url: `/m/${id}`,
    thumbUrl,
  })

  it('AJU-009/orderMedia com tipos misturados dá capa, screenshots, título, seleção, fundo, banner, mantendo a ordem dentro do tipo, sem alterar a entrada', () => {
    const input = [
      item(1, 'banner'),
      item(2, 'screenshot'),
      item(3, 'background'),
      item(4, 'cover'),
      item(5, 'select'),
      item(6, 'screenshot'),
      item(7, 'title'),
      item(8, 'cover'),
    ]
    expect(orderMedia(input).map((m) => m.id)).toEqual([4, 8, 2, 6, 7, 5, 3, 1])
    expect(input.map((m) => m.id)).toEqual([1, 2, 3, 4, 5, 6, 7, 8])
    expect(orderMedia([])).toEqual([])
    expect(carouselKindOrder).toEqual(['cover', 'screenshot', 'title', 'select', 'background', 'banner'])
  })

  it('AJU-015/com capa: cardImageOf devolve a primeira capa com fit contain', () => {
    expect(cardImageOf([item(1, 'title'), item(2, 'cover'), item(3, 'cover')])).toEqual({ url: '/t/2', fit: 'contain' })
  })

  it('AJU-015/sem capa: título vence seleção, screenshot, banner e fundo; seleção vence screenshot; screenshot vence banner', () => {
    expect(cardFallbackOrder).toEqual(['title', 'select', 'screenshot', 'banner', 'background'])
    const all = [item(1, 'background'), item(2, 'banner'), item(3, 'screenshot'), item(4, 'select'), item(5, 'title')]
    expect(cardImageOf(all)).toEqual({ url: '/t/5', fit: 'cover' })
    expect(cardImageOf(all.filter((m) => m.kind !== 'title'))?.url).toBe('/t/4')
    expect(cardImageOf(all.filter((m) => m.kind !== 'title' && m.kind !== 'select'))?.url).toBe('/t/3')
    expect(cardImageOf([item(1, 'banner'), item(3, 'screenshot')])?.url).toBe('/t/3')
  })

  it('AJU-015/só banner e fundo: vence o banner (fit cover)', () => {
    expect(cardImageOf([item(1, 'background'), item(2, 'banner')])).toEqual({ url: '/t/2', fit: 'cover' })
  })

  it('AJU-015/lista vazia dá undefined; thumbUrl vazio usa url', () => {
    expect(cardImageOf([])).toBeUndefined()
    expect(cardImageOf([item(1, 'cover', '')])).toEqual({ url: '/m/1', fit: 'contain' })
  })
})
