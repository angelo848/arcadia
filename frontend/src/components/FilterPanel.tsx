import { useId, useState } from 'react'
import type { JSX, ReactNode } from 'react'
import type { FilterSpec, GameStatus, LibraryQuery, PlayedFilter, PlayModeGroup } from '../lib/filters'
import { resetQuery } from '../lib/filters'
import { activeFacetCount, readFiltersOpen, writeFiltersOpen } from '../lib/uiPrefs'
import type { CatalogView } from '../lib/library'

type FacetKey = Exclude<keyof FilterSpec, 'version' | 'text'>

// Returns a new query with one facet replaced; undefined removes the member. Text and sort are kept.
function withFacet<K extends FacetKey>(q: LibraryQuery, key: K, value: FilterSpec[K] | undefined): LibraryQuery {
  const filter: FilterSpec = { ...q.filter }
  if (value === undefined) {
    delete filter[key]
  } else {
    filter[key] = value
  }
  return { ...q, filter }
}

// Parses a number input; blank or invalid input means "no limit".
function parseNumber(v: string): number | undefined {
  if (v.trim() === '') {
    return undefined
  }
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

const modeOptions: ReadonlyArray<{ value: PlayModeGroup; label: string }> = [
  { value: 'all', label: 'Todos' },
  { value: 'single', label: 'Um jogador' },
  { value: 'multi', label: 'Multijogador' },
  { value: 'coop', label: 'Cooperativo' },
]

const statusOptions: ReadonlyArray<{ value: GameStatus; label: string }> = [
  { value: 'not_played', label: 'Não jogado' },
  { value: 'playing', label: 'Jogando' },
  { value: 'completed', label: 'Concluído' },
  { value: 'abandoned', label: 'Abandonado' },
  { value: 'backlog', label: 'Na fila' },
]

const playedOptions: ReadonlyArray<{ value: PlayedFilter; label: string }> = [
  { value: 'any', label: 'Todos' },
  { value: 'played', label: 'Jogado' },
  { value: 'notPlayed', label: 'Não jogado' },
]

function Field({ label, children }: { label: string; children: (id: string) => ReactNode }): JSX.Element {
  const id = useId()
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-xs font-medium text-muted">
        {label}
      </label>
      {children(id)}
    </div>
  )
}

const controlClass = 'rounded bg-raised px-2 py-1.5 text-sm text-fg'

