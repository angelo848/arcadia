import type { GameStatus } from './filters'
import type { GameDetailView, PlayModeName } from './library'

// EditDraft is the editable state of the edit form. year and completedAt are text as typed;
// lists are arrays of names. diffPatch and validateDraft both read this shape.
export type EditDraft = {
  title: string
  description: string
  year: string
  completedAt: string
  status: GameStatus
  favorite: boolean
  developers: string[]
  publishers: string[]
  genres: string[]
  playModes: PlayModeName[]
  // Launch fields (EDT-010): exe or uri, the target and the arguments.
  launchType: string
  launchTarget: string
  launchArgs: string
  // The game's own emulator (EMU-002); null uses the platform default.
  emulatorId: number | null
}

const dayPattern = /^\d{4}-\d{2}-\d{2}$/

// completedDay gives the calendar day (UTC) of a stored completedAt, as YYYY-MM-DD.
// The stored value is a day or a full instant (EDT-001); '' stands for no date.
export function completedDay(stored: string | null): string {
  if (stored === null || stored === '') {
    return ''
  }
  if (dayPattern.test(stored)) {
    return stored
  }
  const d = new Date(stored)
  if (Number.isNaN(d.getTime())) {
    return stored
  }
  return d.toISOString().slice(0, 10)
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i])
}

// diffPatch returns only the fields of the draft that differ from the stored game (IPC-052).
// Text is trimmed before comparing; a cleared year or completion date is sent as null, and
// an empty field that was already empty is left out. It assumes a draft that passed validateDraft.
export function diffPatch(original: GameDetailView, draft: EditDraft): Record<string, unknown> {
  const patch: Record<string, unknown> = {}

  const title = draft.title.trim()
  if (title !== original.title) {
    patch.title = title
  }

  if (draft.description !== original.description) {
    patch.description = draft.description
  }

  const yearText = draft.year.trim()
  const year = yearText === '' ? null : Number(yearText)
  if (year !== original.year) {
    patch.year = year
  }

  const completed = draft.completedAt.trim()
  if (completed !== completedDay(original.completedAt)) {
    patch.completedAt = completed === '' ? null : completed
  }

  if (draft.status !== original.status) {
    patch.status = draft.status
  }

  if (draft.favorite !== original.favorite) {
    patch.favorite = draft.favorite
  }

  const developers = draft.developers.map((s) => s.trim())
  if (!sameList(developers, original.developers)) {
    patch.developers = developers
  }

  const publishers = draft.publishers.map((s) => s.trim())
  if (!sameList(publishers, original.publishers)) {
    patch.publishers = publishers
  }

  const genres = draft.genres.map((s) => s.trim())
  if (!sameList(genres, original.genres)) {
    patch.genres = genres
  }

  if (!sameList(draft.playModes, original.playModes)) {
    patch.playModes = draft.playModes
  }

  if (draft.launchType !== original.launchType) {
    patch.launchType = draft.launchType
  }

  const launchTarget = draft.launchTarget.trim()
  if (launchTarget !== original.path) {
    patch.launchTarget = launchTarget
  }

  if (draft.launchArgs !== original.launchArgs) {
    patch.launchArgs = draft.launchArgs
  }

  if (draft.emulatorId !== original.emulatorId) {
    patch.emulatorId = draft.emulatorId
  }

  return patch
}
