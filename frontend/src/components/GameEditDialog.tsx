import { useState, type JSX, type ReactNode } from 'react'
import type { EmulatorView } from '../lib/emulators'
import type { GameDetailView } from '../lib/library'
import { GameEditForm } from './GameEditForm'
import { Modal } from './Modal'

export type GameEditDialogProps = {
  game: GameDetailView | undefined
  loading: boolean
  loadError: string | null
  genreSuggestions?: string[]
  emulators?: EmulatorView[]
  extraSection?: ReactNode
  scrollToExtra?: boolean
  onSave: (patch: Record<string, unknown>) => void
  onCancel: () => void
  pending: boolean
  error: string | null
}

// GameEditDialog edits a game in a medium Modal; Salvar and Cancelar sit in the pinned footer [UIX-032].
// While the game is read it shows a loading state, and a failed read shows an alert instead of the form.
export function GameEditDialog(props: GameEditDialogProps): JSX.Element {
  const { game, loading, loadError, ...formProps } = props
  const [target, setTarget] = useState<HTMLElement | null>(null)
  return (
    <Modal
      size="medium"
      title="Editar jogo"
      onClose={props.onCancel}
      closeDisabled={props.pending}
      footer={<div ref={setTarget} className="flex gap-2" />}
    >
      {game !== undefined ? (
        <GameEditForm {...formProps} game={game} actionsTarget={target} />
      ) : loadError !== null ? (
        <p role="alert" className="text-sm text-danger">
          {loadError}
        </p>
      ) : (
        <p className="text-sm text-muted">{loading ? 'Carregando jogo…' : ''}</p>
      )}
    </Modal>
  )
}
