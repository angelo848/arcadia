import { useEffect, useRef } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient, UseMutationResult, UseQueryResult } from '@tanstack/react-query'

// GameView mirrors cmd/arcadia GameView (JSON field names).
export type GameView = {
  id: number
  title: string
  path: string
  totalPlaySeconds: number
  playCount: number
  lastPlayedAt: string
}

export type SessionState = 'running' | 'paused' | 'grace' | 'ended' | 'discarded'

// SessionView mirrors cmd/arcadia SessionView (JSON field names).
export type SessionView = {
  sessionId: number
  gameId: number
  title: string
  state: SessionState
  startedAt: string
  endedAt: string
  activeSeconds: number
  activeMs: number
  asOf: string
}

// PickedExe mirrors cmd/arcadia PickedExe; an empty path means the dialog was cancelled.
export type PickedExe = { path: string; title: string }

// The bound methods and runtime are reached through window with local types,
// the same way lib/agent.ts does.
type WailsApp = {
  ListGames?: () => Promise<GameView[]>
  CurrentSession?: () => Promise<SessionView | null>
  AddGame?: (path: string, title: string) => Promise<GameView>
  LaunchGame?: (id: number) => Promise<SessionView>
  PickGameExe?: () => Promise<PickedExe>
  LaunchGameUntracked?: (id: number) => Promise<void>
  PauseSession?: () => Promise<SessionView>
  ResumeSession?: () => Promise<SessionView>
}

type WailsRuntime = {
  EventsOn?: (eventName: string, callback: (...data: unknown[]) => void) => (() => void) | void
}

type WailsWindow = {
  go?: { main?: { App?: WailsApp } }
  runtime?: WailsRuntime
}

function wails(): WailsWindow {
  return window as unknown as WailsWindow
}

export const gamesKey = ['games'] as const
export const currentSessionKey = ['session', 'current'] as const

const sessionChangedEvent = 'session:changed'

// formatPlayTime renders a game's total play time for the library list [UI-005].
export function formatPlayTime(seconds: number): string {
  if (!(seconds > 0)) {
    return 'Nunca jogado'
  }
  if (seconds < 60) {
    return '< 1 min'
  }
  const totalMinutes = Math.floor(seconds / 60)
  if (totalMinutes < 60) {
    return `${totalMinutes} min`
  }
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return `${hours} h ${String(minutes).padStart(2, '0')} min`
}

// Wails v2 rejects bound calls with a plain string; normalize to Error.
function toError(e: unknown): Error {
  return e instanceof Error ? e : new Error(String(e))
}

function binding<K extends keyof WailsApp>(name: K): NonNullable<WailsApp[K]> {
  const fn = wails().go?.main?.App?.[name]
  if (!fn) {
    throw new Error(`wails bindings unavailable: ${name}`)
  }
  return fn as NonNullable<WailsApp[K]>
}

export async function listGames(): Promise<GameView[]> {
  try {
    return await binding('ListGames')()
  } catch (e) {
    throw toError(e)
  }
}

export async function currentSession(): Promise<SessionView | null> {
  try {
    const s = await binding('CurrentSession')()
    return s ?? null
  } catch (e) {
    throw toError(e)
  }
}

export async function addGame(path: string, title: string): Promise<GameView> {
  try {
    return await binding('AddGame')(path, title)
  } catch (e) {
    throw toError(e)
  }
}

export async function launchGame(id: number): Promise<SessionView> {
  try {
    return await binding('LaunchGame')(id)
  } catch (e) {
    throw toError(e)
  }
}

export async function pickGameExe(): Promise<PickedExe> {
  try {
    return await binding('PickGameExe')()
  } catch (e) {
    throw toError(e)
  }
}

// launchGameUntracked starts a game without counting its time [UI-020].
export async function launchGameUntracked(id: number): Promise<void> {
  try {
    await binding('LaunchGameUntracked')(id)
  } catch (e) {
    throw toError(e)
  }
}

// pauseSession asks the agent to pause the active session [UI-028].
export async function pauseSession(): Promise<SessionView> {
  try {
    return await binding('PauseSession')()
  } catch (e) {
    throw toError(e)
  }
}

// resumeSession asks the agent to resume the paused session [UI-028].
export async function resumeSession(): Promise<SessionView> {
  try {
    return await binding('ResumeSession')()
  } catch (e) {
    throw toError(e)
  }
}

// Per-QueryClient count of applied session:changed events [UI-018].
const eventSeqs = new WeakMap<QueryClient, number>()

export function sessionEventSeq(qc: QueryClient): number {
  return eventSeqs.get(qc) ?? 0
}

function bumpSessionEventSeq(qc: QueryClient): void {
  eventSeqs.set(qc, sessionEventSeq(qc) + 1)
}

function isLive(s: SessionView): boolean {
  return s.state === 'running' || s.state === 'paused' || s.state === 'grace'
}

