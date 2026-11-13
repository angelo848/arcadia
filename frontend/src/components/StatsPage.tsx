import { useId, useState, type JSX } from 'react'
import { formatPlayTime } from '../lib/games'
import { formatLocalDateTime } from '../lib/formatDate'
import {
  defaultStatsPeriod,
  importedShare,
  statsPeriods,
  useStatsSummary,
  useStatsWeekly,
  type StatsPeriod,
} from '../lib/stats'
import { StatsChart } from './StatsChart'

const selectClass =
  'rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-neutral-100 focus:border-emerald-500 focus:outline-none'

// weeksShown is the number of weeks of the chart (EST-002).
const weeksShown = 12

// StatsPage shows where the time went: per game, per week and per platform, for a period chosen
// with the selector (EST-001..EST-004). Imported LaunchBox time is marked (EST-005). The page only
// reads the totals (EST-006); a finished session reloads them through the session event.
export function StatsPage({ onOpenGame, onBack }: { onOpenGame: (id: number) => void; onBack: () => void }): JSX.Element {
  const [period, setPeriod] = useState<StatsPeriod>(defaultStatsPeriod)
  const periodId = useId()
  const summary = useStatsSummary(period)
  const weekly = useStatsWeekly(weeksShown)

  return (
    <section className="flex flex-col gap-6 px-6 py-4">
      <div className="flex flex-wrap items-center gap-4">
        <button type="button" data-bigbox-item onClick={onBack} className="text-sm text-neutral-300 hover:text-neutral-100">
          Voltar
        </button>
        <h1 className="text-xl font-semibold text-neutral-100">Estatísticas</h1>
        <label htmlFor={periodId} className="ml-auto flex items-center gap-2 text-sm text-neutral-300">
          Período
          <select id={periodId} value={period} onChange={(e) => setPeriod(e.target.value as StatsPeriod)} className={selectClass}>
            {statsPeriods.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      {summary.isError && (
        <p role="alert" className="text-sm text-red-400">
          {summary.error.message}
        </p>
      )}

      {summary.data !== undefined && (
        <p className="text-sm text-neutral-200">
          Total no período: <strong>{formatPlayTime(summary.data.totalSeconds)}</strong>
          {summary.data.importedSeconds > 0 && (
            <span className="text-neutral-400">
              {' '}
              · {formatPlayTime(summary.data.importedSeconds)} importados do LaunchBox (
              {Math.round(importedShare(summary.data.totalSeconds, summary.data.importedSeconds) * 100)}%)
            </span>
          )}
        </p>
      )}

      <section className="flex flex-col gap-2">
        <h2 className="text-lg font-medium text-neutral-200">Últimas {weeksShown} semanas</h2>
        {weekly.isError && (
          <p role="alert" className="text-sm text-red-400">
            {weekly.error.message}
          </p>
        )}
        {weekly.data !== undefined && (
          <StatsChart
            label={`Tempo jogado nas últimas ${weeksShown} semanas`}
            format={formatPlayTime}
            bars={weekly.data.weeks.map((w) => ({ label: w.start.slice(5), value: w.seconds, current: w.current }))}
          />
        )}
      </section>

      <section className="flex flex-col gap-2">
        <h2 className="text-lg font-medium text-neutral-200">Plataformas</h2>
        {summary.data !== undefined && summary.data.platforms.length === 0 && (
          <p className="text-sm text-neutral-400">Nenhum tempo jogado neste período.</p>
        )}
        <ul className="flex flex-col gap-2">
          {(summary.data?.platforms ?? []).map((p) => {
            const share = importedShare(summary.data?.totalSeconds ?? 0, p.activeSeconds)
            return (
              <li key={p.platformId} className="flex flex-col gap-1 text-sm">
                <div className="flex justify-between text-neutral-200">
                  <span>{p.name}</span>
                  <span>{formatPlayTime(p.activeSeconds)}</span>
                </div>
                <div className="h-2 rounded bg-neutral-800">
                  <div className="h-2 rounded bg-emerald-500" style={{ width: `${Math.round(share * 100)}%` }} />
                </div>
              </li>
            )
          })}
        </ul>
      </section>

      <section className="flex flex-col gap-2">
        <h2 className="text-lg font-medium text-neutral-200">Jogos</h2>
        {summary.data !== undefined && summary.data.games.length === 0 && (
          <p className="text-sm text-neutral-400">Nenhum jogo com tempo neste período.</p>
        )}
        <ul className="flex flex-col divide-y divide-neutral-800">
          {(summary.data?.games ?? []).map((g) => (
            <li key={g.gameId} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
              <button type="button" data-bigbox-item onClick={() => onOpenGame(g.gameId)} className="text-left text-neutral-100 hover:underline">
                {g.title}
              </button>
              <span className="text-neutral-400">{g.platform}</span>
              <span className="text-neutral-200">{formatPlayTime(g.activeSeconds)}</span>
              <span className="text-neutral-400">
                {g.sessions} {g.sessions === 1 ? 'sessão' : 'sessões'}
                {g.lastPlayedAt !== '' && ` · última vez ${formatLocalDateTime(g.lastPlayedAt)}`}
              </span>
              {g.importedSeconds > 0 && <span className="text-xs text-amber-400">inclui {formatPlayTime(g.importedSeconds)} importados</span>}
            </li>
          ))}
        </ul>
      </section>
    </section>
  )
}
