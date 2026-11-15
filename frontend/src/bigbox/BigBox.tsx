// Big Box shell [BBX-001..BBX-005, BBX-007, BBX-013, BBX-018, BBX-019, BBX-022..BBX-024, BBX-030, CON-019..CON-024].
// Reads the library and launches games; it never writes anything else and never closes a game.
import { useCallback, useEffect, useMemo, useRef, useState, type JSX } from 'react'
import { BackgroundStage } from './BackgroundStage'
import { BigBoxDetails, galleryOf } from './BigBoxDetails'
import { Footer } from './Footer'
import { bigBoxNav, initialNav } from './nav'
import type { NavAction, NavCtx, NavEffect, NavState } from './nav'
import { resolveTheme } from './themes/registry'
import { useBigBoxEvents } from './useBigBoxEvents'
import { useFocusedData, useTabGames } from './useBigBoxData'
import { useBigBoxInput } from './useBigBoxInput'
import type { InputAction } from './useBigBoxInput'
import { buildTabs, buildViewModel, sessionMark, windowAround } from './viewModel'
import type { BigBoxThemeId, BigBoxTab } from './viewModel'
import { Faixa } from './themes/faixa/Faixa'
import { bringToFront, enterBigBox } from '../lib/bigbox'
import { launchGame, launchGameUntracked } from '../lib/games'
import { useCatalog } from '../lib/library'
import { useCardImages } from '../lib/metadata'

export type BigBoxProps = {
  initialPlatformId: number | null
  initialGameId: number | null
  themeId: BigBoxThemeId
  onExit: () => void
}

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

// BigBox waits for the catalog (the tabs come from it) and then mounts the shell.
export function BigBox(p: BigBoxProps): JSX.Element {
  const catalog = useCatalog()
  const theme = resolveTheme(p.themeId)
  if (catalog.isPending) {
    return <div data-bigbox-theme={theme.id} className="relative h-full w-full overflow-hidden bg-bb-bg" />
  }
  const { tabs, initialTab } = buildTabs(catalog.data, p.initialPlatformId)
  return <BigBoxShell {...p} tabs={tabs} initialTab={initialTab} />
}

