import { useId, useState, type FormEvent, type JSX, type ReactNode } from 'react'
import type { GameStatus } from '../lib/filters'
import { completedDay, diffPatch, type EditDraft } from '../lib/gamePatch'
import { playModeLabels, type GameDetailView, type PlayModeName } from '../lib/library'
import { validateDraft } from '../lib/validation'
import { TagInput } from './TagInput'

// The play modes in the order the form shows them (DOM-014).
const playModeOrder: PlayModeName[] = ['single', 'multi_local', 'multi_online', 'coop_local', 'coop_online']

// Labels match the library filter (FilterPanel) and the status names of GamePage.
const statusOptions: Array<{ value: GameStatus; label: string }> = [
  { value: 'not_played', label: 'Não jogado' },
  { value: 'playing', label: 'Jogando' },
  { value: 'completed', label: 'Concluído' },
  { value: 'abandoned', label: 'Abandonado' },
  { value: 'backlog', label: 'Na fila' },
]

// FormValues keeps developers and publishers as the text typed (comma separated); toDraft turns
// them into arrays. Genres are already a list of tags (TAG-001).
type FormValues = {
  title: string
  description: string
  year: string
  completedAt: string
  status: GameStatus
  favorite: boolean
  developers: string
  publishers: string
  genres: string[]
  playModes: PlayModeName[]
}

export type GameEditFormProps = {
  game: GameDetailView
  // genreSuggestions are the names already in the catalog of genres (TAG-001).
  genreSuggestions?: string[]
  onSave: (patch: Record<string, unknown>) => void
  onCancel: () => void
  pending: boolean
  error: string | null
}

const noSuggestions: string[] = []

function initialValues(game: GameDetailView): FormValues {
  return {
    title: game.title,
    description: game.description,
    year: game.year === null ? '' : String(game.year),
    completedAt: completedDay(game.completedAt),
    status: game.status,
    favorite: game.favorite,
    developers: game.developers.join(', '),
    publishers: game.publishers.join(', '),
    genres: [...game.genres],
    playModes: game.playModes,
  }
}

// An empty list text is no names; otherwise each comma is a separator. Blank names are kept
// so validateDraft can report them.
function splitNames(text: string): string[] {
  return text.trim() === '' ? [] : text.split(',')
}

function toDraft(v: FormValues): EditDraft {
  return {
    title: v.title,
    description: v.description,
    year: v.year,
    completedAt: v.completedAt,
    status: v.status,
    favorite: v.favorite,
    developers: splitNames(v.developers),
    publishers: splitNames(v.publishers),
    genres: v.genres,
    playModes: v.playModes,
  }
}

const inputClass =
  'rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-neutral-100 focus:border-emerald-500 focus:outline-none'

