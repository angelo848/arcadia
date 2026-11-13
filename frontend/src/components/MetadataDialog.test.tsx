import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MetadataDialog } from './MetadataDialog'
import type { ScrapeCandidateView } from '../lib/metadata'

const searchMock = vi.fn()
const applyMock = vi.fn()

vi.mock('../lib/metadata', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/metadata')>()
  return {
    ...actual,
    scrapeSearch: (...a: unknown[]) => searchMock(...a),
    scrapeApply: (...a: unknown[]) => applyMock(...a),
  }
})

const candidates: ScrapeCandidateView[] = [
  { providerId: '7', title: 'Celeste', platform: 'PC', year: 2018, score: 0.95, accepted: true },
  { providerId: '9', title: 'Outro jogo', platform: 'Switch', year: 2001, score: 0.4, accepted: false },
]

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MetadataDialog gameId={3} onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('MetadataDialog', () => {
  beforeEach(() => {
    searchMock.mockReset()
    applyMock.mockReset()
  })

  it('MID-002/mostra os candidatos com a pontuação e marca o provável', async () => {
    searchMock.mockResolvedValue(candidates)
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))

    expect(await screen.findByText('Celeste')).toBeInTheDocument()
    expect(screen.getByText(/correspondência 95%/)).toHaveTextContent('provável')
    expect(screen.getByText(/correspondência 40%/)).not.toHaveTextContent('provável')
    expect(searchMock).toHaveBeenCalledWith(3, 'igdb')
  })

  it('MID-003/aplicar sem marcar substituir manda replace falso', async () => {
    searchMock.mockResolvedValue(candidates)
    applyMock.mockResolvedValue({ game: {}, queued: 2 })
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await screen.findByText('Celeste')
    fireEvent.click(screen.getAllByRole('button', { name: 'Aplicar' })[0])

    await waitFor(() => expect(applyMock).toHaveBeenCalledWith(3, 'igdb', '7', false))
    expect(await screen.findByText(/2 imagem\(ns\) na fila/)).toBeInTheDocument()
  })
})
