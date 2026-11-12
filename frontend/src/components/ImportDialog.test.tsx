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
