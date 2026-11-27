// Desktop theme: three columns (platforms, games, details) like the normal window [BBX-060, TDK-001..TDK-004, TDK-023, TDK-026, TDK-029].
// It only draws the model; details come ready from the shell and the art, the footer and the video belong to it.
import { useRef } from 'react'
import type { JSX } from 'react'
import { CENTER_PAD_X, CENTER_PAD_Y, HEADER_PX } from '../../desktopLayout'
import type { BigBoxThemeProps } from '../types'
import { AsideScrollContext } from './DesktopDetails'
import { DesktopGrid } from './DesktopGrid'
import { DesktopList } from './DesktopList'
import { DesktopSidebar } from './DesktopSidebar'
import { DesktopToolbar } from './DesktopToolbar'

const NO_TAB = () => {}

export function Desktop(p: BigBoxThemeProps): JSX.Element {
  const m = p.model
  const asideRef = useRef<HTMLElement>(null)
  const panel = m.screen === 'details'
  const gamesFocused = !panel && m.zone === 'games'
  const platformsFocused = !panel && m.zone === 'platforms'
  const Items =m.view === 'list' ? DesktopList : DesktopGrid

  const empty = m.tab === 0 ? 'Nenhum jogo na biblioteca' : 'Nenhum jogo nesta plataforma'
  const center = m.loading ? (
    <p className="m-0 p-4 text-bb-text-muted">Carregando jogos…</p>
  ) : m.games.length === 0 ? (
    <p className="m-0 p-4 text-bb-text-muted">{empty}</p>
  ) : (
    <div role="listbox" aria-label="Jogos" className="min-h-0 flex-1 overflow-hidden">
      <Items
        games={m.games}
        start={m.stripStart}
        focus={m.focus}
        columns={m.columns}
        zoneFocused={gamesFocused}
        onPick={p.onPick}
        onPlay={p.onPlay}
      />
    </div>
  )

  // The aside is one stable element through loading and loaded states; DesktopDetails draws inside it and scrolls it.
  const side = (
    <aside ref={asideRef} aria-label="Detalhes do jogo" className="h-full min-h-0 overflow-y-auto text-bb-text">
      <AsideScrollContext.Provider value={asideRef}>
        {p.details ?? (
          <p className="m-0 p-4 text-bb-text-muted">{m.focused ? 'Carregando jogo…' : 'Nenhum jogo selecionado'}</p>
        )}
      </AsideScrollContext.Provider>
    </aside>
  )

  return (
    <div data-testid="dsk-root" className="flex h-full min-h-0 flex-col text-bb-text">
      <header
        className="flex shrink-0 items-center px-4 text-lg font-bold text-bb-heading"
        style={{ height: HEADER_PX }}
      >
        Arcadia
      </header>
      {m.alert && (
        <div role="alert" className="shrink-0 px-4 py-2 text-bb-danger">
          {m.alert}
        </div>
      )}
      <div
        className="grid min-h-0 flex-1"
        style={{ gridTemplateColumns: 'var(--col-left) minmax(0, 1fr) var(--col-right)' }}
      >
        <DesktopSidebar tabs={m.tabs} tab={m.tab} focused={platformsFocused} onTab={p.onTab ?? NO_TAB} />
        <section
          aria-label="Jogos"
          className="flex min-h-0 min-w-0 flex-col"
          style={{ padding: `${CENTER_PAD_Y}px ${CENTER_PAD_X}px` }}
        >
          <DesktopToolbar total={m.total} sortLabel={m.sortLabel} view={m.view} />
          {center}
        </section>
        {side}
      </div>
    </div>
  )
}
