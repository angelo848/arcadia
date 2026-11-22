// Pure navigation reducer of the Big Box [BBX-002, BBX-005..BBX-007, BBX-012..BBX-018, BBX-027, CON-022].

export type DetailsFocus = 'play' | 'playUntracked' | 'gallery'
export type DetailsButton = 'play' | 'playUntracked'

export type NavState = {
  screen: 'browse' | 'details'
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
export type NavEffect =
  | { kind: 'launch'; gameId: number }
  | { kind: 'launchUntracked'; gameId: number }
  | { kind: 'exit' }
  | { kind: 'bringToFront' }
  | { kind: 'enterBigBox' }

export const initialNav: NavState = {
  screen: 'browse',
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
  'prev', 'next', 'up', 'down', 'tabPrev', 'tabNext', 'accept', 'details', 'back', 'toggleSound',
])

export function bigBoxNav(s: NavState, a: NavAction, c: NavCtx): { state: NavState; effects: NavEffect[] } {
  const none = (state: NavState = s) => ({ state, effects: [] as NavEffect[] })

  if (s.suspended && inputTypes.has(a.type)) {
    return none()
  }

  switch (a.type) {
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
        if (cols <= 1) return none()
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
          suspended: false,
          awaitingFocusAfterBlur: false,
          launchedGameId: null,
          focus: a.indexInTab ?? s.focus,
        },
        effects: [{ kind: 'bringToFront' }, { kind: 'enterBigBox' }],
      }
    case 'listLoaded':
      return none({ ...s, focus: a.initialIndex ?? 0 })
    case 'videoFailed':
      return none({ ...s, videoFailed: a.gameId })
  }
}
