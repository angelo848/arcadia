// Labels of the Desktop details panel: the same texts as the normal window's GameDetailsPanel [TDK-008].
import { formatLocalDateTime, formatUtcDate } from '../lib/formatDate'
import { playModeLabels, statusLabels } from '../lib/gameLabels'
import { formatPlayTime } from '../lib/games'
import type { GameDetailView } from '../lib/library'

export type DesktopLabels = {
  platform: string
  status: string
  totalTime: string
  firstPlayed: string
  lastPlayed: string
  completedAt: string | null
  playModes: string
  genres: string
  developers: string
  publishers: string
  year: string
  favorite: boolean
}

const EMPTY = '—'

const list = (items: string[]) => (items.length > 0 ? items.join(', ') : EMPTY)

export function desktopLabels(g: GameDetailView): DesktopLabels {
  return {
    platform: g.platform,
    status: statusLabels[g.status],
    totalTime: formatPlayTime(g.totalPlaySeconds),
    firstPlayed: formatLocalDateTime(g.firstPlayedAt),
    lastPlayed: formatLocalDateTime(g.lastPlayedAt),
    completedAt: g.completedAt ? formatUtcDate(g.completedAt) : null,
    playModes: list(g.playModes.map((m) => playModeLabels[m])),
    genres: list(g.genres),
    developers: list(g.developers),
    publishers: list(g.publishers),
    year: g.year != null ? String(g.year) : EMPTY,
    favorite: !!g.favorite,
  }
}
