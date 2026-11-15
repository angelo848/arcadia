// Pure navigation reducer of the Big Box [BBX-002, BBX-005..BBX-007, BBX-012..BBX-018, BBX-027, CON-022].

export const PREFETCH_BEFORE_END = 10

export type DetailsFocus = 'play' | 'playUntracked' | 'description'

export type NavState = {
  screen: 'browse' | 'details'
  tab: number
  focus: number
  detailsFocus: DetailsFocus
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

export type NavEffect =
  | { kind: 'fetchNextPage' }
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
  gallery: 0,
  suspended: false,
  awaitingFocusAfterBlur: false,
  sound: false,
  videoFailed: null,
  launchedGameId: null,
}

const inputTypes = new Set<NavAction['type']>([
  'prev', 'next', 'up', 'down', 'tabPrev', 'tabNext', 'accept', 'details', 'back', 'toggleSound',
])

const detailsOrder: DetailsFocus[] = ['play', 'playUntracked', 'description']

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
        const gallery = Math.min(Math.max(s.gallery + dir, 0), Math.max(c.galleryCount - 1, 0))
        return none({ ...s, gallery })
      }
      const max = Math.max(c.total - 1, 0)
      const focus = Math.min(Math.max(s.focus + dir, 0), max)
      const effects: NavEffect[] = []
      if (focus !== s.focus && c.loaded < c.total && focus >= c.loaded - PREFETCH_BEFORE_END) {
        effects.push({ kind: 'fetchNextPage' })
      }
      return { state: { ...s, focus, videoFailed: null }, effects }
    }
    case 'up':
    case 'down': {
      if (s.screen !== 'details') return none()
      const i = detailsOrder.indexOf(s.detailsFocus) + (a.type === 'down' ? 1 : -1)
      const detailsFocus = detailsOrder[Math.min(Math.max(i, 0), detailsOrder.length - 1)]
      return none({ ...s, detailsFocus })
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
      return none({ ...s, screen: 'details', detailsFocus: 'play', gallery: 0 })
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
