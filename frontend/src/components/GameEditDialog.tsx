import { useState, type JSX } from 'react'
import type { EmulatorView } from '../lib/emulators'
import type { GameDetailView } from '../lib/library'
import { GameEditForm } from './GameEditForm'
import { Modal } from './Modal'

export type GameEditDialogProps = {
  game: GameDetailView
  genreSuggestions?: string[]
  emulators?: EmulatorView[]
  onSave: (patch: Record<string, unknown>) => void
  onCancel: () => void
  pending: boolean
  error: string | null
}

// GameEditDialog edits a game in a medium Modal; Salvar and Cancelar sit in the pinned footer [UIX-032].
export function GameEditDialog(props: GameEditDialogProps): JSX.Element {
  const [target, setTarget] = useState<HTMLElement | null>(null)
  return (
    <Modal
      size="medium"
      title="Editar jogo"
      onClose={props.onCancel}
      footer={<div ref={setTarget} className="flex gap-2" />}
    >
      <GameEditForm {...props} actionsTarget={target} />
    </Modal>
  )
}
