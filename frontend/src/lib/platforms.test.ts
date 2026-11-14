import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  deletePlatform,
  genericIconUrl,
  pickPlatformIcon,
  platformIconSrc,
  seedIconUrls,
  setPlatformIcon,
  updatePlatform,
} from './platforms'
import type { PlatformView } from './library'

const seedKeys = ['windows', '3ds', 'n64', 'nes', 'gba', 'gamecube', 'switch', 'switch2', 'wii', 'ps1', 'ps2', 'ps3', 'snes']

const svgSources = import.meta.glob('../assets/platforms/*.svg', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

function view(over: Partial<PlatformView> = {}): PlatformView {
  return { id: 3, name: 'Sega', category: 'console', emulatorId: null, seedKey: null, iconUrl: '', gameCount: 0, ...over }
}

describe('ícones embutidos', () => {
  it('PLT-009/cada uma das 13 chaves tem ícone embutido', () => {
    expect(Object.keys(seedIconUrls).sort()).toEqual([...seedKeys].sort())
    for (const key of seedKeys) {
      expect(seedIconUrls[key], key).toBeTruthy()
      expect(platformIconSrc({ seedKey: key, iconUrl: '' })).toBe(seedIconUrls[key])
    }
  })

  it('PLT-009/SVGs com viewBox 0 0 64 64 e sem <text>', () => {
    const names = Object.keys(svgSources)
      .map((p) => p.split('/').pop() as string)
      .sort()
    expect(names).toEqual([...seedKeys, 'generic'].map((k) => `${k}.svg`).sort())
    for (const [path, src] of Object.entries(svgSources)) {
      expect(src, path).toContain('viewBox="0 0 64 64"')
      expect(src, path).not.toMatch(/<text[\s>]/)
      expect(src, path).toMatch(/<path[\s>]/)
    }
  })

  it('PLT-010/sem seedKey ou com chave desconhecida mostra o genérico', () => {
    expect(genericIconUrl).toBeTruthy()
    expect(platformIconSrc({ seedKey: null, iconUrl: '' })).toBe(genericIconUrl)
    expect(platformIconSrc({ seedKey: 'dreamcast', iconUrl: '' })).toBe(genericIconUrl)
  })

  it('PLT-011/iconUrl preenchido ganha do ícone embutido', () => {
    expect(platformIconSrc({ seedKey: 'snes', iconUrl: '/media/platforms/x.png' })).toBe('/media/platforms/x.png')
    expect(platformIconSrc({ seedKey: null, iconUrl: '/media/platforms/x.png' })).toBe('/media/platforms/x.png')
  })
})

type Fn = ReturnType<typeof vi.fn>
type FakeApp = { PlatformUpdate: Fn; PlatformSetIcon: Fn; PlatformDelete: Fn; PickPlatformIcon: Fn }
let app: FakeApp

beforeEach(() => {
  app = {
    PlatformUpdate: vi.fn((id: number, name: string, category: string) => Promise.resolve(view({ id, name, category }))),
    PlatformSetIcon: vi.fn((id: number) => Promise.resolve(view({ id }))),
    PlatformDelete: vi.fn(() => Promise.resolve()),
    PickPlatformIcon: vi.fn(() => Promise.resolve('C:\\Imagens\\icone.png')),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
})

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('chamadas ao agente', () => {
  it('PLT-018/updatePlatform chama PlatformUpdate com id, nome e categoria e devolve a view', async () => {
    const got = await updatePlatform(3, 'Mega Drive', 'handheld')
    expect(app.PlatformUpdate).toHaveBeenCalledWith(3, 'Mega Drive', 'handheld')
    expect(got).toEqual(view({ id: 3, name: 'Mega Drive', category: 'handheld' }))
  })

  it('PLT-013/setPlatformIcon com null envia null (restaurar padrão)', async () => {
    await setPlatformIcon(3, null)
    expect(app.PlatformSetIcon).toHaveBeenCalledWith(3, null)
  })

  it('PLT-011/setPlatformIcon envia o caminho', async () => {
    await setPlatformIcon(3, 'C:\\Imagens\\icone.png')
    expect(app.PlatformSetIcon).toHaveBeenCalledWith(3, 'C:\\Imagens\\icone.png')
  })

  it('PLT-020/deletePlatform chama PlatformDelete com o id', async () => {
    await deletePlatform(3)
    expect(app.PlatformDelete).toHaveBeenCalledWith(3)
  })

  it('PLT-023/pickPlatformIcon devolve o caminho escolhido e "" quando cancelado', async () => {
    await expect(pickPlatformIcon()).resolves.toBe('C:\\Imagens\\icone.png')
    app.PickPlatformIcon.mockResolvedValueOnce('')
    await expect(pickPlatformIcon()).resolves.toBe('')
  })

  it('PLT-021/rejeição em string vira Error com a mesma mensagem', async () => {
    const msg = 'Esta plataforma tem 3 jogo(s). Mova-os para outra plataforma ou exclua-os antes.'
    app.PlatformDelete.mockRejectedValueOnce(msg)
    const err = await deletePlatform(3).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(Error)
    expect((err as Error).message).toBe(msg)
  })

  it('PLT-012/rejeição de PlatformSetIcon vira Error com o texto do motivo', async () => {
    app.PlatformSetIcon.mockRejectedValueOnce('O arquivo passa de 2 MB.')
    const err = await setPlatformIcon(3, 'C:\\grande.png').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(Error)
    expect((err as Error).message).toBe('O arquivo passa de 2 MB.')
  })

  it('PLT-018/sem os bindings do Wails a chamada rejeita', async () => {
    delete (window as unknown as { go?: unknown }).go
    await expect(updatePlatform(1, 'X', 'pc')).rejects.toThrow()
    await expect(deletePlatform(1)).rejects.toThrow()
  })
})
