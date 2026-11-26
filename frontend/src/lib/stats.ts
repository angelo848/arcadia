import { useQuery } from '@tanstack/react-query'

// Statistics (phase 8, EST-001..EST-006). The totals are read by the agent; the UI only shows them.

export type StatsPeriod = '7d' | '30d' | '365d' | 'all'

// The periods the selector offers, in order, with their labels (EST-004).
export const statsPeriods: Array<{ value: StatsPeriod; label: string }> = [
  { value: '7d', label: 'Últimos 7 dias' },
  { value: '30d', label: 'Últimos 30 dias' },
  { value: '365d', label: 'Últimos 365 dias' },
  { value: 'all', label: 'Tudo' },
]

// The period selected when the page opens (EST-004).
export const defaultStatsPeriod: StatsPeriod = '7d'

export type StatsGame = {
  gameId: number
  title: string
  platform: string
  sessions: number
  activeSeconds: number
  importedSeconds: number
  manualSeconds: number
  lastPlayedAt: string
}

export type StatsPlatform = {
  platformId: number
  name: string
  activeSeconds: number
  importedSeconds: number
  manualSeconds: number
}

export type StatsSummary = {
  games: StatsGame[]
  platforms: StatsPlatform[]
  totalSeconds: number
  importedSeconds: number
  manualSeconds: number
}

export type StatsWeek= { start: string; seconds: number; current: boolean }

export type StatsWeekly = { weeks: StatsWeek[] }

type WailsApp = {
  StatsSummary?: (period: string) => Promise<StatsSummary>
  StatsWeekly?: (weeks: number) => Promise<StatsWeekly>
}

type WailsWindow = { go?: { main?: { App?: WailsApp } } }

function app(): WailsApp {
  return ((window as unknown as WailsWindow).go?.main?.App ?? {}) as WailsApp
}

// statsKey is the prefix of every statistics query: a finished session invalidates all of them.
export const statsKey = ['stats'] as const

// useStatsSummary reads the totals of a period (EST-001, EST-003, EST-004).
export function useStatsSummary(period: StatsPeriod) {
  return useQuery({
    queryKey: [...statsKey, 'summary', period],
    queryFn: async () => {
      const fn = app().StatsSummary
      if (!fn) {
        throw new Error('StatsSummary indisponível')
      }
      return fn(period)
    },
    retry: false,
  })
}

// useStatsWeekly reads the time of the last weeks (EST-002).
export function useStatsWeekly(weeks: number) {
  return useQuery({
    queryKey: [...statsKey, 'weekly', weeks],
    queryFn: async () => {
      const fn = app().StatsWeekly
      if (!fn) {
        throw new Error('StatsWeekly indisponível')
      }
      return fn(weeks)
    },
    retry: false,
  })
}

// importedShare is the part of a total that came from LaunchBox, as a fraction (EST-005).
export function importedShare(total: number, imported: number): number {
  return total > 0 ? Math.min(1, imported / total) : 0
}
