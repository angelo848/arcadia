import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { StatsPage } from './StatsPage'
import type { StatsSummary } from '../lib/stats'

const summaryMock = vi.fn()
const weeklyMock = vi.fn()

vi.mock('../lib/stats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/stats')>()
  return {
    ...actual,
    useStatsSummary: (period: string) => summaryMock(period),
    useStatsWeekly: (weeks: number) => weeklyMock(weeks),
  }
})

const summary: StatsSummary = {
  totalSeconds: 5400,
  importedSeconds: 3600,
  games: [
    { gameId: 7, title: 'Celeste', platform: 'PC', sessions: 2, activeSeconds: 1800, importedSeconds: 0, lastPlayedAt: '' },
    { gameId: 8, title: 'Hades', platform: 'PC', sessions: 1, activeSeconds: 3600, importedSeconds: 3600, lastPlayedAt: '' },
  ],
  platforms: [{ platformId: 1, name: 'PC', activeSeconds: 5400, importedSeconds: 3600 }],
}

describe('StatsPage (phase 8)', () => {
  beforeEach(() => {
    summaryMock.mockReset()
    weeklyMock.mockReset()
    summaryMock.mockReturnValue({ data: summary, isError: false })
    weeklyMock.mockReturnValue({
      data: { weeks: [{ start: '2026-11-09', seconds: 600, current: true }, { start: '2026-11-02', seconds: 0, current: false }] },
      isError: false,
    })
  })

  it('EST-004/abre com o período de 7 dias e recalcula ao trocar', () => {
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(summaryMock).toHaveBeenLastCalledWith('7d')

    fireEvent.change(screen.getByLabelText('Período'), { target: { value: '30d' } })
    expect(summaryMock).toHaveBeenLastCalledWith('30d')
  })

  it('EST-001/o jogo abre a página dele', () => {
    const onOpenGame = vi.fn()
    render(<StatsPage onOpenGame={onOpenGame} onBack={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Hades' }))
    expect(onOpenGame).toHaveBeenCalledWith(8)
  })

  it('EST-005/o tempo importado aparece marcado no total e no jogo', () => {
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.getByText(/importados do LaunchBox/)).toBeInTheDocument()
    expect(screen.getByText(/inclui 1 h 00 min importados/)).toBeInTheDocument()
  })

  it('EST-003/sem tempo no período mostra a mensagem', () => {
    summaryMock.mockReturnValue({ data: { games: [], platforms: [], totalSeconds: 0, importedSeconds: 0 }, isError: false })
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.getByText('Nenhum tempo jogado neste período.')).toBeInTheDocument()
  })

  it('EST-002/o gráfico tem uma barra por semana com o tempo no título', () => {
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.getByRole('img', { name: /Tempo jogado nas últimas 12 semanas/ })).toBeInTheDocument()
    expect(weeklyMock).toHaveBeenCalledWith(12)
  })
})
