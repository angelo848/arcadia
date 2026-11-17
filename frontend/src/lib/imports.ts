// Import calls (phase 6, IMP-001..IMP-010). The bound methods are reached through
// window with local types, like lib/games.ts, so tests can fake window.go.

import type { PlatformView } from './library'

export type ImportSource = 'steam' | 'epic' | 'file' | 'folder' | 'launchbox' | 'uri'

// ImportCandidate mirrors cmd/arcadia ImportCandidateView (JSON names of contract v0.7.0).
// file, extension, discs and discFiles come with the phase 11 contract (IMV-024).
export type ImportCandidate = {
  source: string
  title: string
  platform: string
  launchType: string
  launchTarget: string
  installDir: string
  developers: string[]
  publishers: string[]
  genres: string[]
  year: number | null
  description: string
  playModes: string[]
  playedSeconds: number | null
  lastPlayed: string | null
  exists: boolean
  file?: string
  extension?: string
  discs?: number
  discFiles?: string[]
  // searchName is the name suggested for the metadata search of an Arquivo candidate (MET-040).
  searchName?: string
}

export type ImportScanResult = {
  candidates: ImportCandidate[]
  warnings: string[]
}

// ImportScanOptions goes to ImportScan (IMV-063, IMV-011, IMV-017).
export type ImportScanOptions = {
  platform: string
  titleFrom: 'file' | 'folder'
  recursive: boolean
}

export type ImportApplyResult = {
  applied: { gameId: number; title: string }[]
  skipped: { title: string; reason: string }[]
}

type WailsApp = {
  ImportScan?: (source: string, path: string, opts: ImportScanOptions) => Promise<ImportScanResult>
  ImportApply?: (candidates: ImportCandidate[], launchboxPath: string) => Promise<ImportApplyResult>
  PickFile?: () => Promise<string>
  PickFolder?: () => Promise<string>
}

type WailsWindow = { go?: { main?: { App?: WailsApp } } }

function app(): WailsApp {
  return ((window as unknown as WailsWindow).go?.main?.App ?? {}) as WailsApp
}

// scanImport reads one source without writing (IMP-001). A missing binding fails
// like a failed call, so the dialog can show the error.
export async function scanImport(
  source: ImportSource,
  path: string,
  opts: ImportScanOptions,
): Promise<ImportScanResult> {
  const fn = app().ImportScan
  if (!fn) {
    throw new Error('ImportScan indisponível')
  }
  return fn(source, path, opts)
}

// isTitleEditable tells whether the title of a source can be edited (IMV-044).
export function isTitleEditable(source: ImportSource): boolean {
  return source === 'file' || source === 'folder' || source === 'uri'
}

// titleProblem checks a title trimmed; the limit counts runes (IMV-045).
export function titleProblem(title: string): 'empty' | 'tooLong' | null {
  const t = title.trim()
  if (t === '') {
    return 'empty'
  }
  if (Array.from(t).length > 200) {
    return 'tooLong'
  }
  return null
}

// repeatedFlags marks the 2nd and later occurrences of a title among the rows
// that do not exist yet; case and surrounding spaces are ignored (IMV-046).
export function repeatedFlags(rows: { title: string; exists: boolean }[]): boolean[] {
  const seen = new Set<string>()
  return rows.map((r) => {
    if (r.exists) {
      return false
    }
    const key = r.title.trim().toLowerCase()
    if (seen.has(key)) {
      return true
    }
    seen.add(key)
    return false
  })
}

// defaultPlatformName picks the platform preselected in the dialog: the seed windows
// one, else the first (IMV-061). null when there are none.
export function defaultPlatformName(platforms: Pick<PlatformView, 'name' | 'seedKey'>[]): string | null {
  const win = platforms.find((p) => p.seedKey === 'windows')
  if (win) {
    return win.name
  }
  return platforms.length > 0 ? platforms[0].name : null
}

export type ReviewRow = { candidate: ImportCandidate; title: string; checked: boolean }

// buildReviewRows makes the review rows of a scan: checked unless it exists or repeats (IMV-041).
export function buildReviewRows(candidates: ImportCandidate[]): ReviewRow[] {
  const repeated = repeatedFlags(candidates.map((c) => ({ title: c.title, exists: c.exists })))
  return candidates.map((c, i) => ({
    candidate: c,
    title: c.title,
    checked: !c.exists && !repeated[i],
  }))
}

