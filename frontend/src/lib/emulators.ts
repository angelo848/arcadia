import { useQuery } from '@tanstack/react-query'
import type { PlatformView } from './library'

// EmulatorView mirrors cmd/arcadia EmulatorView (EMU-001).
export type EmulatorView = {
  id: number
  name: string
  exePath: string
  argsTemplate: string
}

// Markers an emulator argument template may use (EMU-001, EMU-004).
export const emulatorMarkers = ['{rom}', '{dir}'] as const

type WailsApp = {
  EmulatorList?: () => Promise<{ emulators: EmulatorView[] }>
  EmulatorAdd?: (name: string, exePath: string, argsTemplate: string) => Promise<EmulatorView>
  EmulatorUpdate?: (id: number, name: string, exePath: string, argsTemplate: string) => Promise<EmulatorView>
  EmulatorDelete?: (id: number) => Promise<void>
  PlatformSetEmulator?: (platformId: number, emulatorId: number) => Promise<PlatformView>
}

type WailsWindow = { go?: { main?: { App?: WailsApp } } }

function app(): WailsApp {
  return ((window as unknown as WailsWindow).go?.main?.App ?? {}) as WailsApp
}

async function call<T>(name: keyof WailsApp, args: unknown[]): Promise<T> {
  const fn = app()[name] as ((...a: unknown[]) => Promise<T>) | undefined
  if (!fn) {
    throw new Error(`${name} indisponível`)
  }
  return fn(...args)
}

export const emulatorKey = ['library', 'emulators'] as const

// useEmulators lists the emulators, ordered by name (EMU-001).
export function useEmulators() {
  return useQuery({
    queryKey: emulatorKey,
    queryFn: async () => (await call<{ emulators: EmulatorView[] }>('EmulatorList', [])).emulators,
    retry: false,
  })
}

export function addEmulator(name: string, exePath: string, argsTemplate: string): Promise<EmulatorView> {
  return call<EmulatorView>('EmulatorAdd', [name, exePath, argsTemplate])
}

export function updateEmulator(id: number, name: string, exePath: string, argsTemplate: string): Promise<EmulatorView> {
  return call<EmulatorView>('EmulatorUpdate', [id, name, exePath, argsTemplate])
}

export function deleteEmulator(id: number): Promise<void> {
  return call<void>('EmulatorDelete', [id])
}

// setPlatformEmulator sets the default emulator of a platform; null clears it (EMU-002).
export function setPlatformEmulator(platformId: number, emulatorId: number | null): Promise<PlatformView> {
  return call<PlatformView>('PlatformSetEmulator', [platformId, emulatorId ?? 0])
}

// unknownMarkers lists the {...} tokens of a template that are not markers (EMU-001).
export function unknownMarkers(template: string): string[] {
  const found = template.match(/\{[^{}]*\}/g) ?? []
  return found.filter((t) => !(emulatorMarkers as readonly string[]).includes(t))
}

// isAbsoluteWindowsPath accepts a drive path (C:\...) or a UNC path (\\server\...).
export function isAbsoluteWindowsPath(p: string): boolean {
  const drive = /^[A-Za-z]:/.test(p) && (p.charAt(2) === '\\' || p.charAt(2) === '/')
  return drive || p.startsWith('\\\\')
}
