// Pure navigation reducer of the Big Box [BBX-002, BBX-005..BBX-007, BBX-012..BBX-018, BBX-027, CON-022].

export type DetailsFocus = 'play' | 'playUntracked' | 'gallery'
export type DetailsButton = 'play' | 'playUntracked'

export type NavZone = 'games' | 'platforms'

export type NavState = {
  screen: 'browse' | 'details'
  zone: NavZone
  tab: number
  focus: number
  detailsFocus: DetailsFocus
  detailsButton: DetailsButton
  gallery: number
  suspended: boolean
  awaitingFocusAfterBlur: boolean
  sound: boolean
  videoFailed: number | null
  launchedGameId: number | null
}

export type NavCtx = {
  tabs: number
  total: number
  loaded: number
  galleryCount: number
  focusedGameId: number | null
  videoVisible: boolean
  columns: number // 1 on the strip themes [BBX-048]
  axis: 'horizontal' | 'vertical' | 'grid'
  layout: 'screens' | 'columns'
}

export type NavAction =
  | { type: 'prev' }
  | { type: 'next' }
  | { type: 'up' }
  | { type: 'down' }
  | { type: 'tabPrev' }
  | { type: 'tabNext' }
  | { type: 'accept' }
  | { type: 'details' }
  | { type: 'back' }
  | { type: 'toggleSound' }
  | { type: 'launched'; gameId: number }
  | { type: 'windowBlur' }
  | { type: 'windowFocus' }
  | { type: 'sessionEnded'; gameId: number }
  | { type: 'gameClosed'; gameId: number; indexInTab: number | null }
  | { type: 'listLoaded'; initialIndex: number | null }
  | { type: 'videoFailed'; gameId: number }
  | { type: 'galleryChanged'; count: number }
  | { type: 'tabSet'; index: number }
export type NavEffect =
  | { kind: 'launch'; gameId: number }
  | { kind: 'launchUntracked'; gameId: number }
  | { kind: 'exit' }
  | { kind: 'bringToFront' }
  | { kind: 'enterBigBox' }

export const initialNav: NavState = {
  screen: 'browse',
  zone: 'games',
  tab: 0,
  focus: 0,
  detailsFocus: 'play',
  detailsButton: 'play',
  gallery: 0,
  suspended: false,
  awaitingFocusAfterBlur: false,
  sound: true,
  videoFailed: null,
  launchedGameId: null,
}

const inputTypes = new Set<NavAction['type']>([
  'prev', 'next', 'up', 'down', 'tabPrev', 'tabNext', 'accept', 'details', 'back', 'toggleSound', 'tabSet',
])

// Reducer branch of the `columns` layout [TDK-012..TDK-018]; returns null for actions handled as in `screens`.
function columnsNav(s: NavState, a: NavAction, c: NavCtx): { state: NavState; effects: NavEffect[] } | null {
  const none = (state: NavState = s) => ({ state, effects: [] as NavEffect[] })
  const C = Math.max(c.columns, 1)
  const M = Math.max(Math.min(c.total, c.loaded) - 1, 0)
  const hasGames = Math.min(c.total, c.loaded) > 0

  if (s.screen === 'details') {
    switch (a.type) {
      case 'up':
        if (s.detailsFocus === 'playUntracked') return none({ ...s, detailsFocus: 'play', detailsButton: 'play' })
        if (s.detailsFocus === 'play' && c.galleryCount > 0) return none({ ...s, detailsFocus: 'gallery' })
        return none()
      case 'down':
        if (s.detailsFocus === 'gallery') return none({ ...s, detailsFocus: 'play', detailsButton: 'play' })
        if (s.detailsFocus === 'play') return none({ ...s, detailsFocus: 'playUntracked', detailsButton: 'playUntracked' })
        return none()
      case 'prev':
      case 'next': {
        if (s.detailsFocus === 'gallery') return null
        if (a.type === 'next') return none()
        return none({ ...s, screen: 'browse', zone: 'games' })
      }
      case 'back':
        return none({ ...s, screen: 'browse', zone: 'games' })
      default:
        return null
    }
  }

  if (s.zone === 'platforms') {
    switch (a.type) {
      case 'up':
      case 'down': {
        const tab = s.tab + (a.type === 'down' ? 1 : -1)
        if (tab < 0 || tab >= c.tabs) return none()
        return none({ ...s, tab, focus: 0, videoFailed: null })
      }
      case 'next':
      case 'accept':
      case 'back':
        return none({ ...s, zone: 'games' })
      case 'prev':
        return none()
      default:
        return null
    }
  }

  switch (a.type) {
    case 'prev': {
      if (hasGames && s.focus % C > 0) return none({ ...s, focus: s.focus - 1, videoFailed: null })
      return none({ ...s, zone: 'platforms' })
    }
    case 'next': {
      if (hasGames && s.focus % C < C - 1 && s.focus < M) return none({ ...s, focus: s.focus + 1, videoFailed: null })
      if (!hasGames || c.focusedGameId === null) return none()
      return none({ ...s, screen: 'details', detailsFocus: 'play', detailsButton: 'play', gallery: 0 })
    }
    case 'down': {
      const f = s.focus
      let focus = f
      if (f + C <= M) focus = f + C
      else if (Math.floor(f / C) < Math.floor(M / C)) focus = M
      return none({ ...s, focus, videoFailed: null })
    }
    case 'up':
      return none(s.focus - C >= 0 ? { ...s, focus: s.focus - C, videoFailed: null } : s)
    default:
      return null
  }
}

