import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LibraryEmpty } from './LibraryEmpty'
import type { LibrarySummaryView } from '../lib/agent'

describe('LibraryEmpty', () => {
  it('UI-002/mostra Nenhum jogo ainda', () => {
    const summary: LibrarySummaryView = { games: 0, platforms: 0 }
    render(<LibraryEmpty summary={summary} />)

    expect(screen.getByText('Nenhum jogo ainda')).toBeInTheDocument()
  })

  // Plural is fixed: no singular form even for M = 1 (requirements.md UI-002).
  const counts: ReadonlyArray<{ platforms: number; want: string }> = [
    { platforms: 0, want: '0 jogos · 0 plataformas' },
    { platforms: 1, want: '0 jogos · 1 plataformas' },
    { platforms: 3, want: '0 jogos · 3 plataformas' },
  ]

  for (const tc of counts) {
    it(`UI-002/mostra 0 jogos · M plataformas (M=${tc.platforms})`, () => {
      const summary: LibrarySummaryView = { games: 0, platforms: tc.platforms }
      render(<LibraryEmpty summary={summary} />)

      expect(screen.getByText(tc.want)).toBeInTheDocument()
      expect(screen.getByText('Nenhum jogo ainda')).toBeInTheDocument()
    })
  }
})
