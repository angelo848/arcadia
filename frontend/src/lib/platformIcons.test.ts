import { describe, expect, it } from 'vitest'
import * as platforms from './platforms'
import { genericIconUrl, platformIconSrc, seedIconUrls } from './platformIcons'

describe('platformIcons (fase 17)', () => {
  it('BBX-066/ícone personalizado (iconUrl) vence', () => {
    expect(platformIconSrc({ seedKey: 'nes', iconUrl: 'data:image/png;base64,AAAA' })).toBe('data:image/png;base64,AAAA')
  })

  it('BBX-066/seedKey conhecido devolve o SVG embutido', () => {
    const key = Object.keys(seedIconUrls)[0]
    expect(platformIconSrc({ seedKey: key, iconUrl: null })).toBe(seedIconUrls[key])
  })

  it('BBX-066/sem seedKey ou chave desconhecida devolve o genérico', () => {
    expect(platformIconSrc({ seedKey: null, iconUrl: null })).toBe(genericIconUrl)
    expect(platformIconSrc({ seedKey: 'nao-existe', iconUrl: null })).toBe(genericIconUrl)
    expect(genericIconUrl).not.toBe('')
  })

  it('BBX-066/seedIconUrls tem as 13 chaves da semente', () => {
    expect(Object.keys(seedIconUrls)).toHaveLength(13)
  })

  it('BBX-066/lib/platforms reexporta as mesmas referências (platformIconSrc, seedIconUrls, genericIconUrl)', () => {
    expect(platforms.platformIconSrc).toBe(platformIconSrc)
    expect(platforms.seedIconUrls).toBe(seedIconUrls)
    expect(platforms.genericIconUrl).toBe(genericIconUrl)
  })
})