// GameEditForm edits the fields of EDT-001..EDT-004 with the values of the game. It keeps the
// draft and sends only the changed fields through onSave (UI-039). "Salvar" is disabled while
// the request is pending or while the draft has a validation error (UI-040). The parent makes
// the call and shows the error.
export function GameEditForm({
  game,
  genreSuggestions = noSuggestions,
  onSave,
  onCancel,
  pending,
  error,
}: GameEditFormProps): JSX.Element {
  const [values, setValues] = useState<FormValues>(() => initialValues(game))
  const idPrefix = useId()
  const draft = toDraft(values)
  const errors = validateDraft(draft)
  const canSave = !pending && Object.keys(errors).length === 0

  function set<K extends keyof FormValues>(key: K, value: FormValues[K]) {
    setValues((prev) => ({ ...prev, [key]: value }))
  }

  function togglePlayMode(mode: PlayModeName, checked: boolean) {
    setValues((prev) => {
      const others = prev.playModes.filter((m) => m !== mode)
      const next = checked ? [...others, mode] : others
      return { ...prev, playModes: playModeOrder.filter((m) => next.includes(m)) }
    })
  }

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!canSave) {
      return
    }
    onSave(diffPatch(game, draft))
  }

  const id = (name: string) => `${idPrefix}-${name}`

  return (
    <form onSubmit={submit} className="mx-6 my-4 flex flex-col gap-3 rounded border border-neutral-800 bg-neutral-900 p-4">
      <Row id={id('title')} label="Título" message={errors.title}>
        <input
          id={id('title')}
          type="text"
          value={values.title}
          onChange={(e) => set('title', e.target.value)}
          className={inputClass}
        />
      </Row>

      <Row id={id('description')} label="Descrição" message={errors.description}>
        <textarea
          id={id('description')}
          rows={5}
          value={values.description}
          onChange={(e) => set('description', e.target.value)}
          className={inputClass}
        />
      </Row>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <Row id={id('year')} label="Ano" message={errors.year}>
          <input
            id={id('year')}
            type="text"
            inputMode="numeric"
            value={values.year}
            onChange={(e) => set('year', e.target.value)}
            className={inputClass}
          />
        </Row>

        <Row id={id('completedAt')} label="Concluído em" message={errors.completedAt}>
          <input
            id={id('completedAt')}
            type="text"
            placeholder="AAAA-MM-DD"
            value={values.completedAt}
            onChange={(e) => set('completedAt', e.target.value)}
            className={inputClass}
          />
        </Row>

        <Row id={id('status')} label="Status" message={errors.status}>
          <select
            id={id('status')}
            value={values.status}
            onChange={(e) => set('status', e.target.value as GameStatus)}
            className={inputClass}
          >
            {statusOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </Row>
      </div>

      <div className="flex items-center gap-2">
        <input
          id={id('favorite')}
          type="checkbox"
          checked={values.favorite}
          onChange={(e) => set('favorite', e.target.checked)}
          className="h-4 w-4 accent-emerald-600"
        />
        <label htmlFor={id('favorite')} className="text-sm font-medium text-neutral-300">
          Favorito
        </label>
      </div>

      <fieldset className="flex flex-col gap-1.5">
        <legend className="mb-1 text-sm font-medium text-neutral-300">Modos de jogo</legend>
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {playModeOrder.map((mode) => (
            <label key={mode} className="flex items-center gap-2 text-sm text-neutral-300">
              <input
                type="checkbox"
                checked={values.playModes.includes(mode)}
                onChange={(e) => togglePlayMode(mode, e.target.checked)}
                className="h-4 w-4 accent-emerald-600"
              />
              {playModeLabels[mode]}
            </label>
          ))}
        </div>
        {errors.playModes !== undefined && <p className="text-sm text-red-400">{errors.playModes}</p>}
      </fieldset>

      <Row id={id('developers')} label="Desenvolvedora" hint="Separe os nomes por vírgula." message={errors.developers}>
        <input
          id={id('developers')}
          type="text"
          value={values.developers}
          onChange={(e) => set('developers', e.target.value)}
          className={inputClass}
        />
      </Row>

      <Row id={id('publishers')} label="Editora" hint="Separe os nomes por vírgula." message={errors.publishers}>
        <input
          id={id('publishers')}
          type="text"
          value={values.publishers}
          onChange={(e) => set('publishers', e.target.value)}
          className={inputClass}
        />
      </Row>

      <Row id={id('genres')} label="Gêneros" hint="Escolha um gênero cadastrado ou digite um novo." message={errors.genres}>
        <TagInput
          id={id('genres')}
          value={values.genres}
          suggestions={genreSuggestions}
          onChange={(next) => set('genres', next)}
        />
      </Row>

      {error !== null && (
        <p role="alert" className="text-sm text-red-400">
          {error}
        </p>
      )}

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

function Row({
  id,
  label,
  hint,
  message,
  children,
}: {
  id: string
  label: string
  hint?: string
  message: string | undefined
  children: ReactNode
}): JSX.Element {
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-neutral-300">
        {label}
      </label>
      {children}
      {hint !== undefined && <span className="text-xs text-neutral-500">{hint}</span>}
      {message !== undefined && <span className="text-xs text-red-400">{message}</span>}
    </div>
  )
}
