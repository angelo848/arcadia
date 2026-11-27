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
import { shouldPrefetch, shouldPrefetchGrid, useFocusedData, useTabGames } from './useBigBoxData'
import { useViewport } from './useViewport'
import { useBigBoxInput } from './useBigBoxInput'
import type { InputAction } from './useBigBoxInput'
import { buildTabs, buildViewModel, effectiveRows, gridWindowAt, sessionMark, stripWindow, GRID_MAX_NODES } from './viewModel'
import type { BigBoxThemeId, BigBoxTab, LibraryView } from './viewModel'
import { desktopLabels } from './desktopLabels'
import { useGridStart } from './useGridStart'
import type { SortKey } from '../lib/filters'
import { Faixa } from './themes/faixa/Faixa'
import { bringToFront, enterBigBox } from '../lib/bigbox'
import { launchGame, launchGameUntracked } from '../lib/games'
import { useCatalog } from '../lib/library'
import { useCardImages } from '../lib/metadata'
import { TTB_FETCH_DELAY_MS, timeToBeatRows, timeToBeatSource, useTimeToBeat } from '../lib/timeToBeat'

export type BigBoxProps = {
  initialPlatformId: number | null
  initialGameId: number | null
  themeId: BigBoxThemeId
  librarySort: SortKey // order of the normal window, used by themes with sort 'inherit' [BBX-067]
  libraryView: LibraryView // view of the normal window; read only [BBX-068]
  onExit: () => void
}

// useMinuteClock gives the view model a stable "now" that only changes once a minute.
function useMinuteClock(): Date {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 60_000)
    return () => clearInterval(t)
  }, [])
  return now
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
  const { tabs, initial } = buildTabs(catalog.data, p.initialPlatformId, theme.tabs, theme.layout === 'columns')
  return <BigBoxShell {...p} tabs={tabs} initialTab={initial} />
}