export function bigBoxNav(s: NavState, a: NavAction, c: NavCtx): { state: NavState; effects: NavEffect[] } {
  const none = (state: NavState = s) => ({ state, effects: [] as NavEffect[] })

  if (s.suspended && inputTypes.has(a.type)) {
    return none()
  }

  if (c.layout === 'columns') {
    const r = columnsNav(s, a, c)
    if (r) return r
  }

  switch (a.type) {
    case 'tabSet': {
      if (a.index < 0 || a.index >= c.tabs || a.index === s.tab) return none()
      return none({ ...s, screen: 'browse', tab: a.index, focus: 0, videoFailed: null })
    }
    case 'prev':
    case 'next': {
      const dir = a.type === 'next' ? 1 : -1
      if (s.screen === 'details') {
        if (s.detailsFocus === 'gallery') {
          const gallery = Math.min(Math.max(s.gallery + dir, 0), Math.max(c.galleryCount - 1, 0))
          return none({ ...s, gallery })
        }
        const button: DetailsButton = dir > 0 ? 'playUntracked' : 'play'
        return none({ ...s, detailsFocus: button, detailsButton: button })
      }
      const max = Math.max(Math.min(c.total, c.loaded) - 1, 0)
      const focus = Math.min(Math.max(s.focus + dir, 0), max)
      return none({ ...s, focus, videoFailed: null })
    }
    case 'up':
    case 'down': {
      if (s.screen !== 'details') {
        const cols = c.columns
        if (c.axis !== 'grid' || cols <= 1) return none()
        // Grid [BBX-048]: one row down/up; a partial last row is reached with the last item.
        const max = Math.max(Math.min(c.total, c.loaded) - 1, 0)
        const f = s.focus
        let focus = f
        if (a.type === 'down') {
          if (f + cols <= max) focus = f + cols
          else if (Math.floor(f / cols) < Math.floor(max / cols)) focus = max
        } else if (f - cols >= 0) {
          focus = f - cols
        }
        return none({ ...s, focus, videoFailed: null })
      }
      if (a.type === 'down' && s.detailsFocus !== 'gallery' && c.galleryCount > 0) {
        return none({ ...s, detailsFocus: 'gallery' })
      }
      if (a.type === 'up' && s.detailsFocus === 'gallery') {
        return none({ ...s, detailsFocus: s.detailsButton })
      }
      return none()
    }
    case 'galleryChanged': {
      const gallery = Math.min(s.gallery, Math.max(a.count - 1, 0))
      const detailsFocus = a.count === 0 && s.detailsFocus === 'gallery' ? s.detailsButton : s.detailsFocus
      return none({ ...s, gallery, detailsFocus })
    }
    case 'tabPrev':
    case 'tabNext': {
      if (s.screen !== 'browse' || c.tabs <= 0) return none()
      const dir = a.type === 'tabNext' ? 1 : -1
      const tab = (s.tab + dir + c.tabs) % c.tabs
      return none({ ...s, tab, focus: 0, videoFailed: null })
    }
    case 'accept': {
      if (c.focusedGameId === null) return none()
      if (s.screen === 'browse') {
        return { state: s, effects: [{ kind: 'launch', gameId: c.focusedGameId }] }
      }
      if (s.detailsFocus === 'play') return { state: s, effects: [{ kind: 'launch', gameId: c.focusedGameId }] }
      if (s.detailsFocus === 'playUntracked') {
        return { state: s, effects: [{ kind: 'launchUntracked', gameId: c.focusedGameId }] }
      }
      return none()
    }
    case 'details':
      if (s.screen !== 'browse' || c.focusedGameId === null) return none()
      return none({ ...s, screen: 'details', detailsFocus: 'play', detailsButton: 'play', gallery: 0 })
    case 'back':
      if (s.screen === 'details') return none({ ...s, screen: 'browse' })
      return { state: s, effects: [{ kind: 'exit' }] }
    case 'toggleSound':
      if (!c.videoVisible) return none()
      return none({ ...s, sound: !s.sound })
    case 'launched':
      return none({ ...s, suspended: true, awaitingFocusAfterBlur: false, launchedGameId: a.gameId })
    case 'windowBlur':
      return s.suspended ? none({ ...s, awaitingFocusAfterBlur: true }) : none()
    case 'windowFocus':
      return s.suspended && s.awaitingFocusAfterBlur
        ? none({ ...s, suspended: false, awaitingFocusAfterBlur: false })
        : none()
    case 'sessionEnded':
      return s.suspended && s.launchedGameId === a.gameId
        ? none({ ...s, suspended: false, awaitingFocusAfterBlur: false })
        : none()
    case 'gameClosed':
      return {
        state: {
          ...s,
          screen: 'browse',
          zone: 'games',
          suspended: false,
          awaitingFocusAfterBlur: false,
          launchedGameId: null,
          focus: a.indexInTab ?? s.focus,
        },
        effects: [{ kind: 'bringToFront' }, { kind: 'enterBigBox' }],
      }
    case 'listLoaded':
      return none({ ...s, zone: 'games', focus: a.initialIndex ?? 0 })
    case 'videoFailed':
      return none({ ...s, videoFailed: a.gameId })
  }
}
