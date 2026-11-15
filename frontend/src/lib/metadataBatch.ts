import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { libraryGamesKey, libraryKeys } from './library'
import type { MetadataCandidateView, MetadataConfidence } from './metadata'

// Batch of unified metadata search (phase 12, MET-055..067; contracts/ipc-v0.14.0-delta.md §7).
export type Confidence = MetadataConfidence

export type BatchState = 'searching' | 'ready' | 'cancelled' | 'applying' | 'done'

export type BatchResultView = {
  applied: number
  imagesQueued: number
  imagesSkippedByLimit: number
  failedTitles: string[]
}

export type BatchStatusView = {
  batchId: number
  state: BatchState
  total: number
  done: number
  failed: number
  warnings: string[]
  result?: BatchResultView
}

export type BatchRowView = {
  gameId: number
  title: string
  platform: string
  confidence: Confidence
  selected: number
  candidates: MetadataCandidateView[]
}

export type BatchChoice = { gameId: number; candidate: number }

export type BatchResultsPage = { rows: BatchRowView[]; total: number; next: number | null }

export const batchPageSize = 100

const batchEvent = 'metadata:batch'

type WailsApp = Record<string, ((...a: unknown[]) => Promise<unknown>) | undefined>
type WailsWindow = { go?: { main?: { App?: WailsApp } }; runtime?: { EventsOn?: (name: string, cb: (...data: unknown[]) => void) => (() => void) | void } }

function appCall<T>(name: string, args: unknown[]): Promise<T> {
  const app = ((window as unknown as WailsWindow).go?.main?.App ?? {}) as WailsApp
  const fn = app[name]
  if (!fn) {
    return Promise.reject(new Error(`${name} indisponível`))
  }
  return fn(...args) as Promise<T>
}

// invalidateAfterDone reloads the game lists, the games and the catalog; sessions and the current session are not touched [MET-066].
function invalidateAfterDone(qc: QueryClient): void {
  void qc.invalidateQueries({ queryKey: libraryGamesKey })
  void qc.invalidateQueries({ queryKey: libraryKeys.catalog })
  void qc.invalidateQueries({ queryKey: ['library', 'game'] })
}

type BatchEventPayload = { batchId: number; state: BatchState; done: number; total: number; failed: number }

function parseEvent(p: unknown): BatchEventPayload | null {
  if (typeof p !== 'object' || p === null) {
    return null
  }
  const e = p as Partial<BatchEventPayload>
  if (typeof e.batchId !== 'number' || typeof e.state !== 'string') {
    return null
  }
  return { batchId: e.batchId, state: e.state, done: e.done ?? 0, total: e.total ?? 0, failed: e.failed ?? 0 }
}

