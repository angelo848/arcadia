import type { JSX } from 'react'
import type { TimeToBeatRow } from '../lib/timeToBeat'

// TimeToBeatList draws the three time-to-beat pairs and the source line; it never writes anything [TTB-013, TTB-016, TTB-039].
export function TimeToBeatList({ rows, source }: { rows: TimeToBeatRow[]; source: string | null }): JSX.Element {
  return (
    <div data-bb-ttb="" style={{ display: 'flex', flexDirection: 'column', rowGap: '0.2em', lineHeight: 1.3 }}>
      {rows.map((r) => (
        <div key={r.label} style={{ display: 'flex', gap: '0.75em', justifyContent: 'space-between' }}>
          <span style={{ color: 'var(--arc-bb-text-muted)' }}>{r.label}</span>
          <span>{r.value}</span>
        </div>
      ))}
      {source !== null && <p style={{ margin: 0, color: 'var(--arc-bb-text-muted)', fontSize: '0.85em' }}>{`Fonte: ${source}`}</p>}
    </div>
  )
}
