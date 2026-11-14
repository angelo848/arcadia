import { useCallback, useEffect, useMemo, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { AgentConflict } from './components/AgentConflict'
import { AgentIndicator } from './components/AgentIndicator'
import { DeleteDialog } from './components/DeleteDialog'
import { EmulatorsDialog } from './components/EmulatorsDialog'
import { ImportDialog } from './components/ImportDialog'
import { PlatformsDialog } from './components/PlatformsDialog'
import { StatsPage } from './components/StatsPage'
import { enterBigBox, exitBigBox, useBigBoxNav } from './lib/bigbox'
import { useEmulators, type EmulatorView } from './lib/emulators'
import { useCovers } from './lib/metadata'
import { ProviderKeysDialog } from './components/ProviderKeysDialog'
import { ErrorBanner } from './components/ErrorBanner'
import { FilterPanel } from './components/FilterPanel'
import { GameEditForm } from './components/GameEditForm'
import { GamePage } from './components/GamePage'
import { LibraryGrid } from './components/LibraryGrid'
import { LibraryList } from './components/LibraryList'
import { LibraryStates } from './components/LibraryStates'
import { SearchBox } from './components/SearchBox'
import { SessionPanel } from './components/SessionPanel'
import { SortSelect } from './components/SortSelect'
import { ViewToggle } from './components/ViewToggle'
import { disconnectedStatus, useAgentElevated, useAgentStatus } from './lib/agent'
import { resetQuery, withSort, withText, type LibraryQuery } from './lib/filters'
import {
  useCurrentSession,
  useLaunchGame,
  useLaunchGameUntracked,
  useReloadOnReconnect,
  useSessionEvents,
  useToggleSession,
  type SessionView,
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

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

// hasFacets reports whether any filter other than the version is set (text included) [UI-035].
function hasFacets(q: LibraryQuery): boolean {
  return Object.keys(q.filter).some((k) => k !== 'version')
}

type GameScreenProps = {
  id: number
  current: SessionView | null
  genreSuggestions: string[]
  emulators: EmulatorView[]
  editing: boolean
  editError: string | null
  savePending: boolean
  removing: boolean
  removePending: boolean
  onBack: () => void
  onEdit: () => void
  onCancelEdit: () => void
  onSave: (patch: Record<string, unknown>) => void
  onRemove: () => void
  onCancelRemove: () => void
  onConfirmRemove: () => void
}

// GameScreen shows the game page, or its edit form [UI-039], and the removal confirmation
// [UI-041] over it. The page is the only place where these actions start.
function GameScreen({
  id,
  current,
  genreSuggestions,
  emulators,
  editing,
  editError,
  savePending,
  removing,
  removePending,
  onBack,
  onEdit,
  onCancelEdit,
  onSave,
  onRemove,
  onCancelRemove,
  onConfirmRemove,
}: GameScreenProps): JSX.Element {
  const detail = useGame(id)
  const title = detail.data?.title ?? ''

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {editing && detail.data !== undefined ? (
        <GameEditForm
          key={id}
          game={detail.data}
          genreSuggestions={genreSuggestions}
          emulators={emulators}
          onSave={onSave}
          onCancel={onCancelEdit}
          pending={savePending}
          error={editError}
        />
      ) : (
        <GamePage id={id} current={current} onBack={onBack} onEdit={onEdit} onRemove={onRemove} />
      )}
      {removing && (
        <DeleteDialog
          title={title}
          pending={removePending}
          error={null}
          onConfirm={onConfirmRemove}
          onCancel={onCancelRemove}
        />
      )}
    </div>
  )
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
  // The open game and its sub-state; null means the library is shown [UI-037].
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [editing, setEditing] = useState(false)
  const [editError, setEditError] = useState<string | null>(null)
  const [removing, setRemoving] = useState(false)

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
  const update = useUpdateGame()
  const remove = useDeleteGame()

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
  // B (or Esc) goes back one page; on the library it leaves the Big Box (BIG-002).
  const bigBoxBack = () => {
    if (selectedId !== null) {
      closeGame()
    } else if (showingStats) {
      setShowingStats(false)
    } else {
      leaveBigBox()
    }
  }
  useBigBoxNav({
    enabled: bigBox,
    pageKey: `${selectedId ?? 'library'}|${showingStats ? 'stats' : ''}`,
    onBack: bigBoxBack,
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
  const emulators = useEmulators()

  const session = current.data ?? null

  const rows = useMemo(() => games.data?.pages.flatMap((p) => p.games) ?? [], [games.data])
  const total = games.data?.pages.at(-1)?.total
  // The cover of each loaded game, for the grid (MID-006).
  const covers = useCovers(useMemo(() => rows.map((g) => g.id), [rows]))
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

  function openGame(id: number) {
    setSelectedId(id)
    setEditing(false)
    setEditError(null)
    setRemoving(false)
  }

  function closeGame() {
    setSelectedId(null)
    setEditing(false)
    setEditError(null)
    setRemoving(false)
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

  // The edit form sends the changed fields; success closes it, the page reloads through
  // useUpdateGame [UI-039, UI-042]. The error stays in the form [UI-039].
  function saveEdit(patch: Record<string, unknown>) {
    if (selectedId === null) {
      return
    }
    update.mutate(
      { id: selectedId, patch },
      {
        onSuccess: () => {
          setEditing(false)
          setEditError(null)
        },
        onError: (e) => setEditError(errorMessage(e)),
      },
    )
  }

  // Success goes back to the library, where the grid no longer has the game [UI-041, UI-042].
  // Any failure (e.g. -32002, DSK-018) closes the confirmation and shows the message at the top.
  function confirmRemove() {
    if (selectedId === null) {
      return
    }
    remove.mutate(selectedId, {
      onSuccess: () => {
        setError(null)
        closeGame()
      },
      onError: (e) => {
        setRemoving(false)
        setError(errorMessage(e))
      },
    })
  }

  // The empty library has its add button in LibraryStates; the header keeps it otherwise.

  return (
    <main className={`flex h-screen flex-col overflow-hidden bg-neutral-950 text-neutral-100${bigBox ? ' big-box' : ''}`}>
      {bigBox && (
        <button
          type="button"
          onClick={leaveBigBox}
          className="fixed right-4 top-4 z-30 rounded bg-neutral-800 px-3 py-1.5 text-sm text-neutral-100 hover:bg-neutral-700"
        >
          Sair do Big Box (Esc)
        </button>
      )}
      <header className="flex items-center justify-between gap-4 border-b border-neutral-800 px-6 py-4">
        <h1 className="text-2xl font-semibold">Arcadia</h1>
        <div className="flex items-center gap-4">
          <button
            type="button"
            onClick={() => setShowingStats(true)}
            className="rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700"
          >
            Estatísticas
          </button>
          <button
            type="button"
            onClick={() => setKeying(true)}
            className="rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700"
          >
            Provedores
          </button>
          <button
            type="button"
            onClick={() => setEmulating(true)}
            className="rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700"
          >
            Emuladores
          </button>
          <button
            type="button"
            onClick={() => setPlatformsOpen(true)}
            className="rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700"
          >
            Plataformas
          </button>
          <button
            type="button"
            onClick={() => setImporting(true)}
            className="rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700"
          >
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
      {/* Without the agent the UI cannot know whether the session goes on [UI-016]. */}
      {connected && session && (
        <SessionPanel session={session} onToggle={toggleSession} pending={toggle.isPending} />
      )}

      <div className="flex min-h-0 flex-1 flex-col">
        {selectedId === null && showingStats ? (
          <StatsPage
            onOpenGame={(id) => {
              setShowingStats(false)
              openGame(id)
            }}
            onBack={() => setShowingStats(false)}
          />
        ) : selectedId !== null ? (
          <GameScreen
            key={selectedId}
            id={selectedId}
            current={session}
            genreSuggestions={genreNames}
            emulators={emulators.data ?? []}
            editing={editing}
            editError={editError}
            savePending={update.isPending}
            removing={removing}
            removePending={remove.isPending}
            onBack={closeGame}
            onEdit={() => {
              setEditError(null)
              setEditing(true)
            }}
            onCancelEdit={() => {
              setEditing(false)
              setEditError(null)
            }}
            onSave={saveEdit}
            onRemove={() => setRemoving(true)}
            onCancelRemove={() => setRemoving(false)}
            onConfirmRemove={confirmRemove}
          />
        ) : (
          <section className="flex min-h-0 flex-1 flex-col gap-3 px-6 py-4">
            <div className="flex flex-wrap items-center gap-3">
              <ViewToggle view={view} onChange={changeView} />
              <div className="min-w-[200px] flex-1">
                <SearchBox text={search} onChange={setSearch} />
              </div>
              <SortSelect sort={query.sort} onChange={changeSort} />
            </div>
            <FilterPanel query={query} catalog={catalog.data} onChange={changeFilter} />
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
                    covers={covers}
                    total={total}
                    current={session}
                    pending={pending}
                    onNeedMore={needMore}
                    onOpen={openGame}
                    onPlay={play}
                    onPlayUntracked={playUntracked}
                  />
                ) : (
                  <LibraryList
                    key={queryKey}
                    games={rows}
                    total={total}
                    current={session}
                    pending={pending}
                    onNeedMore={needMore}
                    onOpen={openGame}
                    onPlay={play}
                    onPlayUntracked={playUntracked}
                  />
                )}
              </div>
            )}
          </section>
        )}
      </div>
    </main>
  )
}
