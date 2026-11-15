import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MetadataBatchChip, type MetadataBatchChipProps } from './MetadataBatchChip'
import type { BatchStatusView } from '../lib/metadataBatch'

// Indicator of the batch in the header of the main window (phase 12, T38; MET-066, MET-067).
// The props are the contract of the component: status and interrupted come from useMetadataBatch,
// the callbacks are the actions of the header. The chip shows nothing without a batch.

const status = (over: Partial<BatchStatusView> = {}): BatchStatusView => ({
  batchId: 7,
  state: 'searching',
  total: 10,
  done: 0,
  failed: 0,
  warnings: [],
  ...over,
})

function props(over: Partial<MetadataBatchChipProps> = {}): MetadataBatchChipProps {
  return {
    status: null,
    interrupted: false,
    onOpenReview: vi.fn(),
    onCancel: vi.fn(),
    onDiscard: vi.fn(),
    onCloseSummary: vi.fn(),
    ...over,
  }
}

describe('MetadataBatchChip', () => {
  it('MET-067/sem busca não mostra nada', () => {
    const { container } = render(<MetadataBatchChip {...props()} />)

    expect(container).toBeEmptyDOMElement()
  })

  it('MET-067/searching mostra "Buscando metadados k/n" e Cancelar', () => {
    const p = props({ status: status({ state: 'searching', done: 4, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    expect(screen.getByText('Buscando metadados 4/10')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Descartar' })).toBeNull()
  })

  it('MET-067/searching atualiza o contador quando chega progresso', () => {
    const p = props({ status: status({ state: 'searching', done: 4, total: 10 }) })
    const { rerender } = render(<MetadataBatchChip {...p} />)

    rerender(<MetadataBatchChip {...p} status={status({ state: 'searching', done: 5, total: 10 })} />)

    expect(screen.getByText('Buscando metadados 5/10')).toBeInTheDocument()
    expect(screen.queryByText('Buscando metadados 4/10')).toBeNull()
  })

  it('MET-067/searching não tem ação de revisar e clicar no texto não abre a janela', () => {
    const p = props({ status: status({ state: 'searching', done: 2, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    expect(screen.queryByRole('button', { name: /Revisar metadados/ })).toBeNull()
    fireEvent.click(screen.getByText('Buscando metadados 2/10'))
    expect(p.onOpenReview).not.toHaveBeenCalled()
  })

  it('MET-067/searching: Cancelar chama onCancel uma vez', () => {
    const p = props({ status: status({ state: 'searching', done: 1, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))

    expect(p.onCancel).toHaveBeenCalledTimes(1)
    expect(p.onDiscard).not.toHaveBeenCalled()
  })

  it('MET-067/ready mostra "Revisar metadados (n)" e Descartar', () => {
    const p = props({ status: status({ state: 'ready', done: 10, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    expect(screen.getByRole('button', { name: /Revisar metadados \(10\)/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Descartar' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Cancelar' })).toBeNull()
  })

  it('MET-067/cancelled mostra "Revisar metadados (n)" e Descartar', () => {
    const p = props({ status: status({ state: 'cancelled', done: 6, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    expect(screen.getByRole('button', { name: /Revisar metadados \(10\)/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Descartar' })).toBeInTheDocument()
  })

  it('MET-067/clique em Revisar metadados abre a janela (ready e cancelled), uma vez', () => {
    const ready = props({ status: status({ state: 'ready', done: 10, total: 10 }) })
    const { unmount } = render(<MetadataBatchChip {...ready} />)

    fireEvent.click(screen.getByRole('button', { name: /Revisar metadados/ }))

    expect(ready.onOpenReview).toHaveBeenCalledTimes(1)
    expect(ready.onDiscard).not.toHaveBeenCalled()
    unmount()

    const cancelled = props({ status: status({ state: 'cancelled', done: 6, total: 10 }) })
    render(<MetadataBatchChip {...cancelled} />)

    fireEvent.click(screen.getByRole('button', { name: /Revisar metadados/ }))

    expect(cancelled.onOpenReview).toHaveBeenCalledTimes(1)
  })

  it('MET-067/Descartar em ready chama onDiscard e não abre a janela', () => {
    const p = props({ status: status({ state: 'ready', done: 10, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    fireEvent.click(screen.getByRole('button', { name: 'Descartar' }))

    expect(p.onDiscard).toHaveBeenCalledTimes(1)
    expect(p.onOpenReview).not.toHaveBeenCalled()
  })

  it('MET-067/applying mostra "Aplicando metadados k/n" e Cancelar', () => {
    const p = props({ status: status({ state: 'applying', done: 3, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    expect(screen.getByText('Aplicando metadados 3/10')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Descartar' })).toBeNull()
  })

  it('MET-066/Cancelar em applying chama onCancel', () => {
    const p = props({ status: status({ state: 'applying', done: 3, total: 10 }) })
    render(<MetadataBatchChip {...p} />)

    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))

    expect(p.onCancel).toHaveBeenCalledTimes(1)
  })

  it('MET-066/done mostra o resumo com os três números e Fechar chama onCloseSummary', () => {
    const p = props({
      status: status({
        state: 'done',
        done: 10,
        total: 10,
        failed: 2,
        result: { applied: 3, imagesQueued: 5, imagesSkippedByLimit: 0, failedTitles: ['Alfa', 'Beta'] },
      }),
    })
    render(<MetadataBatchChip {...p} />)

    expect(screen.getByText('Metadados aplicados em 3 jogos; 5 imagens na fila; 2 falharam')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Fechar' }))

    expect(p.onCloseSummary).toHaveBeenCalledTimes(1)
    expect(p.onDiscard).not.toHaveBeenCalled()
  })

  it('MET-067/interrupted mostra o aviso uma vez', () => {
    const p = props({ status: null, interrupted: true })
    render(<MetadataBatchChip {...p} />)

    expect(screen.getAllByText('A busca de metadados foi interrompida. Use Buscar metadados no jogo.')).toHaveLength(1)
  })

  it('MET-067/sem interrupção e sem busca não mostra o aviso', () => {
    render(<MetadataBatchChip {...props({ status: null, interrupted: false })} />)

    expect(screen.queryByText(/foi interrompida/)).toBeNull()
  })
})
