import { useId, type JSX, type ReactNode } from 'react'
import { CoverImage } from './CoverImage'
import { providersLabel, scoreLabel, type MetadataCandidateView, type MetadataConfidence } from '../lib/metadata'

export type MetadataOptionsProps = {
  candidates: MetadataCandidateView[]
  confidence: MetadataConfidence
  warnings: string[]
  selected: number | null
  onSelect: (i: number | null) => void
  // Extra control per item, e.g. "Aplicar" in the game dialog; rendered outside the option's label.
  renderAction?: (c: MetadataCandidateView, i: number) => ReactNode
  // Adds the "Nenhuma" option (file import) [MET-041].
  allowNone?: boolean
}

// optionName is the accessible name of an option: "<título> (<ano>), <provedores>"; a year of 0 is omitted [MET-069].
function optionName(c: MetadataCandidateView): string {
  const head = c.year > 0 ? `${c.title} (${c.year})` : c.title
  const providers = providersLabel(c.refs)
  return providers ? `${head}, ${providers}` : head
}

const noop = () => undefined

// MetadataOptions is the single list of search results: a radio group, one option per candidate [MET-044, MET-069].
// Arrow keys and Space are the browser's; a click on a radio fires onClick, also when it is already checked.
export function MetadataOptions({
  candidates,
  confidence,
  warnings,
  selected,
  onSelect,
  renderAction,
  allowNone = false,
}: MetadataOptionsProps): JSX.Element {
  const groupName = useId()

  return (
    <div className="flex flex-col gap-2">
      {warnings.map((w, i) => (
        <p key={i} className="text-sm text-warning">
          {w}
        </p>
      ))}

      <div role="radiogroup" aria-label="Opções de metadados" className="flex flex-col gap-2">
        {candidates.map((c, i) => (
          <div key={i} className="flex items-center gap-3 rounded border border-line p-2">
            <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-3">
              <input
                type="radio"
                name={groupName}
                aria-label={optionName(c)}
                checked={selected === i}
                onChange={noop}
                onClick={() => onSelect(i)}
                className="h-4 w-4 shrink-0 accent-[var(--arc-accent)]"
              />
              <div className="w-12 shrink-0">
                <CoverImage src={c.thumbUrl} referrerPolicy="no-referrer" />
              </div>
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="flex flex-wrap items-baseline gap-x-2">
                  <span className="truncate text-sm text-fg">{c.title}</span>
                  {c.year > 0 && <span className="text-sm text-muted">({c.year})</span>}
                  {i === 0 && confidence === 'match' && <span className="text-xs text-accent">provável</span>}
                </div>
                <div className="flex flex-wrap gap-x-2 text-xs text-muted">
                  <span>{c.platform || 'Plataforma desconhecida'}</span>
                  <span>{providersLabel(c.refs)}</span>
                  <span>{scoreLabel(c.score)}</span>
                </div>
              </div>
            </label>
            {renderAction?.(c, i)}
          </div>
        ))}

        {allowNone && (
          <label className="flex cursor-pointer items-center gap-3 rounded border border-line p-2">
            <input
              type="radio"
              name={groupName}
              aria-label="Nenhuma"
              checked={selected === null}
              onChange={noop}
              onClick={() => onSelect(null)}
              className="h-4 w-4 shrink-0 accent-[var(--arc-accent)]"
            />
            <span className="text-sm text-muted">Nenhuma</span>
          </label>
        )}
      </div>

      {candidates.length === 0 && <p className="text-sm text-muted">Nenhum resultado para este nome.</p>}
    </div>
  )
}
