// Manual edits of a game (phase 14, EDM-001..EDM-005): images picked from the disk and the total play time.

export type ManualImageKind = 'cover' | 'screenshot' | 'background'

// The kinds the edit dialog shows, with the label and the limit of each (the limits are the agent's).
export const manualImageKinds: Array<{ kind: ManualImageKind; label: string; limit: number }> = [
  { kind: 'cover', label: 'Capa', limit: 1 },
  { kind: 'screenshot', label: 'Screenshots de gameplay', limit: 10 },
  { kind: 'background', label: 'Fundo', limit: 2 },
]

// maxPlayHours is the largest total play time the agent accepts.
export const maxPlayHours = 999_999

type ManualApp = {
  PickGameImage?: () => Promise<string>
  MediaAdd?: (gameId: number, kind: string, path: string) => Promise<void>
  MediaRemove?: (mediaId: number) => Promise<void>
  SetPlayTime?: (gameId: number, seconds: number) => Promise<unknown>
}

type ManualWindow = { go?: { main?: { App?: ManualApp } } }

// Wails v2 rejects bound calls with a plain string; normalize to Error (UI-008).
async function call<T>(name: keyof ManualApp, args: unknown[]): Promise<T> {
  const fn = (window as unknown as ManualWindow).go?.main?.App?.[name] as ((...a: unknown[]) => Promise<T>) | undefined
  if (!fn) {
    throw new Error(`${name} indisponível`)
  }
  try {
    return await fn(...args)
  } catch (e) {
    throw e instanceof Error ? e : new Error(String(e))
  }
}

// pickGameImage opens the Explorer dialog for an image; "" means cancelled (EDM-004).
export function pickGameImage(): Promise<string> {
  return call<string>('PickGameImage', [])
}

// addGameImage adds the image file to the game; a cover replaces the current one (EDM-001, EDM-002).
export function addGameImage(gameId: number, kind: ManualImageKind, path: string): Promise<void> {
  return call<void>('MediaAdd', [gameId, kind, path])
}

// removeGameImage removes one image of a game (EDM-003).
export function removeGameImage(mediaId: number): Promise<void> {
  return call<void>('MediaRemove', [mediaId])
}

// setPlayTime replaces the total play time of a game, in seconds (EDM-005).
export async function setPlayTime(gameId: number, seconds: number): Promise<void> {
  await call<unknown>('SetPlayTime', [gameId, seconds])
}

// playTimeParts splits a total in seconds into the whole hours and minutes the form shows.
export function playTimeParts(seconds: number): { hours: string; minutes: string } {
  const total = Math.max(0, Math.floor(seconds / 60))
  return { hours: String(Math.floor(total / 60)), minutes: String(total % 60) }
}

// playTimeSeconds reads the hours and minutes typed; null means the text is not a valid time (EDM-005).
export function playTimeSeconds(hours: string, minutes: string): number | null {
  const h = hours.trim() === '' ? '0' : hours.trim()
  const m = minutes.trim() === '' ? '0' : minutes.trim()
  if (!/^\d{1,6}$/.test(h) || !/^\d{1,2}$/.test(m)) {
    return null
  }
  if (Number(m) > 59 || Number(h) > maxPlayHours) {
    return null
  }
  const seconds = Number(h) * 3600 + Number(m) * 60
  return seconds > maxPlayHours * 3600 ? null : seconds
}
