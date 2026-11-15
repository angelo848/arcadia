import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  acceptedScore,
  applyResultLabel,
  cardFallbackOrder,
  cardImageOf,
  carouselKindOrder,
  metadataApply,
  metadataSearch,
  orderMedia,
  providersLabel,
  scoreLabel,
} from './metadata'
import type {
  MediaItemView,
  MetadataApplyView,
  MetadataCandidateView,
  MetadataRefView,
  MetadataSearchView,
} from './metadata'

// Bindings Wails do desktop (contracts/ipc-v0.14.0-delta.md §7): window.go.main.App.<Nome>.
function install(app: Record<string, unknown>) {
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
}

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

const candidate = (over: Partial<MetadataCandidateView> = {}): MetadataCandidateView => ({
  refs: [{ provider: 'igdb', providerId: '1942' }],
  title: 'Jogo',
  year: 2001,
  platform: 'Windows',
  score: 0.9,
  ...over,
})

describe('metadata helpers', () => {
  it('MID-002/a pontuação aparece como porcentagem inteira', () => {
    expect(scoreLabel(0.95)).toBe('95%')
    expect(scoreLabel(0.4)).toBe('40%')
  })

  it('MID-002/o limiar da UI é o mesmo do agente (0,80)', () => {
    expect(acceptedScore).toBe(0.8)
  })
})

describe('MET-044 rótulos da lista de opções', () => {
  it('MET-044/providersLabel junta os nomes na ordem igdb, steamgriddb, steamstore com " · "', () => {
    const refs: MetadataRefView[] = [
      { provider: 'steamstore', providerId: '10' },
      { provider: 'igdb', providerId: '1942' },
      { provider: 'steamgriddb', providerId: '77' },
    ]
    expect(providersLabel(refs)).toBe('IGDB · SteamGridDB · Steam Store')
  })

  it('MET-044/providersLabel com um provedor só não tem separador', () => {
    expect(providersLabel([{ provider: 'steamgriddb', providerId: '77' }])).toBe('SteamGridDB')
  })

  it('MET-044/providersLabel sem refs devolve string vazia', () => {
    expect(providersLabel([])).toBe('')
  })

  it('MET-044/scoreLabel em percentual arredondado', () => {
    expect(scoreLabel(1)).toBe('100%')
    expect(scoreLabel(0)).toBe('0%')
    expect(scoreLabel(0.956)).toBe('96%')
    expect(scoreLabel(0.954)).toBe('95%')
  })
})

describe('MET-024 texto do resultado da aplicação', () => {
  it('MET-024/applyResultLabel: sem pulados mostra só a fila', () => {
    const text = applyResultLabel(3, 0)
    expect(text).toBe('3 imagens na fila')
    expect(text).not.toContain('não entraram')
  })

  it('MET-024/applyResultLabel: com pulados mostra a segunda frase', () => {
    expect(applyResultLabel(4, 2)).toBe('4 imagens na fila; 2 não entraram porque o jogo já tem o máximo')
  })

  it('MET-024/applyResultLabel: singular na fila e nos pulados', () => {
    // Premissa: a spec não fixa a forma singular; usa-se a gramática natural pt-BR ("1 imagem", "1 não entrou").
    expect(applyResultLabel(1, 0)).toBe('1 imagem na fila')
    expect(applyResultLabel(1, 1)).toBe('1 imagem na fila; 1 não entrou porque o jogo já tem o máximo')
  })

  it('MET-024/applyResultLabel: zero na fila é plural', () => {
    expect(applyResultLabel(0, 0)).toBe('0 imagens na fila')
  })
})

describe('MET-043 chamadas unificadas ao binding', () => {
  const searchView: MetadataSearchView = {
    candidates: [candidate()],
    confidence: 'match',
    warnings: [],
  }

  it('MET-043/metadataSearch repassa título, plataforma e ano ao binding', async () => {
    const fn = vi.fn(() => Promise.resolve(searchView))
    install({ MetadataSearch: fn })

    const got = await metadataSearch('Jogo Teste', 'Windows', 2001)

    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith('Jogo Teste', 'Windows', 2001)
    expect(got).toEqual(searchView)
  })

  it('MET-043/metadataSearch sem ano envia 0 ao binding', async () => {
    const fn = vi.fn(() => Promise.resolve(searchView))
    install({ MetadataSearch: fn })

    await metadataSearch('Jogo Teste', '', 0)

    expect(fn).toHaveBeenCalledWith('Jogo Teste', '', 0)
  })

  it('MET-043/binding ausente falha como chamada com erro', async () => {
    delete (window as unknown as { go?: unknown }).go

    await expect(metadataSearch('Jogo', 'Windows', 0)).rejects.toThrow(Error)
  })

  it('MET-043/metadataApply repassa gameId, refs e replace', async () => {
    const refs: MetadataRefView[] = [
      { provider: 'igdb', providerId: '1942' },
      { provider: 'steamgriddb', providerId: '77' },
    ]
    const result = { game: { id: 5 }, queued: 2, skippedByLimit: 0 } as unknown as MetadataApplyView
    const fn = vi.fn(() => Promise.resolve(result))
    install({ MetadataApply: fn })

    const got = await metadataApply(5, refs, true)

    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith(5, refs, true)
    expect(got).toEqual(result)
  })

  it('MET-043/metadataApply com replace false repassa false, não omite o argumento', async () => {
    const fn = vi.fn(() => Promise.resolve({ game: {}, queued: 0, skippedByLimit: 0 } as unknown as MetadataApplyView))
    install({ MetadataApply: fn })

    await metadataApply(5, [{ provider: 'igdb', providerId: '1942' }], false)

    expect(fn).toHaveBeenCalledWith(5, [{ provider: 'igdb', providerId: '1942' }], false)
  })

  it('MET-043/metadataApply com binding ausente falha como chamada com erro', async () => {
    delete (window as unknown as { go?: unknown }).go

    await expect(metadataApply(5, [{ provider: 'igdb', providerId: '1942' }], false)).rejects.toThrow(Error)
  })

  it('MET-043/metadataApply propaga o erro do binding', async () => {
    install({ MetadataApply: () => Promise.reject(new Error('jogo não encontrado')) })

    await expect(metadataApply(5, [{ provider: 'igdb', providerId: '1942' }], false)).rejects.toThrow('jogo não encontrado')
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