function isSessionView(v: unknown): v is SessionView {
  if (typeof v !== 'object' || v === null) {
    return false
  }
  const state = (v as { state?: unknown }).state
  return (
    state === 'running' ||
    state === 'paused' ||
    state === 'grace' ||
    state === 'ended' ||
    state === 'discarded'
  )
}

// useGames loads the library. While the call fails it retries every 2 s.
export function useGames(): UseQueryResult<GameView[], Error> {
  return useQuery({
    queryKey: gamesKey,
    queryFn: listGames,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: (query) => (query.state.status === 'error' ? 2000 : false),
  })
}

// useCurrentSession loads the current session once (no polling); later changes
// arrive through session:changed. A response that arrives after an applied
// event keeps the cached value instead [UI-018].
export function useCurrentSession(): UseQueryResult<SessionView | null, Error> {
  const qc = useQueryClient()
  return useQuery({
    queryKey: currentSessionKey,
    queryFn: async (): Promise<SessionView | null> => {
      const seq = sessionEventSeq(qc)
      const s = await currentSession()
      if (sessionEventSeq(qc) !== seq) {
        return qc.getQueryData<SessionView | null>(currentSessionKey) ?? null
      }
      return s
    },
    retry: false,
    refetchOnWindowFocus: false,
  })
}

// useSessionEvents applies each session:changed event to the cache [UI-006].
export function useSessionEvents(): void {
  const qc = useQueryClient()

  useEffect(() => {
    const on = wails().runtime?.EventsOn
    if (!on) {
      return undefined
    }
    const off = on(sessionChangedEvent, (...data: unknown[]) => {
      const s = data[0]
      if (!isSessionView(s)) {
        return
      }
      bumpSessionEventSeq(qc)
      if (isLive(s)) {
        qc.setQueryData<SessionView | null>(currentSessionKey, s)
        return
      }
      qc.setQueryData<SessionView | null>(currentSessionKey, null)
      void qc.invalidateQueries({ queryKey: gamesKey })
    })
    return () => {
      if (typeof off === 'function') {
        off()
      }
    }
  }, [qc])
}

// useReloadOnReconnect reloads games and the current session when the agent
// goes from disconnected to connected. The session is reset (not only
// invalidated) so the old value is not shown before the reload [UI-009, UI-016];
// it is also cleared, without refetching, when the agent disconnects.
export function useReloadOnReconnect(connected: boolean): void {
  const qc = useQueryClient()
  const prev = useRef(connected)

  useEffect(() => {
    const was = prev.current
    prev.current = connected
    if (was && !connected) {
      // Drop the session on disconnect so the reconnect render starts from an
      // empty cache and never shows the old value [UI-016]. Query.reset()
      // returns the query to its initial state (data undefined) and cancels an
      // in-flight fetch, without the refetch that qc.resetQueries would start
      // while disconnected; the reload happens on reconnect below.
      qc.getQueryCache().find({ queryKey: currentSessionKey, exact: true })?.reset()
    }
    if (!was && connected) {
      void qc.invalidateQueries({ queryKey: gamesKey })
      void qc.resetQueries({ queryKey: currentSessionKey })
    }
  }, [connected, qc])
}

// useLaunchGame starts a game. Its result goes to the cache only if no
// session:changed was applied since the request [UI-018].
export function useLaunchGame(): UseMutationResult<SessionView, Error, number> {
  const qc = useQueryClient()
  return useMutation<SessionView, Error, number>({
    mutationFn: async (id: number) => {
      const seq = sessionEventSeq(qc)
      const s = await launchGame(id)
      if (sessionEventSeq(qc) === seq) {
        qc.setQueryData<SessionView | null>(currentSessionKey, isLive(s) ? s : null)
      }
      return s
    },
  })
}

// useLaunchGameUntracked starts a game without tracking time. It does not touch
// the session cache nor reload the list [UI-020].
export function useLaunchGameUntracked(): UseMutationResult<void, Error, number> {
  return useMutation<void, Error, number>({
    mutationFn: (id: number) => launchGameUntracked(id),
  })
}

// useToggleSession pauses the session when the cache shows it running (or any
// non-paused state, which the agent then judges) and resumes it when the cache
// shows it paused. The cache changes only with the response, and only if no
// session:changed was applied since the request [UI-028, UI-018].
export function useToggleSession(): UseMutationResult<SessionView, Error, void> {
  const qc = useQueryClient()
  return useMutation<SessionView, Error, void>({
    mutationFn: async () => {
      const seq = sessionEventSeq(qc)
      const current = qc.getQueryData<SessionView | null>(currentSessionKey)
      const s = current?.state === 'paused' ? await resumeSession() : await pauseSession()
      if (sessionEventSeq(qc) === seq) {
        qc.setQueryData<SessionView | null>(currentSessionKey, isLive(s) ? s : null)
      }
      return s
    },
  })
}
