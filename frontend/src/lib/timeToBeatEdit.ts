import type { TimeToBeatCandidate, TimeToBeatView } from './timeToBeat'

// Edit-window calls of the time to beat [TTB-018]; errors propagate to the caller.

export type TimeToBeatSettings = { enabled: boolean; afterMetadata: boolean }

type EditApp = {
  TimeToBeatRefresh?: (gameId: number) => Promise<TimeToBeatView>
  TimeToBeatSearch?: (gameId: number, term: string) => Promise<TimeToBeatCandidate[]>
  TimeToBeatChoose?: (gameId: number, externalId: string) => Promise<TimeToBeatView>
  TimeToBeatSettings?: () => Promise<TimeToBeatSettings>
  TimeToBeatSetSettings?: (enabled: boolean, afterMetadata: boolean) => Promise<TimeToBeatSettings>
}

async function call<T>(name: keyof EditApp, args: unknown[]): Promise<T> {
  const app = (window as unknown as { go?: { main?: { App?: EditApp } } }).go?.main?.App
  const fn = app?.[name] as ((...a: unknown[]) => Promise<T>) | undefined
  if (!fn) {
    throw new Error(`${name} indisponível`)
  }
  return fn(...args)
}

export function refreshTimeToBeat(gameId: number): Promise<TimeToBeatView> {
  return call<TimeToBeatView>('TimeToBeatRefresh', [gameId])
}

// term "" searches by the game title.
export function searchTimeToBeat(gameId: number, term: string): Promise<TimeToBeatCandidate[]> {
  return call<TimeToBeatCandidate[]>('TimeToBeatSearch', [gameId, term])
}

// externalId "" means "none of these".
export function chooseTimeToBeat(gameId: number, externalId: string): Promise<TimeToBeatView> {
  return call<TimeToBeatView>('TimeToBeatChoose', [gameId, externalId])
}

export function getTimeToBeatSettings(): Promise<TimeToBeatSettings> {
  return call<TimeToBeatSettings>('TimeToBeatSettings', [])
}

export function setTimeToBeatSettings(enabled: boolean, afterMetadata: boolean): Promise<TimeToBeatSettings> {
  return call<TimeToBeatSettings>('TimeToBeatSetSettings', [enabled, afterMetadata])
}
