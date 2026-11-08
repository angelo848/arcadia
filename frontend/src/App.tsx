import type { JSX } from 'react'
import { AgentIndicator } from './components/AgentIndicator'
import { LibraryEmpty } from './components/LibraryEmpty'
import { disconnectedStatus, useAgentStatus, useLibrarySummary, type LibrarySummaryView } from './lib/agent'

// LibraryView picks what to show for the library (DSK-007, DSK-008, UI-002):
// while the summary is unknown (loading or error, the query keeps retrying)
// only the empty title is shown, with no counters; with games the empty state
// is not rendered at all.
function LibraryView({ summary }: { summary: LibrarySummaryView | undefined }): JSX.Element {
  if (!summary) {
    return (
      <section className="flex flex-col items-center justify-center gap-2 py-24 text-center">
        <p className="text-xl font-medium text-neutral-200">Nenhum jogo ainda</p>
      </section>
    )
  }
  if (summary.games > 0) {
    return (
      <section className="flex flex-col items-center justify-center gap-2 py-24 text-center">
        <p className="text-sm text-neutral-400">{`${summary.games} jogos · ${summary.platforms} plataformas`}</p>
      </section>
    )
  }
  return <LibraryEmpty summary={summary} />
}

export default function App() {
  const status = useAgentStatus()
  const summary = useLibrarySummary()

  // While loading or on error the agent is shown as disconnected.
  return (
    <main className="min-h-screen bg-neutral-950 text-neutral-100">
      <header className="flex items-center justify-between border-b border-neutral-800 px-6 py-4">
        <h1 className="text-2xl font-semibold">Arcadia</h1>
        <AgentIndicator status={status.data ?? disconnectedStatus} />
      </header>
      <LibraryView summary={summary.data} />
    </main>
  )
}
