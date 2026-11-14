import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ViewToggle } from './ViewToggle'
import { LibraryGrid } from './LibraryGrid'
import { LibraryList } from './LibraryList'
import { readLibraryView, writeLibraryView, type LibraryView } from '../lib/libraryView'
import type { GameView } from '../lib/library'

// Every element reports this box, so the virtualizers compute their window from a
// fixed size [UI-030]. Nothing here depends on real layout.
const BOX_WIDTH = 800
const BOX_HEIGHT = 600

const STORAGE_KEY = 'arcadia.libraryView'

class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () =>
      ({
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        bottom: BOX_HEIGHT,
        right: BOX_WIDTH,
        width: BOX_WIDTH,
        height: BOX_HEIGHT,
        toJSON: () => ({}),
      }) as DOMRect,
  )
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(BOX_HEIGHT)
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(BOX_WIDTH)
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
})

afterEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function game(id: number): GameView {
  return {
    id,
    platformId: 1,
    title: `Jogo ${id}`,
    platform: 'Windows',
    path: `C:\\Games\\jogo${id}.exe`,
    status: 'not_played',
    favorite: false,
    year: null,
    totalPlaySeconds: 0,
    playCount: 0,
    firstPlayedAt: '',
    lastPlayedAt: '',
  }
}

const games: GameView[] = [game(0), game(1), game(2)]

// Harness is the parent that owns the view: it reads the saved choice when it
// mounts, writes it on every change, and shows LibraryGrid or LibraryList with the
// same page [UI-030]. Where the real parent lives (App or a panel) is not fixed by
// tasks.md; this harness stands in for it.
function Harness({ onNeedMore = vi.fn() }: { onNeedMore?: () => void }) {
  const [view, setView] = useState<LibraryView>(() => readLibraryView())
  const change = (v: LibraryView) => {
    writeLibraryView(v)
    setView(v)
  }
  const page = {
    games,
    total: games.length,
    current: null,
    onNeedMore,
    selectedId: null,
    onSelect: vi.fn(),
    onLaunch: vi.fn(),
  }
  return (
    <>
      <ViewToggle view={view} onChange={change} />
      {view === 'grid' ? (
        <section data-testid="grid-view">
          <LibraryGrid {...page} />
        </section>
      ) : (
        <section data-testid="list-view">
          <LibraryList {...page} />
        </section>
      )}
    </>
  )
}

// switchTo clicks the toggle for the other view. The toggle is found by the name of
// the view it switches to ("Lista" from the grid, "Grade" from the list).
function switchTo(view: LibraryView): void {
  const name = view === 'list' ? /lista/i : /grade/i
  fireEvent.click(screen.getByRole('button', { name }))
}

describe('ViewToggle', () => {
  it('UI-030/clicar no alternador muda de grade para lista e de volta', () => {
    render(<Harness />)
    expect(screen.getByTestId('grid-view')).toBeInTheDocument()
    expect(screen.queryByTestId('list-view')).toBeNull()

    switchTo('list')
    expect(screen.getByTestId('list-view')).toBeInTheDocument()
    expect(screen.queryByTestId('grid-view')).toBeNull()
    expect(screen.getByText('Jogo 0')).toBeInTheDocument()

    switchTo('grid')
    expect(screen.getByTestId('grid-view')).toBeInTheDocument()
    expect(screen.queryByTestId('list-view')).toBeNull()
    expect(screen.getByText('Jogo 2')).toBeInTheDocument()
  })

  it('UI-030/a escolha fica guardada em localStorage e volta a valer ao abrir de novo; sem valor salvo é grade', () => {
    // The choice is a local preference: nothing goes to the agent [UI-030].
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)

    // Without a saved value the view is the grid.
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
    expect(readLibraryView()).toBe('grid')
    const first = render(<Harness />)
    expect(screen.getByTestId('grid-view')).toBeInTheDocument()

    switchTo('list')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('list')
    first.unmount()

    // Opening again brings the saved choice back.
    render(<Harness />)
    expect(screen.getByTestId('list-view')).toBeInTheDocument()
    expect(screen.queryByTestId('grid-view')).toBeNull()

    switchTo('grid')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('grid')

    expect(fetchSpy).not.toHaveBeenCalled()
  })
})
