import { useQuery } from '@tanstack/react-query'
import type { JSX } from 'react'
import {
  TTB_FETCH_DELAY_MS,
  timeToBeatRows,
  timeToBeatSource,
  useTimeToBeat,
  type TimeToBeatView,
} from '../lib/timeToBeat'
import { getTimeToBeatSettings } from '../lib/timeToBeatEdit'

export const timeToBeatSettingsKey = ['timeToBeatSettings'] as const

// useTimeToBeatEnabled reads whether the service is on; undefined until known. A missing binding counts as on.
export function useTimeToBeatEnabled(): boolean | undefined {
  const q = useQuery({
    queryKey: timeToBeatSettingsKey,
    queryFn: async () => {
      try {
        return (await getTimeToBeatSettings()).enabled
      } catch {
        return true
      }
    },
    retry: false,
    gcTime: 0,
    refetchOnWindowFocus: false,
  })
  return q.data
}

export function hasTimeToBeatValue(v: TimeToBeatView | undefined): boolean {
  return v?.mainSeconds != null || v?.mainExtraSeconds != null || v?.completionistSeconds != null
}

export type TimeToBeatBlockProps = {
  gameId: number
  onEdit?: (opts?: { section?: 'timeToBeat' }) => void
}

// TimeToBeatBlock shows the time to beat inside the details panel's <dl> [TTB-013, TTB-015, TTB-017, TTB-039].
export function TimeToBeatBlock({ gameId, onEdit }: TimeToBeatBlockProps): JSX.Element | null {
  const enabled = useTimeToBeatEnabled()
  const q = useTimeToBeat(gameId, { fetchAfterMs: enabled === true ? TTB_FETCH_DELAY_MS : null })
  const v = q.data
  if (enabled === false && (v === undefined || v.state === 'none')) {
    return null
  }
  const source = hasTimeToBeatValue(v) ? timeToBeatSource(v) : null
  return (
    <div className="col-span-full flex flex-col gap-1">
      <dt className="text-xs font-medium text-muted">Tempo para zerar</dt>
      <dd className="text-sm text-fg">
        <ul className="grid grid-cols-3 gap-4">
          {timeToBeatRows(v).map((r) => (
            <li key={r.label} className="flex flex-col gap-0.5">
              <span className="text-muted">{r.label}</span>
              <span>{r.value}</span>
            </li>
          ))}
        </ul>
        {source !== null && <p className="mt-1 text-xs text-muted">{`Fonte: ${source}`}</p>}
        {v?.state === 'doubt' && (
          <div className="flex items-center gap-2 text-xs text-muted">
            <span>Correspondência incerta</span>
            {onEdit && (
              <button
                type="button"
                onClick={() => onEdit({ section: 'timeToBeat' })}
                className="rounded bg-raised px-2 py-0.5 text-fg hover:bg-hover"
              >
                Confirmar…
              </button>
            )}
          </div>
        )}
      </dd>
    </div>
  )
}
