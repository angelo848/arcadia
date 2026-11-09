import { useId, useState, type FormEvent, type JSX } from 'react'
import type { PickedExe } from '../lib/games'

// AddGameForm shows the chosen path (read-only) and the editable "Título"
// prefilled with the default title. "Salvar" is disabled with a blank title
// (after trimming) or while the request is pending [UI-007].
export function AddGameForm({
  picked,
  pending,
  onSave,
  onCancel,
}: {
  picked: PickedExe
  pending: boolean
  onSave: (title: string) => void
  onCancel: () => void
}): JSX.Element {
  const [title, setTitle] = useState(picked.title)
  const titleId = useId()
  const trimmed = title.trim()
  const canSave = trimmed !== '' && !pending

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!canSave) {
      return
    }
    onSave(trimmed)
  }

  return (
    <form
      onSubmit={submit}
      className="mx-6 my-4 flex flex-col gap-3 rounded border border-neutral-800 bg-neutral-900 p-4"
    >
      <p className="break-all font-mono text-sm text-neutral-400">{picked.path}</p>
      <label htmlFor={titleId} className="text-sm font-medium text-neutral-300">
        Título
      </label>
      <input
        id={titleId}
        type="text"
        value={title}
        maxLength={200}
        onChange={(e) => setTitle(e.target.value)}
        className="rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-neutral-100 focus:border-emerald-500 focus:outline-none"
      />
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="rounded px-4 py-1.5 text-sm font-medium text-neutral-300 hover:bg-neutral-800"
        >
          Cancelar
        </button>
        <button
          type="submit"
          disabled={!canSave}
          className="rounded bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400"
        >
          Salvar
        </button>
      </div>
    </form>
  )
}
