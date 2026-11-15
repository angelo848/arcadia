import { memo, useEffect, useId, useRef } from 'react'
import type { CSSProperties } from 'react'
import { isTitleEditable, selectionCounts, titleProblem } from '../lib/imports'
import type { ImportSource, ReviewRow } from '../lib/imports'

// Review table of the import dialog (phase 11, IMV-040..IMV-052). Colors come only from
// the --arc-* theme tokens (IMV-051 / theme scan), never from literals.

export type ImportReviewTableProps = {
  rows: ReviewRow[]
  source: ImportSource
  busy: boolean
  repeated: boolean[]
  onToggle: (i: number, on: boolean) => void
  onToggleAll: (on: boolean) => void
  onTitle: (i: number, value: string) => void
}

const DASH = '—'

const scrollerStyle: CSSProperties = {
  overflowY: 'auto',
  maxHeight: '60vh',
  border: '1px solid var(--arc-border)',
  borderRadius: 6,
}

const tableStyle: CSSProperties = {
  width: '100%',
  borderCollapse: 'collapse',
  color: 'var(--arc-text)',
  fontSize: 13,
}

const headerCellStyle: CSSProperties = {
  position: 'sticky',
  top: 0,
  zIndex: 1,
  background: 'var(--arc-surface)',
  color: 'var(--arc-text-muted)',
  textAlign: 'left',
  padding: '6px 8px',
  borderBottom: '1px solid var(--arc-border)',
  fontWeight: 600,
}

const cellStyle: CSSProperties = {
  padding: '4px 8px',
  borderBottom: '1px solid var(--arc-border)',
  verticalAlign: 'middle',
}

const inputStyle: CSSProperties = {
  width: '100%',
  boxSizing: 'border-box',
  background: 'var(--arc-bg)',
  color: 'var(--arc-text)',
  border: '1px solid var(--arc-border)',
  borderRadius: 4,
  padding: '3px 6px',
}

const badgeStyle: CSSProperties = {
  display: 'inline-block',
  marginLeft: 6,
  padding: '0 6px',
  borderRadius: 4,
  border: '1px solid var(--arc-border)',
  color: 'var(--arc-text-muted)',
  fontSize: 11,
}

const errorStyle: CSSProperties = {
  display: 'block',
  color: 'var(--arc-danger)',
  fontSize: 11,
  marginTop: 2,
}

const TITLE_ERRORS = {
  empty: 'Título vazio',
  tooLong: 'Título com mais de 200 caracteres',
} as const

type RowProps = {
  row: ReviewRow
  index: number
  source: ImportSource
  busy: boolean
  repeated: boolean
  onToggle: (i: number, on: boolean) => void
  onTitle: (i: number, value: string) => void
}

// ReviewLine is one table row. Memoized: with stable callbacks, typing in one row
// re-renders only that row (IMV-051, IMV-052).
const ReviewLine = memo(function ReviewLine({ row, index, source, busy, repeated, onToggle, onTitle }: RowProps) {
  const errorId = useId()
  const c = row.candidate
  const exists = c.exists
  const editable = isTitleEditable(source) && !exists
  const problem = editable && row.checked ? titleProblem(row.title) : null
  const fileName = c.file ?? row.title
  const discs = c.discs ?? 1
  const discFiles = c.discFiles ?? []

  return (
    <tr>
      <td style={cellStyle}>
        <input
          type="checkbox"
          aria-label={`Selecionar ${row.title}`}
          checked={row.checked}
          disabled={exists || busy}
          onChange={(e) => onToggle(index, e.target.checked)}
        />
      </td>
      <td style={cellStyle}>
        {editable ? (
          <>
            <input
              type="text"
              aria-label={`Título de ${fileName}`}
              aria-invalid={problem ? true : undefined}
              aria-describedby={problem ? errorId : undefined}
              value={row.title}
              disabled={busy}
              style={inputStyle}
              onChange={(e) => onTitle(index, e.target.value)}
            />
            {problem && (
              <span id={errorId} style={errorStyle}>
                {TITLE_ERRORS[problem]}
              </span>
            )}
          </>
        ) : (
          <>
            <span>{row.title}</span>
          </>
        )}
        {exists && <span style={badgeStyle}>já existe</span>}
        {!exists && repeated && <span style={badgeStyle}>título repetido</span>}
      </td>
      <td style={cellStyle}>
        {c.file ? (
          <span title={c.launchTarget || undefined}>{c.file}</span>
        ) : (
          DASH
        )}
      </td>
      <td style={cellStyle}>{c.extension ? c.extension : DASH}</td>
      <td style={cellStyle}>
        {discs > 1 ? (
          <span title={discFiles.length > 0 ? discFiles.join('\n') : undefined}>{`${discs} discos`}</span>
        ) : (
          DASH
        )}
      </td>
    </tr>
  )
})

export function ImportReviewTable({ rows, source, busy, repeated, onToggle, onToggleAll, onTitle }: ImportReviewTableProps) {
  const selectAllRef = useRef<HTMLInputElement>(null)
  const counts = selectionCounts(rows)
  const allChecked = counts.selectable > 0 && counts.checked === counts.selectable
  const partial = counts.checked > 0 && !allChecked

  useEffect(() => {
    if (selectAllRef.current) {
      selectAllRef.current.indeterminate = partial
    }
  }, [partial])

  return (
    <div style={scrollerStyle}>
      <table style={tableStyle}>
        <thead>
          <tr>
            <th scope="col" style={headerCellStyle}>
              <input
                ref={selectAllRef}
                type="checkbox"
                aria-label="Selecionar todos"
                checked={allChecked}
                disabled={busy || counts.selectable === 0}
                onChange={() => onToggleAll(!allChecked)}
              />
            </th>
            <th scope="col" style={headerCellStyle}>Título</th>
            <th scope="col" style={headerCellStyle}>Arquivo</th>
            <th scope="col" style={headerCellStyle}>Extensão</th>
            <th scope="col" style={headerCellStyle}>Disco</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <ReviewLine
              key={i}
              row={row}
              index={i}
              source={source}
              busy={busy}
              repeated={repeated[i] ?? false}
              onToggle={onToggle}
              onTitle={onTitle}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}
