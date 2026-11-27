import type { PlatformView } from './library'

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
export function platformIconSrc(p: Pick<PlatformView, 'seedKey'> & { iconUrl: string | null }): string {
  if (p.iconUrl !== null && p.iconUrl !== '') {
    return p.iconUrl
  }
  if (p.seedKey !== null && Object.prototype.hasOwnProperty.call(seedIconUrls, p.seedKey)) {
    return seedIconUrls[p.seedKey]
  }
  return genericIconUrl
}
