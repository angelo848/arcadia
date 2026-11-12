import { describe, expect, it, vi, beforeEach } from 'vitest'
import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EmulatorsDialog, validateEmulator } from './EmulatorsDialog'
import type { EmulatorView } from '../lib/emulators'

const addMock = vi.fn()
const updateMock = vi.fn()
const deleteMock = vi.fn()
const setDefaultMock = vi.fn()

const emulators: EmulatorView[] = [{ id: 1, name: 'SNES9x', exePath: 'C:\\Emu\\snes9x.exe', argsTemplate: '-f {rom}' }]

vi.mock('../lib/emulators', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/emulators')>()
  return {
    ...actual,
    useEmulators: () => ({ data: emulators }),
    addEmulator: (...a: unknown[]) => addMock(...a),
    updateEmulator: (...a: unknown[]) => updateMock(...a),
    deleteEmulator: (...a: unknown[]) => deleteMock(...a),
    setPlatformEmulator: (...a: unknown[]) => setDefaultMock(...a),
  }
})

vi.mock('../lib/library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/library')>()
  return { ...actual, useCatalog: () => ({ data: { platforms: [] } }) }
})

function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<EmulatorsDialog onClose={vi.fn()} />, { wrapper: wrapper(client) })
}

describe('EmulatorsDialog', () => {
  beforeEach(() => {
    addMock.mockReset()
    updateMock.mockReset()
    deleteMock.mockReset()
    setDefaultMock.mockReset()
  })

  it('EMU-001/cadastro recusa marcador desconhecido e não grava', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Emu' } })
    fireEvent.change(screen.getByLabelText('Executável (caminho completo)'), { target: { value: 'C:\\Emu\\e.exe' } })
    fireEvent.change(screen.getByLabelText(/Argumentos/), { target: { value: '-x {foo}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Marcador desconhecido: {foo}')
    expect(addMock).not.toHaveBeenCalled()
  })

  it('EMU-001/executável com caminho relativo é recusado', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Emu' } })
    fireEvent.change(screen.getByLabelText('Executável (caminho completo)'), { target: { value: 'e.exe' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('caminho completo')
    expect(addMock).not.toHaveBeenCalled()
  })

  it('EMU-006/apagar pede confirmação antes de excluir', async () => {
    deleteMock.mockResolvedValue(undefined)
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Excluir' }))
    expect(deleteMock).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar exclusão' }))
    await waitFor(() => expect(deleteMock).toHaveBeenCalledWith(1))
  })
})

describe('validateEmulator', () => {
  it('EMU-001/aceita argumentos com {rom} e {dir} e um caminho absoluto', () => {
    expect(
      validateEmulator({ id: null, name: 'Emu', exePath: 'C:\\Emu\\e.exe', argsTemplate: '-f {rom} -d {dir}' }),
    ).toBe('')
  })
})
