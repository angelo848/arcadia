import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { QueryClient, UseMutationResult } from '@tanstack/react-query'
import { call, libraryBinding, libraryGamesKey, libraryKeys } from './library'
import type { GameDetailView, SessionItemView } from './library'
import { statsKey } from './stats'
import type { SessionCreateInput, SessionUpdatePatch } from './sessionForm'

// Write hooks of the sessions of a game (phase 16, SEL-013).

export type SessionWriteView = { session: SessionItemView; game: GameDetailView }

export const sessionNotFoundMessage = 'Esta sessão não existe mais.'

export function isSessionNotFound(err: unknown): boolean {
  return err instanceof Error && err.message === sessionNotFoundMessage
}

// After a write the game detail comes from the response; lists and stats are reloaded once.
async function refresh(qc: QueryClient, gameId: number, game: GameDetailView): Promise<void> {
  qc.setQueryData(libraryKeys.game(gameId), game)
  await Promise.all([
    qc.invalidateQueries({ queryKey: libraryKeys.sessions(gameId) }),
    qc.invalidateQueries({ queryKey: libraryGamesKey }),
    qc.invalidateQueries({ queryKey: statsKey }),
  ])
}

export function useCreateSession(gameId: number): UseMutationResult<SessionWriteView, Error, SessionCreateInput> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SessionCreateInput) => call(() => libraryBinding('CreateSession')(gameId, input)),
    onSuccess: (r) => refresh(qc, gameId, r.game),
  })
}

export function useUpdateSession(
  gameId: number,
): UseMutationResult<SessionWriteView, Error, { sessionId: number; patch: SessionUpdatePatch }> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ sessionId, patch }: { sessionId: number; patch: SessionUpdatePatch }) =>
      call(() => libraryBinding('UpdateSession')(sessionId, patch)),
    onSuccess: (r) => refresh(qc, gameId, r.game),
  })
}

export function useDeleteSession(gameId: number): UseMutationResult<GameDetailView, Error, number> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (sessionId: number) => call(() => libraryBinding('DeleteSession')(sessionId)),
    onSuccess: (game) => refresh(qc, gameId, game),
  })
}
