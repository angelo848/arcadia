import { useState, type JSX, type KeyboardEvent } from 'react'

// Longest tag the catalog accepts (DOM-016, counted in runes).
export const maxTagLength = 100

const emptyMessage = 'Digite um gênero.'
const tooLongMessage = `Use no máximo ${maxTagLength} caracteres.`

// sameName compares names without regard to case (TAG-003, EDT-004).
function sameName(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase()
}

export type TagInputProps = {
  id: string
  value: string[]
  // suggestions are the names already in the catalog of genres.
  suggestions: string[]
  onChange: (next: string[]) => void
}

// TagInput edits a list of genre names as tags [TAG-001..TAG-004]. Typing filters the
// catalog; Enter or a comma turns the text into a tag; a catalog name keeps its spelling;
// a name the catalog does not have is marked "nova" and created when the game is saved
// (EDT-004). The component never talks to the agent.
export function TagInput({ id, value, suggestions, onChange }: TagInputProps): JSX.Element {
  const [draft, setDraft] = useState('')
  const [message, setMessage] = useState<string | null>(null)

  const query = draft.trim().toLowerCase()
  const matches =
    query === ''
      ? []
      : suggestions
          .filter((s) => s.toLowerCase().includes(query) && !value.some((v) => sameName(v, s)))
          .slice(0, 8)

  function add(text: string) {
    const name = text.trim()
    if (name === '') {
      setMessage(emptyMessage)
      return
    }
    if ([...name].length > maxTagLength) {
      setMessage(tooLongMessage)
      return
    }
    const known = suggestions.find((s) => sameName(s, name))
    const tag = known ?? name
    if (!value.some((v) => sameName(v, tag))) {
      onChange([...value, tag])
    }
    setDraft('')
    setMessage(null)
  }

  function remove(index: number) {
    onChange(value.filter((_, i) => i !== index))
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      add(draft)
      return
    }
    if (e.key === 'Backspace' && draft === '' && value.length > 0) {
      remove(value.length - 1)
    }
  }

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-wrap items-center gap-1.5 rounded border border-neutral-700 bg-neutral-950 px-2 py-1.5 focus-within:border-emerald-500">
        {value.map((tag, i) => {
          const isNew = !suggestions.some((s) => sameName(s, tag))
          return (
            <span key={tag} className="flex items-center gap-1 rounded bg-neutral-800 px-2 py-0.5 text-sm text-neutral-100">
              {tag}
              {isNew && <span className="text-xs text-emerald-400">nova</span>}
              <button
                type="button"
                aria-label={`Remover ${tag}`}
                onClick={() => remove(i)}
                className="text-neutral-400 hover:text-neutral-100"
              >
                ×
              </button>
            </span>
          )
        })}
        <input
          id={id}
          type="text"
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value)
            setMessage(null)
          }}
          onKeyDown={onKeyDown}
          className="min-w-[8rem] flex-1 bg-transparent text-neutral-100 focus:outline-none"
        />
      </div>
      {matches.length > 0 && (
        <ul aria-label="Gêneros cadastrados" className="flex flex-wrap gap-1.5">
          {matches.map((s) => (
            <li key={s}>
              <button
                type="button"
                role="option"
                aria-selected={false}
                onClick={() => add(s)}
                className="rounded border border-neutral-700 px-2 py-0.5 text-sm text-neutral-200 hover:bg-neutral-800"
              >
                {s}
              </button>
            </li>
          ))}
        </ul>
      )}
      {message !== null && <p className="text-sm text-red-400">{message}</p>}
    </div>
  )
}
