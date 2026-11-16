// Selection is the game in focus of the details panel; UI state only, never sent to the agent
// [UIX-017, UIX-021..UIX-024, UIX-034, UIX-035].
export type Selection =
  | { kind: 'empty' }
  | { kind: 'loading'; gameId: number }
  | { kind: 'shown'; gameId: number }
  | { kind: 'error'; gameId: number; message: string }

export type SelectionEvent =
  | { type: 'select'; gameId: number }
  | { type: 'readOk'; gameId: number }
  | { type: 'readMissing'; gameId: number }
  | { type: 'readError'; gameId: number; message: string }
  | { type: 'retry' }
  | { type: 'removed'; gameId: number }
  | { type: 'clear' }

export const initialSelection: Selection = { kind: 'empty' }

export function focusedGameId(s: Selection): number | null {
  return s.kind === 'empty' ? null : s.gameId
}

// selectionReducer ignores events about a game other than the one in focus [UIX-021].
export function selectionReducer(s: Selection, e: SelectionEvent): Selection {
  switch (e.type) {
    case 'select':
      if (s.kind !== 'empty' && s.gameId === e.gameId) {
        return s
      }
      return { kind: 'loading', gameId: e.gameId }
    case 'retry':
      return s.kind === 'error' ? { kind: 'loading', gameId: s.gameId } : s
    case 'readOk':
      if (s.kind === 'empty' || s.gameId !== e.gameId) {
        return s
      }
      return s.kind === 'shown' ? s : { kind: 'shown', gameId: s.gameId }
    case 'readMissing':
      if (s.kind === 'empty' || s.gameId !== e.gameId) {
        return s
      }
      return initialSelection
    case 'readError':
      if (s.kind === 'empty' || s.gameId !== e.gameId || s.kind === 'shown') {
        return s
      }
      return { kind: 'error', gameId: s.gameId, message: e.message }
    case 'clear':
      return initialSelection
    case 'removed':
      if (s.kind === 'empty' || s.gameId !== e.gameId) {
        return s
      }
      return initialSelection
  }
}
