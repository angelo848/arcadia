import type { JSX } from 'react'
import type { LibrarySummaryView } from '../lib/agent'

// LibraryEmpty renders the empty library state (UI-002). The counters always
// use the plural form, by requirement.
export function LibraryEmpty({ summary }: { summary: LibrarySummaryView }): JSX.Element {
  return (
    <section className="flex flex-col items-center justify-center gap-2 py-24 text-center">
      <p className="text-xl font-medium text-neutral-200">Nenhum jogo ainda</p>
      <p className="text-sm text-neutral-400">{`${summary.games} jogos · ${summary.platforms} plataformas`}</p>
    </section>
  )
}
