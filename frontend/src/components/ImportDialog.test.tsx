import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ImportDialog } from './ImportDialog'
import type { ImportCandidate } from '../lib/imports'

const scanMock = vi.fn()
const applyMock = vi.fn()
const pickMock = vi.fn()

vi.mock('../lib/imports', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/imports')>()
  return {
    ...actual,
    scanImport: (...args: unknown[]) => scanMock(...args),
    applyImport: (...args: unknown[]) => applyMock(...args),
    pickFolder: () => pickMock(),
  }
})

function candidate(over: Partial<ImportCandidate> = {}): ImportCandidate {
  return {
    source: 'steam',
    title: 'Counter-Strike 2',
    platform: 'PC',
    launchType: 'uri',
    launchTarget: 'steam://rungameid/730',
    installDir: '',
    developers: [],
    publishers: [],
    genres: [],
    year: null,
    description: '',
    playModes: [],
    playedSeconds: null,
    lastPlayed: null,
    exists: false,
    ...over,
  }
}

async function scanSteam() {
  fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
}

describe('ImportDialog', () => {
  beforeEach(() => {
    scanMock.mockReset()
    applyMock.mockReset()
    pickMock.mockReset()
    // applyImportInLots reaches the binding through window, not through the mocked export.
    ;(window as unknown as { go?: unknown }).go = {
      main: { App: { ImportApply: (c: unknown, p: unknown) => applyMock(c, p) } },
    }
  })

  async function scanMany(n: number) {
    scanMock.mockResolvedValue({
      candidates: Array.from({ length: n }, (_, i) =>
        candidate({ title: `Jogo ${i}`, launchTarget: `steam://rungameid/${i}` }),
      ),
      warnings: [],
    })
  }

  async function checkAll(n: number) {
    await scanSteam()
    await screen.findByRole('checkbox', { name: 'Jogo 0' })
    for (let i = 0; i < n; i++) {
      fireEvent.click(screen.getByRole('checkbox', { name: `Jogo ${i}` }))
    }
  }

  it('AJU-025/diálogo: "Importando k de N…" durante os lotes; Fechar desativado e Esc sem efeito', async () => {
    await scanMany(30)
    let release: (v: unknown) => void = () => {}
    applyMock.mockImplementationOnce(() => Promise.resolve({ applied: [], skipped: [] }))
    applyMock.mockImplementationOnce(
      () => new Promise((r) => {
        release = r
      }),
    )
    const onClose = vi.fn()
    render(<ImportDialog onImported={vi.fn()} onClose={onClose} />)
    await checkAll(30)
    fireEvent.click(screen.getByRole('button', { name: 'Importar selecionados' }))

    const status = await screen.findByText('Importando 25 de 30…')
    expect(status).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeDisabled()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()

    release({ applied: [], skipped: [] })
    await waitFor(() => expect(screen.queryByText(/Importando \d+ de 30…/)).toBeNull())
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeEnabled()
  })

  it('AJU-025/diálogo: falha no 2º lote mostra o resumo do 1º e role=alert com quantos não foram enviados; onImported é chamado', async () => {
    await scanMany(60)
    applyMock.mockResolvedValueOnce({
      applied: Array.from({ length: 25 }, (_, i) => ({ gameId: i, title: `Jogo ${i}` })),
      skipped: [],
    })
    applyMock.mockRejectedValueOnce(new Error('O agente não está respondendo.'))
    const onImported = vi.fn()
    render(<ImportDialog onImported={onImported} onClose={vi.fn()} />)
    await checkAll(60)
    fireEvent.click(screen.getByRole('button', { name: 'Importar selecionados' }))

    expect(await screen.findByText('Importados: 25')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent(/35/)
    expect(applyMock).toHaveBeenCalledTimes(2)
    expect(onImported).toHaveBeenCalledTimes(1)
  })

  it('IMP-010/Importar selecionados fica desabilitado sem marcados', async () => {
    scanMock.mockResolvedValue({ candidates: [candidate()], warnings: [] })
    render(<ImportDialog onImported={vi.fn()} onClose={vi.fn()} />)

    const apply = screen.getByRole('button', { name: 'Importar selecionados' })
    expect(apply).toBeDisabled()

    await scanSteam()
    const box = await screen.findByRole('checkbox', { name: /Counter-Strike 2/ })
    expect(apply).toBeDisabled()

    fireEvent.click(box)
    expect(apply).toBeEnabled()
  })

  it('IMP-010/mostra já existe e avisos, e não deixa marcar o jogo que já existe', async () => {
    scanMock.mockResolvedValue({
      candidates: [candidate({ title: 'Hades', exists: true })],
      warnings: ['Steam: appmanifest_570.acf ilegível'],
    })
    render(<ImportDialog onImported={vi.fn()} onClose={vi.fn()} />)

    await scanSteam()
    expect(await screen.findByText('já existe')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: /Hades/ })).toBeDisabled()
    expect(screen.getByRole('status')).toHaveTextContent('Steam: appmanifest_570.acf ilegível')
  })

  it('IMP-010/foco preso no diálogo: Tab no último elemento volta ao primeiro', async () => {
    render(<ImportDialog onImported={vi.fn()} onClose={vi.fn()} />)

    const first = screen.getByLabelText('Fonte')
    const close = screen.getByRole('button', { name: 'Fechar' })
    close.focus()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Tab' })

    expect(document.activeElement).toBe(first)
  })

  it('IMP-001/gravação só dos marcados e resultado com ignorados', async () => {
    scanMock.mockResolvedValue({
      candidates: [candidate({ title: 'Um' }), candidate({ title: 'Dois', launchTarget: 'steam://rungameid/2' })],
      warnings: [],
    })
    applyMock.mockResolvedValue({
      applied: [{ gameId: 9, title: 'Um' }],
      skipped: [{ title: 'Dois', reason: 'duplicate' }],
    })
    const onImported = vi.fn()
    render(<ImportDialog onImported={onImported} onClose={vi.fn()} />)

    await scanSteam()
    fireEvent.click(await screen.findByRole('checkbox', { name: /Um/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Importar selecionados' }))

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1))
    expect(applyMock.mock.calls[0][0]).toHaveLength(1)
    expect(applyMock.mock.calls[0][0][0].title).toBe('Um')
    expect(await screen.findByText('Importados: 1')).toBeInTheDocument()
    expect(screen.getByText('Dois: já estava na biblioteca')).toBeInTheDocument()
    expect(onImported).toHaveBeenCalledTimes(1)
  })
})