// useMetadataBatch follows the batch of the agent: one metadata:batch subscription, reads the status when the state changes [MET-055].
// interrupted turns true when there was a batch and the agent now reports none (restart) [MET-067].
// connected is the agent connection of the caller: a reconnection reads the state again [MET-055].
// refresh reads the state now (e.g. after a batch was started by another window) [MET-053].
export function useMetadataBatch(connected = true): {
  status: BatchStatusView | null
  interrupted: boolean
  refresh(): Promise<void>
  start(ids: number[]): Promise<void>
  cancel(): Promise<void>
  discard(): Promise<void>
  apply(choices: BatchChoice[]): Promise<void>
} {
  const qc = useQueryClient()
  const [status, setStatus] = useState<BatchStatusView | null>(null)
  const [interrupted, setInterrupted] = useState(false)
  const current = useRef<BatchStatusView | null>(null)
  const alive = useRef(false)
  const wasConnected = useRef(connected)

  const commit = useCallback((b: BatchStatusView | null) => {
    current.current = b
    if (alive.current) {
      setStatus(b)
    }
  }, [])

  const refresh = useCallback(async (): Promise<void> => {
    const res = await appCall<{ batch: BatchStatusView | null }>('MetadataBatchStatus', [])
    if (!alive.current) {
      return
    }
    const next = res.batch ?? null
    const prev = current.current
    // The notice turns on once, when the batch disappears; the read after it turns it off [MET-067].
    setInterrupted(next === null && prev !== null)
    commit(next)
    if (next !== null && next.state === 'done' && prev?.state !== 'done') {
      invalidateAfterDone(qc)
    }
  }, [qc, commit])

  useEffect(() => {
    alive.current = true
    void refresh().catch(() => undefined)

    const on = (window as unknown as WailsWindow).runtime?.EventsOn
    const off = on?.(batchEvent, (...data: unknown[]) => {
      const p = parseEvent(data[0])
      if (p === null) {
        return
      }
      const cur = current.current
      if (cur === null || cur.batchId !== p.batchId || cur.state !== p.state) {
        void refresh().catch(() => undefined)
        return
      }
      commit({ ...cur, done: p.done, total: p.total, failed: p.failed })
    })

    return () => {
      alive.current = false
      if (typeof off === 'function') {
        off()
      }
    }
  }, [refresh, commit])

  // A reconnection reads the state again, as the other hooks do on reconnect (MET-055, UI-009).
  useEffect(() => {
    const was = wasConnected.current
    wasConnected.current = connected
    if (!was && connected) {
      void refresh().catch(() => undefined)
    }
  }, [connected, refresh])

  const start = useCallback(
    async (ids: number[]): Promise<void> => {
      await appCall('MetadataBatchStart', [ids])
      setInterrupted(false)
      await refresh()
    },
    [refresh],
  )

  const cancel = useCallback(async (): Promise<void> => {
    const cur = current.current
    if (cur === null) {
      return
    }
    await appCall('MetadataBatchCancel', [cur.batchId])
    await refresh()
  }, [refresh])

  const discard = useCallback(async (): Promise<void> => {
    const cur = current.current
    if (cur === null) {
      return
    }
    await appCall('MetadataBatchDiscard', [cur.batchId])
    commit(null)
  }, [commit])

  const apply = useCallback(
    async (choices: BatchChoice[]): Promise<void> => {
      const cur = current.current
      if (cur === null) {
        throw new Error('Nenhum lote ativo.')
      }
      await appCall('MetadataBatchApply', [cur.batchId, choices])
      await refresh()
    },
    [refresh],
  )

  return { status, interrupted, refresh, start, cancel, discard, apply }
}

// loadBatchRows reads every row of a batch in pages of 100 until next is null [MET-057].
export async function loadBatchRows(
  batchId: number,
  onProgress?: (loaded: number, total: number) => void,
): Promise<BatchRowView[]> {
  const rows: BatchRowView[] = []
  let offset = 0
  for (;;) {
    const page = await appCall<BatchResultsPage>('MetadataBatchResults', [batchId, offset, batchPageSize])
    rows.push(...page.rows)
    onProgress?.(rows.length, page.total)
    if (page.next === null || page.next <= offset) {
      return rows
    }
    offset = page.next
  }
}

export type ReviewRow = { row: BatchRowView; choice: number; checked: boolean; byUser: boolean }

// buildReviewRows sets the default marking [MET-061]: match is checked on its first item (or the selected one);
// doubt keeps the first item unchecked; none has no choice.
export function buildReviewRows(rows: BatchRowView[]): ReviewRow[] {
  return rows.map((row) => {
    if (row.candidates.length === 0 || row.confidence === 'none') {
      return { row, choice: -1, checked: false, byUser: false }
    }
    if (row.confidence === 'match') {
      return { row, choice: row.selected >= 0 ? row.selected : 0, checked: true, byUser: false }
    }
    return { row, choice: 0, checked: false, byUser: false }
  })
}

// chooseCandidate marks the row with the chosen item; -1 ("Nenhum") unmarks it [MET-061, MET-062].
export function chooseCandidate(r: ReviewRow, choice: number): ReviewRow {
  if (choice < 0) {
    return { ...r, choice: -1, checked: false, byUser: true }
  }
  return { ...r, choice, checked: true, byUser: true }
}

// reviewCounts: selectable = rows with a selected item (candidates and choice >= 0; "Nenhum" is not selectable),
// checked = marked rows, none = rows with confidence "none", with or without candidates [MET-063].
export function reviewCounts(rows: ReviewRow[]): { selectable: number; checked: number; none: number } {
  return {
    selectable: rows.filter((r) => r.row.candidates.length > 0 && r.choice >= 0).length,
    checked: rows.filter((r) => r.checked).length,
    none: rows.filter((r) => r.row.confidence === 'none').length,
  }
}

// toChoices returns the marked rows, in order, as the apply payload [MET-064].
export function toChoices(rows: ReviewRow[]): BatchChoice[] {
  return rows.filter((r) => r.checked).map((r) => ({ gameId: r.row.gameId, candidate: r.choice }))
}