// selectionCounts counts the checked rows, the selectable ones (not existing) and the existing ones (IMV-047).
export function selectionCounts(rows: ReviewRow[]): { checked: number; selectable: number; existing: number } {
  let checked = 0
  let selectable = 0
  let existing = 0
  for (const r of rows) {
    if (r.candidate.exists) {
      existing++
      continue
    }
    selectable++
    if (r.checked) {
      checked++
    }
  }
  return { checked, selectable, existing }
}

// markImported applies the result of a lot: each applied title turns a not-yet-existing row
// with that title into exists. The sent rows are tried first; the other rows only when no sent
// row matches. The sent rows get unchecked (IMV-050).
export function markImported(
  rows: ReviewRow[],
  sent: ReviewRow[],
  applied: { title: string }[],
): ReviewRow[] {
  const sentSet = new Set(sent)
  const consumed = new Set<number>()
  for (const a of applied) {
    const free = (idx: number) => !consumed.has(idx) && !rows[idx].candidate.exists && rows[idx].title === a.title
    let i = rows.findIndex((r, idx) => sentSet.has(r) && free(idx))
    if (i < 0) {
      i = rows.findIndex((r, idx) => !sentSet.has(r) && free(idx))
    }
    if (i >= 0) {
      consumed.add(i)
    }
  }
  return rows.map((r, idx) => {
    const wasSent = sentSet.has(r)
    const isNew = consumed.has(idx)
    if (!wasSent && !isNew) {
      return r
    }
    return {
      ...r,
      candidate: isNew ? { ...r.candidate, exists: true } : r.candidate,
      checked: wasSent ? false : r.checked,
    }
  })
}

// applyImport writes the selected candidates (IMP-001).
// launchboxPath is the Data\Platforms folder of the LaunchBox scan, "" for the other sources (MID-010).
export async function applyImport(candidates: ImportCandidate[], launchboxPath = ''): Promise<ImportApplyResult> {
  const fn = app().ImportApply
  if (!fn) {
    throw new Error('ImportApply indisponível')
  }
  return fn(candidates, launchboxPath)
}

// IMPORT_LOT_SIZE is how many candidates go in each ImportApply call (AJU-025).
export const IMPORT_LOT_SIZE = 25

export type ImportLotsOutcome = {
  result: ImportApplyResult
  // notSent counts the candidates of the failed lot and of the lots after it.
  notSent: number
  error?: Error
}

// applyImportInLots sends the candidates in sequential lots, in order, and stops
// at the first lot that fails (AJU-025). onProgress(sent, total) runs before the
// first lot and after each one that succeeds.
export async function applyImportInLots(
  candidates: ImportCandidate[],
  launchboxPath: string,
  onProgress: (sent: number, total: number) => void,
): Promise<ImportLotsOutcome> {
  const total = candidates.length
  const result: ImportApplyResult = { applied: [], skipped: [] }
  let sent = 0
  onProgress(0, total)
  for (let i = 0; i < total; i += IMPORT_LOT_SIZE) {
    const lot = candidates.slice(i, i + IMPORT_LOT_SIZE)
    try {
      const res = await applyImport(lot, launchboxPath)
      result.applied.push(...res.applied)
      result.skipped.push(...res.skipped)
    } catch (e) {
      return { result, notSent: total - sent, error: e instanceof Error ? e : new Error(String(e)) }
    }
    sent += lot.length
    onProgress(sent, total)
  }
  return { result, notSent: 0 }
}

// pickFile opens the single-file dialog of the file source; "" means cancelled (IMP-004).
export async function pickFile(): Promise<string> {
  const fn = app().PickFile
  if (!fn) {
    throw new Error('PickFile indisponível')
  }
  return fn()
}

// pickFolder opens the folder dialog; "" means cancelled (IMP-010).
export async function pickFolder(): Promise<string> {
  const fn = app().PickFolder
  if (!fn) {
    throw new Error('PickFolder indisponível')
  }
  return fn()
}

// sourceLabels names the import sources in the dialog.
export const sourceLabels: Record<ImportSource, string> = {
  steam: 'Steam',
  epic: 'Epic Games',
  file: 'Arquivo',
  folder: 'Pasta de jogos',
  launchbox: 'LaunchBox',
  uri: 'Link (URI)',
}

// needsPath tells whether a source reads a folder chosen by the user (IMP-004, IMP-005).
export function needsPath(source: ImportSource): boolean {
  return source === 'file' || source === 'folder' || source === 'launchbox' || source === 'uri'
}

// skipReasonLabels names the reasons of ImportApplyResult.skipped.
export const skipReasonLabels: Record<string, string> = {
  duplicate: 'já estava na biblioteca',
  failed: 'não pôde ser gravado',
}