// FilterPanel has one control per facet [UI-032]. Every change yields a new query; "Limpar filtros" resets it [UI-035].
export function FilterPanel({
  query,
  catalog,
  onChange,
}: {
  query: LibraryQuery
  catalog: CatalogView | undefined
  onChange: (q: LibraryQuery) => void
}): JSX.Element {
  const f = query.filter
  const genres = catalog?.genres ?? []
  const companies = catalog?.companies ?? []
  const [open, setOpen] = useState(() => readFiltersOpen())
  const count = activeFacetCount(f)
  const bodyId = useId()

  function toggle() {
    const next = !open
    setOpen(next)
    writeFiltersOpen(next)
  }

  return (
    <section className="flex flex-col gap-2">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={bodyId}
        onClick={toggle}
        className="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm font-medium text-fg hover:bg-hover"
      >
        <span>{count > 0 ? `Filtros (${count})` : 'Filtros'}</span>
        <span aria-hidden="true" className="chevron" />
      </button>
      {open && (
    <div id={bodyId} className="flex flex-col gap-3">
      <Field label="Gênero">
        {(id) => (
          <select
            id={id}
            className={controlClass}
            value={f.genreIds?.[0] ?? ''}
            onChange={(e) => {
              const v = e.target.value
              onChange(withFacet(query, 'genreIds', v === '' ? undefined : [Number(v)]))
            }}
          >
            <option value="">Todos</option>
            {genres.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        )}
      </Field>

      <Field label="Modo">
        {(id) => (
          <select
            id={id}
            className={controlClass}
            value={f.playMode ?? 'all'}
            onChange={(e) => {
              const v = e.target.value
              const mode = modeOptions.find((m) => m.value === v)?.value
              onChange(withFacet(query, 'playMode', mode === undefined || mode === 'all' ? undefined : mode))
            }}
          >
            {modeOptions.map((m) => (
              <option key={m.value} value={m.value}>
                {m.label}
              </option>
            ))}
          </select>
        )}
      </Field>

      <Field label="Desenvolvedora">
        {(id) => (
          <select
            id={id}
            className={controlClass}
            value={f.developerIds?.[0] ?? ''}
            onChange={(e) => {
              const v = e.target.value
              onChange(withFacet(query, 'developerIds', v === '' ? undefined : [Number(v)]))
            }}
          >
            <option value="">Todas</option>
            {companies.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        )}
      </Field>

      <Field label="Editora">
        {(id) => (
          <select
            id={id}
            className={controlClass}
            value={f.publisherIds?.[0] ?? ''}
            onChange={(e) => {
              const v = e.target.value
              onChange(withFacet(query, 'publisherIds', v === '' ? undefined : [Number(v)]))
            }}
          >
            <option value="">Todas</option>
            {companies.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        )}
      </Field>

      <Field label="Ano de">
        {(id) => (
          <input
            id={id}
            type="number"
            inputMode="numeric"
            className={controlClass}
            value={f.yearFrom ?? ''}
            onChange={(e) => onChange(withFacet(query, 'yearFrom', parseNumber(e.target.value)))}
          />
        )}
      </Field>

      <Field label="Ano até">
        {(id) => (
          <input
            id={id}
            type="number"
            inputMode="numeric"
            className={controlClass}
            value={f.yearTo ?? ''}
            onChange={(e) => onChange(withFacet(query, 'yearTo', parseNumber(e.target.value)))}
          />
        )}
      </Field>

      <Field label="Status">
        {(id) => (
          <select
            id={id}
            className={controlClass}
            value={f.statuses?.[0] ?? ''}
            onChange={(e) => {
              const s = statusOptions.find((o) => o.value === e.target.value)?.value
              onChange(withFacet(query, 'statuses', s === undefined ? undefined : [s]))
            }}
          >
            <option value="">Todos</option>
            {statusOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        )}
      </Field>

      <Field label="Favorito">
        {(id) => (
          <select
            id={id}
            className={controlClass}
            value={f.favorite === true ? 'true' : f.favorite === false ? 'false' : ''}
            onChange={(e) => {
              const v = e.target.value
              onChange(withFacet(query, 'favorite', v === '' ? undefined : v === 'true'))
            }}
          >
            <option value="">Todos</option>
            <option value="true">Sim</option>
            <option value="false">Não</option>
          </select>
        )}
      </Field>

      <Field label="Jogado">
        {(id) => (
          <select
            id={id}
            className={controlClass}
            value={f.played ?? 'any'}
            onChange={(e) => {
              const p = playedOptions.find((o) => o.value === e.target.value)?.value
              onChange(withFacet(query, 'played', p === undefined || p === 'any' ? undefined : p))
            }}
          >
            {playedOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        )}
      </Field>

      {/* Playtime is entered in hours and sent in seconds [FLT-006]. */}
      <Field label="Tempo mínimo (h)">
        {(id) => (
          <input
            id={id}
            type="number"
            min={0}
            step="any"
            className={controlClass}
            value={f.playSecondsMin == null ? '' : f.playSecondsMin / 3600}
            onChange={(e) => {
              const h = parseNumber(e.target.value)
              onChange(withFacet(query, 'playSecondsMin', h === undefined || h < 0 ? undefined : Math.round(h * 3600)))
            }}
          />
        )}
      </Field>

      <Field label="Tempo máximo (h)">
        {(id) => (
          <input
            id={id}
            type="number"
            min={0}
            step="any"
            className={controlClass}
            value={f.playSecondsMax == null ? '' : f.playSecondsMax / 3600}
            onChange={(e) => {
              const h = parseNumber(e.target.value)
              onChange(withFacet(query, 'playSecondsMax', h === undefined || h < 0 ? undefined : Math.round(h * 3600)))
            }}
          />
        )}
      </Field>

      <button
        type="button"
        onClick={() => onChange(resetQuery())}
        className="rounded bg-raised px-3 py-1.5 text-sm font-medium text-fg hover:bg-hover"
      >
        Limpar filtros
      </button>
    </div>
      )}
    </section>
  )
}
