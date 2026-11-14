// Import calls (phase 6, IMP-001..IMP-010). The bound methods are reached through
// window with local types, like lib/games.ts, so tests can fake window.go.

export type ImportSource = 'steam' | 'epic' | 'file' | 'folder' | 'launchbox'

// ImportCandidate mirrors cmd/arcadia ImportCandidateView (JSON names of contract v0.7.0).
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
}

export type ImportScanResult = {
  candidates: ImportCandidate[]
  warnings: string[]
}

export type ImportApplyResult = {
  applied: { gameId: number; title: string }[]
  skipped: { title: string; reason: string }[]
}

type WailsApp = {
  ImportScan?: (source: string, path: string) => Promise<ImportScanResult>
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
export async function scanImport(source: ImportSource, path: string): Promise<ImportScanResult> {
  const fn = app().ImportScan
  if (!fn) {
    throw new Error('ImportScan indisponível')
  }
  return fn(source, path)
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
}

// needsPath tells whether a source reads a folder chosen by the user (IMP-004, IMP-005).
export function needsPath(source: ImportSource): boolean {
  return source === 'file' || source === 'folder' || source === 'launchbox'
}

// skipReasonLabels names the reasons of ImportApplyResult.skipped.
export const skipReasonLabels: Record<string, string> = {
  duplicate: 'já estava na biblioteca',
  failed: 'não pôde ser gravado',
}
