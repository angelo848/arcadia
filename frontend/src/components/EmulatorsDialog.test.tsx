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
const pickMock = vi.fn()

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
    pickEmulatorExe: () => pickMock(),
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
    pickMock.mockReset()
  })

  it('EMU-006/Procurar preenche o executável escolhido no Explorer', async () => {
    pickMock.mockResolvedValue('D:\\Emus\\mgba.exe')
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.click(screen.getByRole('button', { name: 'Procurar...' }))

    await waitFor(() =>
      expect(screen.getByLabelText('Caminho do aplicativo (executável do emulador)')).toHaveValue('D:\\Emus\\mgba.exe'),
    )
  })

  it('EMU-006/Procurar cancelado não muda o caminho', async () => {
    pickMock.mockResolvedValue('')
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.change(screen.getByLabelText('Caminho do aplicativo (executável do emulador)'), { target: { value: 'C:\\a.exe' } })
    fireEvent.click(screen.getByRole('button', { name: 'Procurar...' }))

    await waitFor(() => expect(pickMock).toHaveBeenCalledTimes(1))
    expect(screen.getByLabelText('Caminho do aplicativo (executável do emulador)')).toHaveValue('C:\\a.exe')
  })

  it('EMU-001/os botões de marcador acrescentam {rom} e {dir} aos parâmetros', () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.click(screen.getByRole('button', { name: 'Jogo ({rom})' }))
    fireEvent.click(screen.getByRole('button', { name: 'Pasta do jogo ({dir})' }))

    expect(screen.getByLabelText('Parâmetros de linha de comando padrão')).toHaveValue('{rom} {dir}')
  })

  it('EMU-004/a amostra de comando aparece com o caminho ao lado do executável', () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.change(screen.getByLabelText('Caminho do aplicativo (executável do emulador)'), { target: { value: 'C:\\Emu\\lichii.exe' } })
    fireEvent.change(screen.getByLabelText('Parâmetros de linha de comando padrão'), { target: { value: '-f {rom}' } })

    expect(screen.getByText('lichii.exe -f "CAMINHO\\COMPLETO\\PARA\\ARQUIVO\\ROM"')).toBeInTheDocument()
  })

  it('EMU-001/cadastro recusa marcador desconhecido e não grava', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Emu' } })
    fireEvent.change(screen.getByLabelText('Caminho do aplicativo (executável do emulador)'), { target: { value: 'C:\\Emu\\e.exe' } })
    fireEvent.change(screen.getByLabelText('Parâmetros de linha de comando padrão'), { target: { value: '-x {foo}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Marcador desconhecido: {foo}')
    expect(addMock).not.toHaveBeenCalled()
  })

  it('EMU-001/executável com caminho relativo é recusado', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Novo emulador' }))
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Emu' } })
    fireEvent.change(screen.getByLabelText('Caminho do aplicativo (executável do emulador)'), { target: { value: 'e.exe' } })
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
