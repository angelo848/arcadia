import { useEffect, useRef, useState } from 'react'
import type { JSX } from 'react'
import { SEARCH_DEBOUNCE_MS, createDebouncer } from '../lib/debounce'
import type { Timers } from '../lib/debounce'

// SearchBox edits the free-text filter [UI-033]. The text shows at once; the query is
// sent only after the last keystroke has waited SEARCH_DEBOUNCE_MS. Unmounting cancels it.
export function SearchBox({
  text,
  onChange,
  timers,
}: {
  text: string
  onChange: (text: string) => void
  timers?: Timers
}): JSX.Element {
  const [value, setValue] = useState(text)
  // Last text sent to the caller; tells an outside change (e.g. "Limpar filtros") from our own echo.
  const sent = useRef(text)
  const onChangeRef = useRef(onChange)
  const [debouncer] = useState(() =>
    createDebouncer<string>(
      SEARCH_DEBOUNCE_MS,
      (v) => {
        sent.current = v
        onChangeRef.current(v)
      },
      timers,
    ),
  )

  useEffect(() => {
    onChangeRef.current = onChange
  })

  useEffect(() => () => debouncer.cancel(), [debouncer])

  // The caller changed the text from outside: show it and drop any pending send.
  useEffect(() => {
    if (text !== sent.current) {
      sent.current = text
      debouncer.cancel()
      setValue(text)
    }
  }, [text, debouncer])

  return (
    <input
      type="search"
      role="searchbox"
      aria-label="Buscar na biblioteca"
      placeholder="Buscar jogos"
      value={value}
      onChange={(e) => {
        const v = e.target.value
        setValue(v)
        debouncer.call(v)
      }}
      className="w-full rounded bg-raised px-3 py-1.5 text-sm text-fg placeholder:text-muted"
    />
  )
}
