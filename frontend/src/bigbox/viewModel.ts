// Display model of the Big Box and its pure helpers [BBX-004, BBX-006, BBX-008..BBX-010, BBX-017, BBX-021].
import { formatPlayTime } from '../lib/games'
import type { SessionView } from '../lib/games'
import type { CatalogView, GameView } from '../lib/library'
import type { CardImage, MediaItemView } from '../lib/metadata'
import { hintsFor } from './hints'
import type { Hint } from './hints'

export const SHORT_DESCRIPTION_MAX = 280
export const STRIP_WINDOW = 7

export type BigBoxThemeId = 'faixa' | 'mosaico' | 'roda'
export type BigBoxTab = { platformId: number | null; label: string }
export type BigBoxGame = { id: number; title: string; coverUrl: string | null; session: 'running' | 'paused' | null }
export type BigBoxFocused = {
  game: BigBoxGame
  platform: string
  year: number | null
  genre: string | null
  shortDescription: string
  playTimeLabel: string
  lastPlayedLabel: string | null
  sessionsLabel: string
  totalPlaySeconds: number
  lastPlayedAt: string | null
  playCount: number
  backgroundUrl: string | null
  videoUrl: string | null
}
export type BigBoxViewModel = {
  screen: 'browse' | 'details'
  tabs: BigBoxTab[]
  tab: number
  games: BigBoxGame[]
  total: number
  focus: number
  focused: BigBoxFocused | null
  videoVisible: boolean
  sound: boolean
  hints: Hint[]
  alert: string | null
}

export type ViewModelInput = {
  screen: 'browse' | 'details'
  detailsFocus?: string
  tabs: BigBoxTab[]
  tab: number
  games: GameView[] // loaded games of the tab
  total: number
  focus: number
  covers: Record<number, CardImage> // useCardImages result
  media: MediaItemView[] // media of the focused game
  description: string // full description of the focused game
  genres: string | null // comma separated genres of the focused game
  session: SessionView | null
  sound: boolean
  videoFailed: number | null
  alert: string | null
}

export function buildTabs(
  catalog: CatalogView | undefined,
  initialPlatformId: number | null,
): { tabs: BigBoxTab[]; initialTab: number } {
  const tabs: BigBoxTab[] = [{ platformId: null, label: 'Todos' }]
  for (const p of catalog?.platforms ?? []) {
    if (p.gameCount > 0) tabs.push({ platformId: p.id, label: p.name })
  }
  const idx = initialPlatformId === null ? -1 : tabs.findIndex((t) => t.platformId === initialPlatformId)
  return { tabs, initialTab: idx > 0 ? idx : 0 }
}

export function shortDescription(text: string): string {
  if (text.length <= SHORT_DESCRIPTION_MAX) return text
  const cut = text.slice(0, SHORT_DESCRIPTION_MAX - 1)
  const space = cut.lastIndexOf(' ')
  const base = (space > 0 ? cut.slice(0, space) : cut).trimEnd()
  return `${base}…`
}

export function infoLine(platform: string, year: number | null, genre: string | null): string {
  const first = (genre ?? '').split(',')[0].trim()
  return [platform.trim(), year ? String(year) : '', first].filter((p) => p !== '').join(' · ')
}

const backgroundOrder = ['background', 'screenshot', 'title', 'select', 'banner', 'cover']

export function pickBackground(media: MediaItemView[]): string | null {
  for (const kind of backgroundOrder) {
    const m = media.find((x) => x.kind === kind)
    if (m) return m.url
  }
  return null
}

export function pickVideo(media: MediaItemView[]): string | null {
  return media.find((m) => m.kind === 'video')?.url ?? null
}

export function windowAround<T>(items: T[], focus: number): { slice: T[]; offset: number } {
  if (items.length === 0) return { slice: [], offset: 0 }
  const offset = Math.max(0, focus - STRIP_WINDOW)
  return { slice: items.slice(offset, focus + STRIP_WINDOW + 1), offset }
}

export function sessionMark(session: SessionView | null, gameId: number): 'running' | 'paused' | null {
  if (!session || session.gameId !== gameId) return null
  if (session.state === 'running' || session.state === 'grace') return 'running'
  if (session.state === 'paused') return 'paused'
  return null
}

// shortDate renders an ISO instant as a local DD/MM/YYYY date; null when never played or invalid [BBX-009].
export function shortDate(iso: string | null): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return d.toLocaleDateString('pt-BR')
}

export function sessionsLabel(playCount: number): string {
  return `Sessões: ${playCount}`
}

export function buildViewModel(i: ViewModelInput): BigBoxViewModel {
  const toGame = (g: GameView): BigBoxGame => ({
    id: g.id,
    title: g.title,
    coverUrl: i.covers[g.id]?.url ?? null,
    session: sessionMark(i.session, g.id),
  })
  const { slice } = windowAround(i.games, i.focus)
  const games = slice.map(toGame)
  const g = i.games[i.focus]
  const videoUrl = g ? pickVideo(i.media) : null
  const videoVisible = !!g && videoUrl !== null && i.videoFailed !== g.id && i.screen === 'browse'
  const focused: BigBoxFocused | null = g
    ? {
        game: toGame(g),
        platform: g.platform,
        year: g.year,
        genre: i.genres,
        shortDescription: shortDescription(i.description),
        playTimeLabel: formatPlayTime(g.totalPlaySeconds),
        lastPlayedLabel: shortDate(g.lastPlayedAt),
        sessionsLabel: sessionsLabel(g.playCount),
        totalPlaySeconds: g.totalPlaySeconds,
        lastPlayedAt: g.lastPlayedAt || null,
        playCount: g.playCount,
        backgroundUrl: pickBackground(i.media),
        videoUrl,
      }
    : null
  return {
    screen: i.screen,
    tabs: i.tabs,
    tab: i.tab,
    games,
    total: i.total,
    focus: i.focus,
    focused,
    videoVisible,
    sound: i.sound,
    hints: hintsFor(i.screen, { empty: i.total === 0, videoVisible, detailsFocus: i.detailsFocus }),
    alert: i.alert,
  }
}
