// Display model of the Big Box and its pure helpers [BBX-004, BBX-006, BBX-008..BBX-010, BBX-017, BBX-021].
import { formatPlayTime } from '../lib/games'
import type { SessionView } from '../lib/games'
import type { CatalogView, GameView } from '../lib/library'
import type { CardImage, MediaItemView } from '../lib/metadata'
import { hintsFor } from './hints'
import type { Hint } from './hints'
import type { DetailsFocus } from './nav'

export const SHORT_DESCRIPTION_MAX = 280
export const STRIP_ANCHOR = 2
export const STRIP_MAX = 15
export const STRIP_FALLBACK_CAPACITY = 12

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
  loading: boolean
  games: BigBoxGame[]
  total: number
  focus: number
  stripStart: number
  counter: string | null
  focused: BigBoxFocused | null
  videoVisible: boolean
  sound: boolean
  hints: Hint[]
  alert: string | null
}

export type ViewModelInput = {
  screen: 'browse' | 'details'
  detailsFocus?: DetailsFocus
  hasGallery?: boolean
  tabs: BigBoxTab[]
  tab: number
  games: GameView[] // loaded games of the tab
  total: number // T: total of the tab
  loaded: number // L: games read so far
  capacity: number // V: whole covers the theme fits
  loading: boolean // first page of the tab not read yet
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

// stripWindow is the half-open range [start, end) of covers drawn for a tab of `total` games [BBX-039].
export function stripWindow(total: number, focus: number, capacity: number): { start: number; end: number } {
  if (total <= 0) return { start: 0, end: 0 }
  const v = Math.min(Math.max(capacity, 1), STRIP_MAX - 1)
  const start = Math.min(Math.max(focus - STRIP_ANCHOR, 0), Math.max(total - v, 0))
  return { start, end: Math.min(start + v + 1, total) }
}

// stripCounter renders "position / total" of the focused game [BBX-045].
export function stripCounter(total: number, focus: number, loading: boolean): string | null {
  if (loading || total <= 0) return null
  return `${focus + 1} / ${total}`
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
  const win = stripWindow(i.total, i.focus, i.capacity)
  const games = i.loading ? [] : i.games.slice(win.start, Math.min(win.end, i.loaded)).map(toGame)
  const g = i.loading ? undefined : i.games[i.focus]
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
    loading: i.loading,
    games,
    total: i.total,
    focus: i.focus,
    stripStart: win.start,
    counter: stripCounter(i.total, i.focus, i.loading),
    focused,
    videoVisible,
    sound: i.sound,
    hints: hintsFor(i.screen, {
      empty: i.total === 0,
      videoVisible,
      detailsFocus: i.detailsFocus,
      hasGallery: i.hasGallery,
    }),
    alert: i.alert,
  }
}