function BigBoxShell(p: BigBoxProps & { tabs: BigBoxTab[]; initialTab: number }): JSX.Element {
  const { tabs, initialTab } = p
  const theme = resolveTheme(p.themeId)
  const Theme = theme.Component ?? Faixa

  const [nav, setNav] = useState<NavState>(() => ({ ...initialNav, tab: initialTab }))
  const navRef = useRef(nav)
  const ctxRef = useRef<NavCtx>({ tabs: tabs.length, total: 0, loaded: 0, galleryCount: 0, focusedGameId: null, videoVisible: false })
  const effectRef = useRef<(e: NavEffect) => void>(() => {})
  const gamesRef = useRef<Array<{ id: number }>>([])
  const pendingRef = useRef(false)
  const [alert, setAlert] = useState<string | null>(null)
  const [scroll, setScroll] = useState({ n: 0, dir: 1 })

  // One entry for every action: the pure reducer decides, the shell runs the effects.
  const dispatchNav = useCallback((a: NavAction) => {
    let action = a
    if (action.type === 'gameClosed') {
      const closedId = action.gameId
      const idx = gamesRef.current.findIndex((g) => g.id === closedId)
      action = { ...action, indexInTab: idx >= 0 ? idx : null }
    }
    const r = bigBoxNav(navRef.current, action, ctxRef.current)
    navRef.current = r.state
    setNav(r.state)
    for (const e of r.effects) effectRef.current(e)
  }, [])

  const tabDef = tabs[nav.tab] ?? tabs[0]
  const tabGames = useTabGames(tabDef.platformId)
  const games = tabGames.games
  gamesRef.current = games
  const focusedGame = games[nav.focus]
  const focusedId = focusedGame?.id ?? null
  const data = useFocusedData(focusedId)

  // First load of each tab: the focus goes to the game of the normal window (first tab only) or the first game [BBX-027].
  const loadedTab = useRef<number | null>(null)
  useEffect(() => {
    if (tabGames.isPending || loadedTab.current === nav.tab) return
    const first = loadedTab.current === null
    loadedTab.current = nav.tab
    const idx = first && p.initialGameId !== null ? games.findIndex((g) => g.id === p.initialGameId) : -1
    dispatchNav({ type: 'listLoaded', initialIndex: idx >= 0 ? idx : null })
  })

  const { slice } = windowAround(games, nav.focus)
  const covers = useCardImages(useMemo(() => slice.map((g) => g.id), [slice]))
  const model = buildViewModel({
    screen: nav.screen,
    detailsFocus: nav.detailsFocus,
    tabs,
    tab: nav.tab,
    games,
    total: tabGames.total,
    focus: nav.focus,
    covers,
    media: data.media,
    description: data.description,
    genres: data.genres,
    session: data.session,
    sound: nav.sound,
    videoFailed: nav.videoFailed,
    alert,
  })
  const gallery = galleryOf(data.media)
  ctxRef.current = {
    tabs: tabs.length,
    total: tabGames.total,
    loaded: tabGames.loaded,
    galleryCount: gallery.length,
    focusedGameId: focusedId,
    videoVisible: model.videoVisible,
  }

  const startLaunch = useCallback((gameId: number, untracked: boolean) => {
    if (pendingRef.current || navRef.current.suspended) return
    pendingRef.current = true
    setAlert(null)
    const run = untracked ? launchGameUntracked(gameId) : launchGame(gameId).then(() => undefined)
    run
      .then(
        () => dispatchNav({ type: 'launched', gameId }),
        (e: unknown) => setAlert(errorMessage(e)),
      )
      .finally(() => {
        pendingRef.current = false
      })
  }, [dispatchNav])

  const { onExit } = p
  const { fetchNextPage } = tabGames
  effectRef.current = (e: NavEffect) => {
    switch (e.kind) {
      case 'fetchNextPage':
        fetchNextPage()
        break
      case 'launch':
        startLaunch(e.gameId, false)
        break
      case 'launchUntracked':
        startLaunch(e.gameId, true)
        break
      case 'exit':
        onExit()
        break
      case 'bringToFront':
        void bringToFront()
        break
      case 'enterBigBox':
        void enterBigBox()
        break
    }
  }

  useBigBoxEvents({ dispatch: dispatchNav, launchedGameId: nav.launchedGameId, active: true })
  useBigBoxInput({
    enabled: true,
    axis: theme.axis,
    screen: nav.screen,
    onAction: (type: InputAction) => {
      setAlert(null)
      if (type === 'scrollUp' || type === 'scrollDown') {
        if (!navRef.current.suspended && navRef.current.screen === 'details') {
          setScroll((s) => ({ n: s.n + 1, dir: type === 'scrollUp' ? -1 : 1 }))
        }
        return
      }
      dispatchNav({ type } as NavAction)
    },
  })

  const details =
    nav.screen === 'details' && data.detail ? (
      <BigBoxDetails
        game={data.detail}
        media={data.media}
        session={sessionMark(data.session, data.detail.id)}
        detailsFocus={nav.detailsFocus}
        gallery={nav.gallery}
        sound={nav.sound}
        scrollSignal={scroll.n * scroll.dir}
        onPlay={() => startLaunch(data.detail!.id, false)}
        onPlayUntracked={() => startLaunch(data.detail!.id, true)}
      />
    ) : null

  return (
    <div data-bigbox-theme={theme.id} className="relative flex h-full w-full flex-col overflow-hidden bg-bb-bg text-bb-text">
      <BackgroundStage
        gameId={focusedId}
        artUrl={model.focused?.backgroundUrl ?? null}
        videoUrl={model.focused?.videoUrl ?? null}
        active={nav.screen === 'browse' && !nav.suspended}
        sound={nav.sound}
        failed={focusedId !== null && nav.videoFailed === focusedId}
        onVideoError={(gameId) => dispatchNav({ type: 'videoFailed', gameId })}
        onVideoVisible={() => {}}
      />
      <div className="relative z-10 min-h-0 flex-1">
        <Theme
          model={model}
          details={details}
          onPick={(index) => dispatchNav({ type: 'listLoaded', initialIndex: index })}
          onPlay={(index) => {
            const g = games[index]
            if (g) startLaunch(g.id, false)
          }}
          onDetails={() => dispatchNav({ type: 'details' })}
        />
      </div>
      <div className="relative z-10 px-8 py-3" style={{ background: 'var(--arc-bb-scrim)' }}>
        <Footer hints={model.hints} />
      </div>
    </div>
  )
}