function BigBoxShell(p: BigBoxProps & { tabs: BigBoxTab[]; initialTab: number }): JSX.Element {
  const { tabs, initialTab } = p
  const theme = resolveTheme(p.themeId)
  const Theme = theme.Component ?? Faixa

  const [nav, setNav] = useState<NavState>(() => ({ ...initialNav, tab: initialTab }))
  const navRef = useRef(nav)
  const ctxRef = useRef<NavCtx>({
    tabs: tabs.length, total: 0, loaded: 0, galleryCount: 0, focusedGameId: null, videoVisible: false,
    columns: theme.columns, axis: theme.axis, layout: theme.layout ?? 'screens',
  })
  const recenterRef = useRef<() => void>(() => {})
  const effectRef = useRef<(e: NavEffect) => void>(() => {})
  const gamesRef = useRef<Array<{ id: number }>>([])
  const pendingRef = useRef(false)
  const [alert, setAlert] = useState<string | null>(null)
  const [scroll, setScroll] = useState({ n: 0, dir: 1 })

  // One entry for every action: the pure reducer decides, the shell runs the effects.
  const dispatchNav = useCallback((a: NavAction) => {
    let action = a
    if (action.type === 'gameClosed') {
      recenterRef.current()
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
  const viewport = useViewport()
  const isGrid = theme.axis === 'grid'
  const layout = theme.layout ?? 'screens'
  const view = p.libraryView
  const columns = isGrid ? Math.max(theme.gridColumns?.(viewport, view) ?? theme.columns, 1) : theme.columns
  // Themes that predate the view receive it only when it is not the default grid.
  const rawRows = theme.gridRows ? (view === 'grid' ? theme.gridRows(viewport) : theme.gridRows(viewport, view)) : 1
  const maxNodes = theme.maxNodes ?? GRID_MAX_NODES
  const rows = isGrid ? effectiveRows(rawRows, columns, maxNodes) : 1
  const listCapacity = theme.listCapacity(viewport)
  // A grid theme shows R' whole rows of C cards; a strip theme shows V whole covers.
  const capacity = isGrid ? columns * rows : listCapacity
  const inherited = theme.sort === 'inherit'
  const sort: SortKey = theme.sort === 'inherit' ? p.librarySort : theme.sort
  const tabGames = useTabGames(tabDef, sort, isGrid ? { kind: 'grid', columns, rows, maxNodes } : { kind: 'strip', capacity })
  const games = tabGames.games
  gamesRef.current = games
  const { loaded, total, exhausted, error: pageError, isPending, isFetchingNextPage, fetchNextPage } = tabGames
  const needMore = useCallback(
    (focus: number) =>
      isGrid
        ? shouldPrefetchGrid(loaded, total, focus, columns, rows, exhausted, maxNodes)
        : shouldPrefetch(loaded, total, focus, capacity, exhausted),
    [isGrid, loaded, total, columns, rows, capacity, exhausted, maxNodes],
  )
  const { startRow, recenter } = useGridStart({
    mode: theme.gridScroll ?? 'focusTop', focus: nav.focus, total, columns, rows, tabKey: tabDef.key,
  })
  recenterRef.current = recenter

  // Entry [BBX-027]: the first tab shows "Carregando jogos…" until the game of the normal window has its strip complete.
  const [entryDone, setEntryDone] = useState(false)
  const entering = !entryDone && nav.tab === initialTab
  const loading = isPending || entering
  const { initialGameId } = p
  useEffect(() => {
    if (!entering || isPending) return
    const idx = initialGameId !== null ? games.findIndex((g) => g.id === initialGameId) : -1
    if (!pageError && needMore(idx >= 0 ? idx : 0)) {
      if (!isFetchingNextPage) fetchNextPage()
      return
    }
    recenter()
    dispatchNav({ type: 'listLoaded', initialIndex: idx >= 0 ? idx : null })
    setEntryDone(true)
  }, [entering, isPending, games, initialGameId, pageError, needMore, isFetchingNextPage, fetchNextPage, dispatchNav, recenter])

  // Read ahead of the focus: one page at a time, retried after a failure only when the focus moves [BBX-007].
  const lastAsk = useRef<string | null>(null)
  useEffect(() => {
    if (entering || isPending || isFetchingNextPage) return
    if (!needMore(nav.focus)) return
    const ask = `${nav.tab}:${nav.focus}`
    if (pageError && lastAsk.current === ask) return
    lastAsk.current = ask
    fetchNextPage()
  }, [entering, isPending, isFetchingNextPage, needMore, nav.focus, nav.tab, pageError, fetchNextPage])

  const focusedId = loading ? null : (games[nav.focus]?.id ?? null)
  const data = useFocusedData(focusedId)

  const win = isGrid ? gridWindowAt(total, startRow, columns, rows) : stripWindow(total, nav.focus, capacity, theme.anchor)
  const coverKey = loading ? '' : games.slice(win.start, Math.min(win.end, loaded)).map((g) => g.id).join(',')
  const covers = useCardImages(useMemo(() => (coverKey === '' ? [] : coverKey.split(',').map(Number)), [coverKey]))
  const gallery = galleryOf(data.media)

  // Time to beat: the cache is always read; the provider is asked in Detalhes at once and in Navegar only when the theme shows it [BBX-059].
  const ttb = useTimeToBeat(focusedId, {
    fetchAfterMs: nav.suspended ? null : nav.screen === 'details' ? 0 : theme.timeToBeatInBrowse ? TTB_FETCH_DELAY_MS : null,
  })
  const ttbRows = timeToBeatRows(ttb.data)
  const ttbSource = timeToBeatSource(ttb.data)
  const now = useMinuteClock()
  const model = buildViewModel({
    axis: theme.axis,
    columns,
    anchor: theme.anchor,
    sort,
    sortInherited: inherited,
    layout,
    zone: nav.zone,
    view,
    startRow: isGrid ? startRow : undefined,
    now,
    timeToBeat: ttb.data,
    tabsLabel: theme.tabs === 'collections' ? 'Aba' : 'Plataforma',
    screen: nav.screen,
    detailsFocus: nav.detailsFocus,
    hasGallery: gallery.length > 0,
    tabs,
    tab: nav.tab,
    games,
    total,
    loaded,
    capacity,
    loading,
    focus: nav.focus,
    covers,
    media: data.media,
    description: data.description,
    genres: data.genres,
    session: data.session,
    sound: nav.sound,
    videoFailed: nav.videoFailed,
    alert: alert ?? (pageError ? `Falha ao ler os jogos: ${errorMessage(pageError)}` : null),
  })
  ctxRef.current = {
    tabs: tabs.length,
    total,
    loaded,
    galleryCount: gallery.length,
    focusedGameId: focusedId,
    videoVisible: model.videoVisible,
    columns,
    axis: theme.axis,
    layout,
  }

  // The gallery of the focused game can shrink while the details are open [BBX-032, BBX-034].
  const galleryCount = gallery.length
  useEffect(() => {
    dispatchNav({ type: 'galleryChanged', count: galleryCount })
  }, [galleryCount, dispatchNav])

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
  effectRef.current = (e: NavEffect) => {
    switch (e.kind) {
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

  const Details = theme.Details ?? BigBoxDetails
  const columnsLayout = layout === 'columns'
  const detail = data.detail
  const extra =
    columnsLayout && detail
      ? {
          active: nav.screen === 'details',
          platformIconUrl: tabs.find((t) => t.key === `p:${detail.platformId}`)?.iconUrl ?? null,
          coverUrl: covers[detail.id]?.url ?? null,
          preview: {
            gameId: detail.id,
            videoUrl: model.focused?.videoUrl ?? null,
            active: nav.screen === 'browse' && !nav.suspended,
            failed: nav.videoFailed === detail.id,
            sound: nav.sound,
            onVideoError: (gameId: number) => dispatchNav({ type: 'videoFailed', gameId }),
          },
          labels: desktopLabels(detail),
        }
      : {}
  const details =
    (nav.screen === 'details' || columnsLayout) && data.detail ? (
      <Details
        {...extra}
        game={data.detail}
        media={data.media}
        session={sessionMark(data.session, data.detail.id)}
        detailsFocus={nav.detailsFocus}
        gallery={nav.gallery}
        sound={nav.sound}
        scrollSignal={scroll.n * scroll.dir}
        onPlay={() => startLaunch(data.detail!.id, false)}
        onPlayUntracked={() => startLaunch(data.detail!.id, true)}
        timeToBeat={ttbRows}
        timeToBeatSource={ttbSource}
      />
    ) : null

  return (
    <div data-bigbox-theme={theme.id} className="relative h-full w-full overflow-hidden bg-bb-bg text-bb-text">
      <BackgroundStage
        backdrop={theme.backdrop}
        gameId={focusedId}
        artUrl={model.focused?.backgroundUrl ?? null}
        videoUrl={theme.videoSlot === 'background' ? (model.focused?.videoUrl ?? null) : null}
        active={nav.screen === 'browse' && !nav.suspended}
        sound={nav.sound}
        failed={focusedId !== null && nav.videoFailed === focusedId}
        onVideoError={(gameId) => dispatchNav({ type: 'videoFailed', gameId })}
        onVideoVisible={() => {}}
      />
      <div
        data-bb-safe=""
        className="relative z-10 flex h-full min-h-0 flex-col gap-3"
        style={{ padding: 'var(--arc-bb-safe-y) var(--arc-bb-safe-x)' }}
      >
        <div className="min-h-0 flex-1">
          <Theme
            model={model}
            details={details}
            onPick={(index) => dispatchNav({ type: 'listLoaded', initialIndex: index })}
            onPlay={(index) => {
              const g = games[index]
              if (g) startLaunch(g.id, false)
            }}
            onDetails={() => dispatchNav({ type: 'details' })}
            onVideoError={(gameId) => dispatchNav({ type: 'videoFailed', gameId })}
            {...(columnsLayout ? { onTab: (index: number) => dispatchNav({ type: 'tabSet', index }) } : {})}
          />
        </div>
        <Footer hints={model.hints} />
      </div>
    </div>
  )
}
