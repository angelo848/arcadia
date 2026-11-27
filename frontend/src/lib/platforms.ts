import type { PlatformView } from './library'

export type { PlatformAddView } from './library'

// The icon helpers live in platformIcons (no write functions); re-exported for the current imports.
export { genericIconUrl, platformIconSrc, seedIconUrls } from './platformIcons'

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
