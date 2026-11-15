import type { ComponentType } from 'react'

export type BigBoxThemeDef = {
  id: 'faixa' | 'mosaico' | 'roda'
  name: string
  available: boolean
  axis: 'horizontal' | 'vertical'
  // The Faixa component arrives in T37; until then it is absent.
  Component?: ComponentType
}

// Mutable on purpose: tests register a fake theme [BBX-024].
export const BIGBOX_THEMES: BigBoxThemeDef[] = [
  { id: 'faixa', name: 'Faixa', available: true, axis: 'horizontal' },
  { id: 'mosaico', name: 'Mosaico (preparado)', available: false, axis: 'horizontal' },
  { id: 'roda', name: 'Roda (preparado)', available: false, axis: 'horizontal' },
]

// resolveTheme reads the registry on every call; unknown or unavailable ids fall back to faixa [BBX-023].
export function resolveTheme(id: string | null | undefined): BigBoxThemeDef {
  const found = BIGBOX_THEMES.find((t) => t.id === id && t.available)
  return found ?? BIGBOX_THEMES[0]
}
