import type { PlatformView } from './library'

export type { PlatformAddView } from './library'

// The bundled icons, one SVG per seed key plus the generic one (PLT-009, PLT-010).
const iconModules = import.meta.glob('../assets/platforms/*.svg', {
  query: '?url',
  import: 'default',
  eager: true,
}) as Record<string, string>

function iconUrlOf(key: string): string {
  return iconModules[`../assets/platforms/${key}.svg`] ?? ''
}

// The keys of the seed platforms (contracts/seed-platforms.md §1).
const seedKeys = ['windows', '3ds', 'n64', 'nes', 'gba', 'gamecube', 'switch', 'switch2', 'wii', 'ps1', 'ps2', 'ps3', 'snes']

// seedIconUrls maps each seed key to its bundled SVG (PLT-009).
export const seedIconUrls: Record<string, string> = Object.fromEntries(seedKeys.map((k) => [k, iconUrlOf(k)]))

// genericIconUrl is the icon of a platform without seed key or custom icon (PLT-010).
export const genericIconUrl: string = iconUrlOf('generic')

// platformIconSrc picks the icon of a platform: the custom one, else the bundled one of its key, else the generic one
// (PLT-009..PLT-011).
export function platformIconSrc(p: Pick<PlatformView, 'seedKey' | 'iconUrl'>): string {
  if (p.iconUrl !== '') {
    return p.iconUrl
  }
  if (p.seedKey !== null && Object.prototype.hasOwnProperty.call(seedIconUrls, p.seedKey)) {
    return seedIconUrls[p.seedKey]
  }
  return genericIconUrl
}

type WailsApp = {
  PlatformUpdate?: (id: number, name: string, category: string) => Promise<PlatformView>
  PlatformSetIcon?: (id: number, path: string | null) => Promise<PlatformView>
  PlatformDelete?: (id: number) => Promise<void>
  PickPlatformIcon?: () => Promise<string>
}

type WailsWindow = { go?: { main?: { App?: WailsApp } } }

// Wails v2 rejects bound calls with a plain string; normalize to Error (UI-008).
async function call<T>(name: keyof WailsApp, args: unknown[]): Promise<T> {
  const fn = (window as unknown as WailsWindow).go?.main?.App?.[name] as ((...a: unknown[]) => Promise<T>) | undefined
  if (!fn) {
    throw new Error(`${name} indisponível`)
  }
  try {
    return await fn(...args)
  } catch (e) {
    throw e instanceof Error ? e : new Error(String(e))
  }
}

// updatePlatform renames a platform and sets its category (PLT-018, PLT-019).
export function updatePlatform(id: number, name: string, category: string): Promise<PlatformView> {
  return call<PlatformView>('PlatformUpdate', [id, name, category])
}

// setPlatformIcon sets the icon from an image file; null goes back to the bundled icon (PLT-011, PLT-013).
export function setPlatformIcon(id: number, path: string | null): Promise<PlatformView> {
  return call<PlatformView>('PlatformSetIcon', [id, path])
}

// deletePlatform removes a platform without games (PLT-020).
export function deletePlatform(id: number): Promise<void> {
  return call<void>('PlatformDelete', [id])
}

// pickPlatformIcon opens the Explorer dialog for the icon image; "" means cancelled (PLT-023).
export function pickPlatformIcon(): Promise<string> {
  return call<string>('PickPlatformIcon', [])
}
