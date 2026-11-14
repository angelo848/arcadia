import { useCallback, useEffect, useMemo, useReducer, useRef, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { AgentConflict } from './components/AgentConflict'
import { AgentIndicator } from './components/AgentIndicator'
import { DeleteDialog } from './components/DeleteDialog'
import { EmulatorsDialog } from './components/EmulatorsDialog'
import { ImportDialog } from './components/ImportDialog'
import { PlatformsDialog } from './components/PlatformsDialog'
import { StatsPage } from './components/StatsPage'
import { enterBigBox, exitBigBox, useBigBoxNav } from './lib/bigbox'
import { useEmulators } from './lib/emulators'
import { useCardImages } from './lib/metadata'
import { ProviderKeysDialog } from './components/ProviderKeysDialog'
import { ErrorBanner } from './components/ErrorBanner'
import { GameDetailsPanel } from './components/GameDetailsPanel'
import { GameEditDialog } from './components/GameEditDialog'
import { LibraryGrid } from './components/LibraryGrid'
import { LibraryList } from './components/LibraryList'
import { LibraryStates } from './components/LibraryStates'
import { LibraryToolbar } from './components/LibraryToolbar'
import { MetadataDialog } from './components/MetadataDialog'
import { PanelEmpty } from './components/PanelEmpty'
import { Sidebar } from './components/Sidebar'
import { SessionPanel } from './components/SessionPanel'
import { disconnectedStatus, useAgentElevated, useAgentStatus } from './lib/agent'
import { resetQuery, withSort, withText, type LibraryQuery } from './lib/filters'
import {
  useCurrentSession,
  useLaunchGame,
  useLaunchGameUntracked,
  useReloadOnReconnect,
  useSessionEvents,
  useToggleSession,
} from './lib/games'
import {
  libraryGamesKey,
  libraryKeys,
  useCatalog,
  useDeleteGame,
  useGame,
  useInfiniteGames,
  useUpdateGame,
} from './lib/library'
import { readLibraryView, writeLibraryView, type LibraryView } from './lib/libraryView'
import type { PendingLaunch } from './lib/pendingLaunch'
import { focusedGameId, initialSelection, selectionReducer, type SelectionEvent } from './lib/selection'

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

// hasFacets reports whether any filter other than the version is set (text included) [UI-035].
function hasFacets(q: LibraryQuery): boolean {
  return Object.keys(q.filter).some((k) => k !== 'version')
}

type GameDialog = 'edit' | 'metadata' | 'remove'

// GameDialogs opens the edit, metadata and removal modals for the game in focus [UIX-032..UIX-034].
// The details panel is the only place where these actions start.
function GameDialogs({
  gameId,
  dialog,
  genreSuggestions,
  onClose,
  onRemoved,
  onError,
}: {
  gameId: number
  dialog: GameDialog | null
  genreSuggestions: string[]
  onClose: () => void
  onRemoved: (id: number) => void
  onError: (message: string | null) => void
}): JSX.Element | null {
  const qc = useQueryClient()
  const detail = useGame(gameId)
  const emulators = useEmulators()
  const update = useUpdateGame()
  const remove = useDeleteGame()
  const [editError, setEditError] = useState<string | null>(null)

  if (dialog === 'edit') {
    return (
      <GameEditDialog
        game={detail.data}
        loading={detail.isPending}
        loadError={detail.isError ? errorMessage(detail.error) : null}
        genreSuggestions={genreSuggestions}
        emulators={emulators.data ?? []}
        pending={update.isPending}
        error={editError}
        onSave={(patch) => {
          update.mutate(
            { id: gameId, patch },
            {
              onSuccess: (saved) => {
                // The detail the agent returned is what the panel shows.
                qc.setQueryData(libraryKeys.game(gameId), saved)
                setEditError(null)
                onClose()
              },
              onError: (e) => setEditError(errorMessage(e)),
            },
          )
        }}
        onCancel={() => {
          setEditError(null)
          onClose()
        }}
      />
    )
  }
  if (dialog === 'metadata') {
    return <MetadataDialog gameId={gameId} onClose={onClose} />
  }
  if (dialog === 'remove') {
    return (
      <DeleteDialog
        title={detail.data?.title ?? ''}
        pending={remove.isPending}
        error={null}
        onCancel={onClose}
        onConfirm={() => {
          // Success empties the panel; any failure (e.g. -32002, DSK-018) closes the
          // confirmation and shows the message at the top, the game stays in focus.
          remove.mutate(gameId, {
            onSuccess: () => {
              onError(null)
              onClose()
              onRemoved(gameId)
              document.querySelector<HTMLElement>('[role="listbox"][aria-label="Jogos"]')?.focus()
            },
            onError: (e) => {
              onClose()
              onError(errorMessage(e))
            },
          })
        }}
      />
    )
  }
  return null
}

export default function App() {
  const qc = useQueryClient()
  const status = useAgentStatus()
  const agent = status.data ?? disconnectedStatus
  // A connected agent without administrator rights is not used (ADM-006): nothing is
  // read from it and no launch or session change is sent to it. While the check is pending,
  // the agent counts as usable.
  const elevation = useAgentElevated(agent)
  const conflict = agent.connected && elevation.data === false
  const connected = agent.connected && !conflict

  // The facets and the sort are one state; the free text is another, set by the debounced
  // SearchBox. The query sent to the agent is both together [UI-032..UI-035].
  const [base, setBase] = useState<LibraryQuery>(resetQuery)
  const [search, setSearch] = useState('')
  const query = useMemo(() => withText(base, search), [base, search])
  const queryKey = JSON.stringify(query)

  const [view, setView] = useState<LibraryView>(() => readLibraryView())
  // The game in focus of the details panel; UI state only, never sent to the agent [UIX-017].
  const [selection, dispatch] = useReducer(selectionReducer, initialSelection)
  const focusedId = focusedGameId(selection)
  const [dialog, setDialog] = useState<GameDialog | null>(null)

  // The list waits for the first agent status, so the connect does not reload a page still on its way [UI-009].
  const games = useInfiniteGames(query, { enabled: status.data !== undefined && !conflict })
  const catalog = useCatalog()
  // The genre names of the catalog, offered as tags in the edit form (TAG-001).
  const genreNames = useMemo(() => catalog.data?.genres.map((g) => g.name) ?? [], [catalog.data])
  const current = useCurrentSession()
  useSessionEvents()
  useReloadOnReconnect(connected)

  const launch = useLaunchGame()
  const untracked = useLaunchGameUntracked()
  const toggle = useToggleSession()

  // Last failed action's message [UI-008, UI-020, UI-028, UI-041].
  const [error, setError] = useState<string | null>(null)
  const [importing, setImporting] = useState(false)
  const [emulating, setEmulating] = useState(false)
  const [platformsOpen, setPlatformsOpen] = useState(false)
  const [keying, setKeying] = useState(false)
  // The statistics page replaces the library while it is open (phase 8).
  const [showingStats, setShowingStats] = useState(false)
  // Big Box: the library in full screen, driven by the gamepad or the keyboard (phase 8).
  const [bigBox, setBigBox] = useState(false)
  const leaveBigBox = () => {
    void exitBigBox()
    setBigBox(false)
  }
  // The game whose "Jogar" gets the focus once the panel shows it (A on a card, a choice in the
  // statistics) [UIX-044, UIX-045].
  const pendingPlayFocus = useRef<number | null>(null)
  const selectionRef = useRef(selection)
  selectionRef.current = selection
  const focusPlayOf = (gameId: number): boolean => {
    const s = selectionRef.current
    if (s.kind !== 'shown' || s.gameId !== gameId) {
      return false
    }
    const btn = document.querySelector<HTMLElement>('aside [data-bigbox-play]')
    btn?.focus()
    return btn !== null
  }
  const wantPlayFocus = (gameId: number) => {
    pendingPlayFocus.current = focusPlayOf(gameId) ? null : gameId
  }
  useEffect(() => {
    const id = pendingPlayFocus.current
    if (id === null) {
      return
    }
    // The pending focus belongs to one game: another selection or a failed read drops it.
    if (selection.kind === 'error' || selection.kind === 'empty') {
      pendingPlayFocus.current = null
    } else if (selection.kind === 'shown' && selection.gameId === id) {
      if (focusPlayOf(id)) {
        pendingPlayFocus.current = null
      }
    } else if (focusedGameId(selection) !== null && focusedGameId(selection) !== id) {
      pendingPlayFocus.current = null
    }
  })
  // B (or Esc): from the panel back to the card of the game in focus; from the statistics back to
  // the library; on the library it leaves the Big Box [UIX-044].
  const bigBoxBack = () => {
    const inPanel = document.activeElement?.closest('aside') != null
    if (inPanel && !showingStats) {
      const target =
        (focusedId !== null
          ? document.querySelector<HTMLElement>(`[role="option"][data-game-id="${focusedId}"]`)
          : null) ?? document.querySelector<HTMLElement>('[role="listbox"][aria-label="Jogos"]')
      if (target) {
        target.focus()
        return
      }
    }
    if (showingStats) {
      setShowingStats(false)
    } else {
      leaveBigBox()
    }
  }
  useBigBoxNav({
    enabled: bigBox,
    pageKey: showingStats ? 'stats' : 'library',
    onBack: bigBoxBack,
    onPlay: (id) => play(id),
    onOpenCard: wantPlayFocus,
  })
  useEffect(() => {
    const onF11 = (e: KeyboardEvent) => {
      if (e.key === 'F11') {
        e.preventDefault()
        void enterBigBox()
        setBigBox(true)
      }
    }
    window.addEventListener('keydown', onF11)
    return () => window.removeEventListener('keydown', onF11)
  }, [])

  const session = current.data ?? null

  const rows = useMemo(() => games.data?.pages.flatMap((p) => p.games) ?? [], [games.data])
  const total = games.data?.pages.at(-1)?.total
  // The cover of each loaded game, for the grid (MID-006).
  const images = useCardImages(useMemo(() => rows.map((g) => g.id), [rows]))
  const loaded = games.data !== undefined
  const filtered = hasFacets(query)
  const { fetchNextPage, hasNextPage, isFetchingNextPage } = games
  // Asks for the next page once per set of loaded games [UI-036].
  const needMore = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) {
      void fetchNextPage()
    }
  }, [fetchNextPage, hasNextPage, isFetchingNextPage])

  // The one launch in flight, with the mode it was started with [UI-021]. Only
  // one launch runs at a time, so at most one game's buttons are disabled.
  const pending: PendingLaunch =
    launch.isPending && launch.variables !== undefined
      ? { id: launch.variables, mode: 'tracked' }
      : untracked.isPending && untracked.variables !== undefined
        ? { id: untracked.variables, mode: 'untracked' }
        : null

  function play(id: number) {
    if (pending !== null) {
      return
    }
    launch.mutate(id, {
      onSuccess: () => setError(null),
      onError: (e) => setError(errorMessage(e)),
    })
  }

  // Untracked launch: no session or list change; the result only clears or sets the banner [UI-020].
  function playUntracked(id: number) {
    if (pending !== null) {
      return
    }
    untracked.mutate(id, {
      onSuccess: () => setError(null),
      onError: (e) => setError(errorMessage(e)),
    })
  }

  // Pause or resume the current session; the panel label follows the session state, not the click [UI-027, UI-028].
  function toggleSession() {
    if (toggle.isPending) {
      return
    }
    toggle.mutate(undefined, {
      onSuccess: () => setError(null),
      onError: (e) => setError(errorMessage(e)),
    })
  }

  function select(id: number) {
    dispatch({ type: 'select', gameId: id })
  }

  // A double click (or Enter) puts the game in focus and launches it [UIX-018].
  function launchFromList(id: number) {
    select(id)
    play(id)
  }

  function changeView(v: LibraryView) {
    setView(v)
    writeLibraryView(v)
  }

  // A filter change keeps the sort and resets nothing else; the text it carries (if any)
  // becomes the search text, so "Limpar filtros" also clears the search box [UI-035].
  function changeFilter(q: LibraryQuery) {
    setBase(withText(q, ''))
    setSearch(q.filter.text ?? '')
  }

  function changeSort(s: LibraryQuery['sort']) {
    setBase((b) => withSort(b, s))
  }

  // The panel reports what it read about the game in focus; a game that no longer exists
  // empties the panel and reloads the lists once [UIX-023].
  const onSelectionEvent = useCallback(
    (e: SelectionEvent) => {
      dispatch(e)
      if (e.type === 'readMissing') {
        setDialog(null)
        void qc.invalidateQueries({ queryKey: libraryGamesKey })
      }
    },
    [qc],
  )

  const topButton =
    'rounded bg-raised px-4 py-1.5 text-sm font-medium text-fg hover:bg-hover'

  return (
    <main className={`flex h-screen flex-col overflow-hidden bg-app text-fg${bigBox ? ' big-box' : ''}`}>
      {bigBox && (
        <button
          type="button"
          onClick={leaveBigBox}
          className="fixed right-4 top-4 z-30 rounded bg-raised px-3 py-1.5 text-sm text-fg hover:bg-hover"
        >
          Sair do Big Box (Esc)
        </button>
      )}
      <header className="flex items-center justify-between gap-4 border-b border-line px-6 py-4">
        <h1 className="text-2xl font-semibold">Arcadia</h1>
        <div className="flex items-center gap-4">
          <button type="button" onClick={() => setShowingStats(true)} className={topButton}>
            Estatísticas
          </button>
          <button type="button" onClick={() => setKeying(true)} className={topButton}>
            Provedores
          </button>
          <button type="button" onClick={() => setEmulating(true)} className={topButton}>
            Emuladores
          </button>
          <button type="button" onClick={() => setPlatformsOpen(true)} className={topButton}>
            Plataformas
          </button>
          <button type="button" onClick={() => setImporting(true)} className={topButton}>
            Importar
          </button>
          <AgentIndicator status={agent} elevated={elevation.data ?? undefined} />
        </div>
      </header>
      {conflict && <AgentConflict />}
      {error !== null && <ErrorBanner message={error} onClose={() => setError(null)} />}
      {emulating && <EmulatorsDialog onClose={() => setEmulating(false)} />}
      {platformsOpen && <PlatformsDialog onClose={() => setPlatformsOpen(false)} />}
      {keying && <ProviderKeysDialog onClose={() => setKeying(false)} />}
      {importing && (
        <ImportDialog
          onImported={() => {
            void qc.invalidateQueries({ queryKey: libraryGamesKey })
            void qc.invalidateQueries({ queryKey: libraryKeys.catalog })
          }}
          onClose={() => setImporting(false)}
        />
      )}
      {focusedId !== null && (
        <GameDialogs
          key={focusedId}
          gameId={focusedId}
          dialog={dialog}
          genreSuggestions={genreNames}
          onClose={() => setDialog(null)}
          onRemoved={(id) => dispatch({ type: 'removed', gameId: id })}
          onError={setError}
        />
      )}
      {/* Without the agent the UI cannot know whether the session goes on [UI-016]. */}
      {connected && session && (
        <SessionPanel session={session} onToggle={toggleSession} pending={toggle.isPending} />
      )}

      <div className="flex min-h-0 flex-1 flex-col">
        {showingStats ? (
          <StatsPage
            onOpenGame={(id) => {
              setShowingStats(false)
              if (bigBox) {
                wantPlayFocus(id)
              }
              select(id)
            }}
            onBack={() => setShowingStats(false)}
          />
        ) : (
          <div
            className="grid min-h-0 flex-1"
            style={{ gridTemplateColumns: 'var(--col-left) 1fr var(--col-right)' }}
          >
            <nav
              aria-label="Filtros da biblioteca"
              className="min-h-0 overflow-y-auto border-r border-line bg-surface px-3 py-4"
            >
              <Sidebar
                search={search}
                onSearchChange={setSearch}
                query={query}
                catalog={catalog.data}
                onChange={changeFilter}
              />
            </nav>
            <section aria-label="Jogos" className="flex min-h-0 min-w-0 flex-col gap-3 px-6 py-4">
              <LibraryToolbar
                total={total}
                sort={query.sort}
                view={view}
                onSortChange={changeSort}
                onViewChange={changeView}
              />
              <LibraryStates
                loaded={loaded}
                total={total}
                filtered={filtered}
                onClear={() => changeFilter(resetQuery())}
              />
              {loaded && total !== undefined && total > 0 && (
                <div className="flex min-h-0 flex-1 flex-col">
                  {view === 'grid' ? (
                    <LibraryGrid
                      key={queryKey}
                      games={rows}
                      images={images}
                      total={total}
                      current={session}
                      selectedId={focusedId}
                      keyboard={!bigBox}
                      onNeedMore={needMore}
                      onSelect={select}
                      onLaunch={launchFromList}
                    />
                  ) : (
                    <LibraryList
                      key={queryKey}
                      games={rows}
                      total={total}
                      current={session}
                      selectedId={focusedId}
                      keyboard={!bigBox}
                      onNeedMore={needMore}
                      onSelect={select}
                      onLaunch={launchFromList}
                    />
                  )}
                </div>
              )}
            </section>
            <aside aria-label="Detalhes do jogo" className="min-h-0 overflow-hidden border-l border-line bg-surface">
              {focusedId === null ? (
                <PanelEmpty onSelect={select} />
              ) : (
                <GameDetailsPanel
                  gameId={focusedId}
                  current={session}
                  pending={pending}
                  onPlay={play}
                  onPlayUntracked={playUntracked}
                  onEdit={() => setDialog('edit')}
                  onSearchMetadata={() => setDialog('metadata')}
                  onRemove={() => setDialog('remove')}
                  onSelectionEvent={onSelectionEvent}
                />
              )}
            </aside>
          </div>
        )}
      </div>
    </main>
  )
}
