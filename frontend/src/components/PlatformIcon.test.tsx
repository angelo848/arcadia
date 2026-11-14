import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { PlatformIcon } from './PlatformIcon'
import { genericIconUrl, seedIconUrls } from '../lib/platforms'
import type { PlatformView } from '../lib/library'

const seedKeys = ['windows', '3ds', 'n64', 'nes', 'gba', 'gamecube', 'switch', 'switch2', 'wii', 'ps1', 'ps2', 'ps3', 'snes']

function platform(over: Partial<PlatformView> = {}): PlatformView {
  return {
    id: 1,
    name: 'Qualquer',
    category: 'console',
    emulatorId: null,
    seedKey: null,
    iconUrl: '',
    gameCount: 0,
    ...over,
  }
}

function imgOf(container: HTMLElement): HTMLImageElement {
  const img = container.querySelector('img')
  expect(img, 'PlatformIcon renders one <img>').not.toBeNull()
  return img as HTMLImageElement
}

describe('PlatformIcon', () => {
  it('PLT-009/cada uma das 13 chaves tem ícone embutido', () => {
    const srcs = new Set<string>()
    for (const key of seedKeys) {
      expect(seedIconUrls[key], `ícone de ${key}`).toBeTruthy()
      const { container, unmount } = render(<PlatformIcon platform={platform({ seedKey: key })} />)
      expect(imgOf(container).getAttribute('src')).toBe(seedIconUrls[key])
      srcs.add(seedIconUrls[key])
      unmount()
    }
    // Every key has its own drawing, none is the generic one.
    expect(srcs.size).toBe(13)
    expect(srcs.has(genericIconUrl)).toBe(false)
    expect(Object.keys(seedIconUrls).sort()).toEqual([...seedKeys].sort())
  })

  it('PLT-010/sem seedKey mostra o genérico', () => {
    const { container } = render(<PlatformIcon platform={platform({ seedKey: null })} />)
    expect(genericIconUrl).toBeTruthy()
    expect(imgOf(container).getAttribute('src')).toBe(genericIconUrl)
  })

  it('PLT-010/chave desconhecida mostra o genérico', () => {
    const { container } = render(<PlatformIcon platform={platform({ seedKey: 'dreamcast' })} />)
    expect(imgOf(container).getAttribute('src')).toBe(genericIconUrl)
  })

  it('PLT-011/iconUrl preenchido ganha do ícone embutido', () => {
    const url = '/media/platforms/3f9a.png'
    const { container } = render(<PlatformIcon platform={platform({ seedKey: 'snes', iconUrl: url })} />)
    expect(imgOf(container).getAttribute('src')).toBe(url)
  })

  it('PLT-011/iconUrl preenchido sem chave também é usado', () => {
    const url = '/media/platforms/aa11.jpg'
    const { container } = render(<PlatformIcon platform={platform({ seedKey: null, iconUrl: url })} />)
    expect(imgOf(container).getAttribute('src')).toBe(url)
  })

  it('PLT-009/size define a largura e a altura do ícone', () => {
    const { container } = render(<PlatformIcon platform={platform({ seedKey: 'wii' })} size={40} />)
    const img = imgOf(container)
    expect(img.getAttribute('width')).toBe('40')
    expect(img.getAttribute('height')).toBe('40')
  })
})
