import { useState, type JSX } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AddGameForm } from './components/AddGameForm'
import { AgentIndicator } from './components/AgentIndicator'
import { ErrorBanner } from './components/ErrorBanner'
import { GameList } from './components/GameList'
import { SessionPanel } from './components/SessionPanel'
import { disconnectedStatus, useAgentStatus } from './lib/agent'
import {
  addGame,
  gamesKey,
  pickGameExe,
  useCurrentSession,
  useGames,
  useLaunchGame,
  useReloadOnReconnect,
  useSessionEvents,
  type GameView,
  type PickedExe,
  type SessionView,
} from './lib/games'

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

// LibraryBody picks what to show for the library [UI-004, UI-010]: while the
// list has not loaded or the call fails (the query retries every 2 s) only
// "Conectando ao agente…"; with [] the empty state, without counters.
function LibraryBody({
  games,
  current,
  pendingId,
  onPlay,
}: {
  games: GameView[] | undefined
  current: SessionView | null
  pendingId: number | null
  onPlay: (id: number) => void
}): JSX.Element {
  if (!games) {
    return (
      <section className="flex flex-col items-center justify-center gap-2 py-24 text-center">
        <p className="text-sm text-neutral-400">Conectando ao agente…</p>
      </section>
    )
  }
  if (games.length === 0) {
    return (
      <section className="flex flex-col items-center justify-center gap-2 py-24 text-center">
        <p className="text-xl font-medium text-neutral-200">Nenhum jogo ainda</p>
      </section>
    )
  }
  return <GameList games={games} current={current} pendingId={pendingId} onPlay={onPlay} />
}

export default function App() {
  const qc = useQueryClient()
  const status = useAgentStatus()
  const agent = status.data ?? disconnectedStatus
  const connected = agent.connected

  const games = useGames()
  const current = useCurrentSession()
  useSessionEvents()
  useReloadOnReconnect(connected)

  const launch = useLaunchGame()
  const add = useMutation<GameView, Error, { path: string; title: string }>({
    mutationFn: ({ path, title }) => addGame(path, title),
  })

  // Last failed action's message [UI-008] and the exe chosen in the dialog [UI-007].
  const [error, setError] = useState<string | null>(null)
  const [picked, setPicked] = useState<PickedExe | null>(null)

  const session = current.data ?? null
  const pendingId = launch.isPending ? (launch.variables ?? null) : null

  function play(id: number) {
    if (launch.isPending) {
      return
    }
    launch.mutate(id, {
      onSuccess: () => setError(null),
      onError: (e) => setError(errorMessage(e)),
    })
  }

  async function pick() {
    try {
      const p = await pickGameExe()
      // An empty path means the dialog was cancelled: nothing changes.
      if (p.path !== '') {
        setPicked(p)
      }
    } catch (e) {
      setError(errorMessage(e))
    }
  }

  function save(title: string) {
    if (!picked) {
      return
    }
    add.mutate(
      { path: picked.path, title },
      {
        onSuccess: async () => {
          setError(null)
          setPicked(null)
          await qc.invalidateQueries({ queryKey: gamesKey })
        },
        // The form stays open so the title can be fixed and saved again.
        onError: (e) => setError(errorMessage(e)),
      },
    )
  }

  const list = games.data && !games.isError ? games.data : undefined

  return (
    <main className="min-h-screen bg-neutral-950 text-neutral-100">
      <header className="flex items-center justify-between gap-4 border-b border-neutral-800 px-6 py-4">
        <h1 className="text-2xl font-semibold">Arcadia</h1>
        <div className="flex items-center gap-4">
          <button
            type="button"
            onClick={() => void pick()}
            className="rounded bg-neutral-800 px-4 py-1.5 text-sm font-medium text-neutral-100 hover:bg-neutral-700"
          >
            Adicionar jogo
          </button>
          <AgentIndicator status={agent} />
        </div>
      </header>
      {error !== null && <ErrorBanner message={error} onClose={() => setError(null)} />}
      {picked && (
        <AddGameForm
          key={picked.path}
          picked={picked}
          pending={add.isPending}
          onSave={save}
          onCancel={() => setPicked(null)}
        />
      )}
      {/* Without the agent the UI cannot know whether the session goes on [UI-016]. */}
      {connected && session && <SessionPanel session={session} />}
      <LibraryBody games={list} current={session} pendingId={pendingId} onPlay={play} />
    </main>
  )
}
