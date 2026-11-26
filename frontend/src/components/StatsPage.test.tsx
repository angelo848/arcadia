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
  manualSeconds: 0, // alterado em 2026-11-25 (fase 16)
  games: [
    { gameId: 7, title: 'Celeste', platform: 'PC', sessions: 2, activeSeconds: 1800, importedSeconds: 0, manualSeconds: 0, lastPlayedAt: '' }, // alterado em 2026-11-25 (fase 16)
    { gameId: 8, title: 'Hades', platform: 'PC', sessions: 1, activeSeconds: 3600, importedSeconds: 3600, manualSeconds: 0, lastPlayedAt: '' }, // alterado em 2026-11-25 (fase 16)
  ],
  platforms: [{ platformId: 1, name: 'PC', activeSeconds: 5400, importedSeconds: 3600, manualSeconds: 0 }], // alterado em 2026-11-25 (fase 16)
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
    summaryMock.mockReturnValue({ data: { games: [], platforms: [], totalSeconds: 0, importedSeconds: 0, manualSeconds: 0 }, isError: false }) // alterado em 2026-11-25 (fase 16)
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.getByText('Nenhum tempo jogado neste período.')).toBeInTheDocument()
  })

  // SEL-014 (fase 16): marca de tempo adicionado à mão.
  const withManual: StatsSummary = {
    ...summary,
    totalSeconds: 7200,
    manualSeconds: 1800,
    games: [{ ...summary.games[0], activeSeconds: 3600, manualSeconds: 1800 }, summary.games[1]],
  }

  it('SEL-014/total do período com manualSeconds > 0 mostra "· X adicionados à mão" ao lado dos importados', () => {
    summaryMock.mockReturnValue({ data: withManual, isError: false })
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.getByText(/· 30 min adicionados à mão/)).toBeInTheDocument()
    expect(screen.getByText(/importados do LaunchBox/)).toBeInTheDocument()
  })

  it('SEL-014/linha de jogo com manualSeconds > 0 mostra "inclui X adicionados à mão"', () => {
    summaryMock.mockReturnValue({ data: withManual, isError: false })
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.getByText(/inclui 30 min adicionados à mão/)).toBeInTheDocument()
  })

  it('SEL-014/manualSeconds 0 não mostra a marca', () => {
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.queryByText(/adicionados à mão/)).toBeNull()
  })

  it('SEL-014/importados e manuais no mesmo item mostram as duas marcas', () => {
    summaryMock.mockReturnValue({
      data: {
        ...summary,
        manualSeconds: 0,
        games: [{ ...summary.games[1], activeSeconds: 5400, importedSeconds: 3600, manualSeconds: 1800 }],
      },
      isError: false,
    })
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    // alterado em 2026-11-25 (fase 16)
    expect(screen.getByText(/inclui 1 h 00 min importados/)).toBeInTheDocument()
    expect(screen.getByText(/30 min adicionados à mão/)).toBeInTheDocument()
  })

  it('EST-002/o gráfico tem uma barra por semana com o tempo no título', () => {
    render(<StatsPage onOpenGame={vi.fn()} onBack={vi.fn()} />)
    expect(screen.getByRole('img', { name: /Tempo jogado nas últimas 12 semanas/ })).toBeInTheDocument()
    expect(weeklyMock).toHaveBeenCalledWith(12)
  })
})
